# internal/interactionchannel

Phase 2 IPC between the orchestrator pane and the three display panes (header, log, footer) over a Unix domain socket. The orchestrator is the server; each display pane sub-process is a client.

## Package purpose

`internal/interactionchannel` owns the full wire protocol: framing, serialisation, fanout, and the readiness handshake. The orchestrator calls `Serve`; each pane sub-process calls `Dial`. From that point forward both sides exchange typed `Message` values through a `Channel` handle; the framing and connection management are invisible to callers.

The socket lives at `<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock` inside the existing `0700`-mode `.pr9k/` directory (D-21). Access control is filesystem-permission-based; no client-identity handshake beyond the filesystem permissions is performed.

## Exported types

### `Channel`

The central handle returned by both `Serve` and `Dial`. Callers do not instantiate `Channel` directly.

| Method | Description |
|---|---|
| `Recv() <-chan Message` | Returns the receive channel. On the server side, delivers messages from all connected clients. On the client side, delivers messages from the server. |
| `Send(msg Message)` | Broadcasts `msg` to all active connections (server side) or sends to the server (client side). Non-blocking: if the outbound channel is full the message is dropped for that recipient. |
| `SendStateHeader(msg StateHeader)` | Sends a header-state update. Latest-wins semantics: if the recipient's channel already has an unread `StateHeader`, it is replaced. |
| `SendStateLog(msg StateLog)` | Sends a log-line batch. Drop-oldest semantics: if the recipient's log channel is full, the oldest unread entry is discarded to make room. |
| `SendStateFooter(msg StateFooter)` | Sends a footer-state update. Latest-wins semantics: same as `SendStateHeader`. |
| `AwaitReady(ctx context.Context, timeout time.Duration) error` | Blocks until all three roles (`header`, `log`, `footer`) have sent a `Ready` message, or until the deadline fires. Returns a structured error naming the missing roles on timeout. Only meaningful on the server (orchestrator) side. |
| `Close() error` | Cancels the channel's internal context, closes the listener (server) or connection (client), and unlinks the socket file (server only). Safe to call more than once. |

### `Message` (interface)

All wire messages implement `Message`. Callers type-switch on the concrete type after receiving from `Recv()`.

```go
type Message interface {
    WireType() string
    sealed()
}
```

### Pane → orchestrator messages

| Type | Fields | Purpose |
|---|---|---|
| `Ready` | `Role string` | Sent by each pane immediately after dialling. The `Role` value is one of `header`, `log`, `footer`. Duplicate `Ready` messages from the same role are idempotent. |
| `Intent` | `Kind IntentType` | User-initiated action forwarded from the footer pane's key handler. |
| `DoneAck` | `Role string` | Final-state acknowledgement sent after a pane has rendered its response to `WorkspaceDone`. |

### Orchestrator → pane messages

| Type | Fields | Purpose |
|---|---|---|
| `StateHeader` | `IterationLine string`, `StepNames []string`, `StepStates []string` | Full header pane state snapshot. Pane re-renders on receipt. |
| `StateLog` | `Lines [][]byte` | Batch of pre-rendered log lines. Pane appends to its viewport. |
| `StateFooter` | `Mode int`, `ShortcutLine string` | Footer pane state: keyboard-mode identifier and the rendered shortcut bar string. |
| `WorkspaceDone` | `ExitCode int` | Signals workflow completion. Each pane renders its final state and replies with `DoneAck`. |

### `IntentType`

Typed string constant for `Intent.Kind`:

| Value | Meaning |
|---|---|
| `IntentRetry` | `"retry"` |
| `IntentContinue` | `"continue"` |
| `IntentQuit` | `"quit"` |
| `IntentSkip` | `"skip"` |
| `IntentNext` | `"next"` |

## Exported functions

```go
func Serve(ctx context.Context, socketPath string) (*Channel, error)
```

Binds the Unix socket at `socketPath` and starts the accept loop. Returns a `*Channel` ready to call `AwaitReady`, `Send*`, and `Recv`. The socket file is unlinked when `Close` is called or when the context is cancelled. If a stale socket exists at the path, `Serve` unlinks it before binding.

```go
func Dial(ctx context.Context, socketPath string, role string) (*Channel, error)
```

Connects to the socket at `socketPath` and immediately sends a `Ready{Role: role}` message. Returns a `*Channel` ready to call `Send` and `Recv`. The `role` value must be one of `"header"`, `"log"`, or `"footer"`.

## Constants

```go
const ReadyHandshakeTimeout = 10 * time.Second
```

The default deadline passed to `AwaitReady` by `runCmuxOrchestrator`.

## Concurrency model

### Per-connection goroutines (D-2, D-3)

Each accepted connection spawns three goroutines on the server side:

- **read goroutine** — reads length-prefixed frames from the connection, unmarshals them, and sends to the shared `recvCh` channel (capacity 64).
- **write goroutine** — drains four source channels (general broadcast, log, header notify, footer notify) and writes frames to the connection.
- **watcher goroutine** — detects connection close and cancels the per-connection context so the read/write goroutines exit cleanly.

The client side runs a symmetric read + write pair (no watcher; cancellation comes from the parent context).

### Per-role buffered channels (D-2)

Each connection's write goroutine owns four channels:

| Channel | Capacity | Semantics |
|---|---|---|
| general broadcast | 32 | FIFO; non-blocking send drops on full |
| log | 256 | Drop-oldest when full |
| header notify | 1 | Latest-wins (buffered channel with drain-before-send) |
| footer notify | 1 | Latest-wins (buffered channel with drain-before-send) |

### Handshake mutex (D-4)

`AwaitReady` tracks per-role arrival behind a `sync.Mutex`. The per-role boolean is set to `true` on the first `Ready` from that role; subsequent duplicates are no-ops. When all three booleans are true the waiting goroutine is unblocked via a `sync.Cond`.

### Ack tracker (D-15)

The `WorkspaceDone` ack wait uses a separate `sync.Mutex`-protected counter. Each `DoneAck` decrements the counter. The orchestrator waits up to 5 seconds (`DoneAckTimeout`) for the counter to reach zero; non-acking panes do not block shutdown.

## Wire protocol

### Framing

Messages are sent as length-prefixed JSON frames:

```
[ uint32 length (big-endian) ][ JSON payload (length bytes) ]
```

Maximum message size: 1 MiB. Frames exceeding this limit are rejected at the read boundary with a connection-close.

### JSON discriminator (R3 forward-compat)

Every JSON payload carries a `"type"` field whose value is the `WireType()` string of the concrete message. Unknown `"type"` values are silently ignored; unknown JSON fields within known types are discarded. This allows orchestrators and panes from adjacent versions to coexist without hard failures.

Example `Ready` frame payload:

```json
{"type":"ready","role":"log"}
```

Example `StateLog` frame payload:

```json
{"type":"state_log","lines":["<base64-encoded rendered line>"]}
```

## Lifecycle

```
[orchestrator]                    [display pane]
Serve(ctx, path)
  bind socket
  start accept loop
                                  Dial(ctx, path, role)
  accept connection                 connect
  spawn read+write+watcher          send Ready{role}
  read goroutine receives Ready
  handshake counter incremented
  (×3 for header/log/footer)
AwaitReady returns nil
pre-populate StateHeader (D-8)
begin workflow steps
  SendStateHeader(...)           receive StateHeader → re-render
  SendStateLog(...)              receive StateLog → append to viewport
  SendStateFooter(...)           receive StateFooter → re-render
  ...
workflow finishes
  Send(WorkspaceDone{ExitCode})  receive WorkspaceDone → render final state
                                   send DoneAck{role}
AwaitDoneAck (≤5s)
Close()
  cancel context
  close listener
  unlink socket file
                                 Recv() channel closed → exit
```

## Test doubles

### `FakeInteractionChannel`

Implements both the orchestrator-facing and pane-facing `Channel` contract without a real socket. Use in orchestrator unit tests to verify send/receive sequencing without spawning sub-processes.

| Method | Behaviour |
|---|---|
| `EnqueueReady(role string)` | Injects a `Ready` message as if a pane connected |
| `InjectIntent(kind IntentType)` | Injects an `Intent` message |
| `InjectMessage(msg Message)` | Injects any message |
| `ExpectStatePush(n int) []Message` | Drains and returns the next `n` messages from the sent queue (blocking up to a short timeout) |
| `IsClosed() bool` | Returns true once `Close` has been called |

### `FakeDisplayPane`

Dials a real socket and simulates a display pane's read loop. Use in channel integration tests to verify handshake, fan-out, and `WorkspaceDone` sequencing against a real `Serve` listener.

| Method | Behaviour |
|---|---|
| `Connect(ctx, socketPath, role)` | Dials and sends `Ready{role}` |
| `SendReady(role string)` | Sends an additional `Ready` (for duplicate-ready tests) |
| `ExpectMessage(t, timeout) Message` | Blocks until a message arrives or the timeout fires; calls `t.Fatal` on timeout |
| `Disconnect()` | Closes the connection |
| `Received() []Message` | Returns a snapshot of all messages received so far |
| `SetSlowConsumer(delay time.Duration)` | Injects a read delay to simulate back-pressure |
| `SetPrematureExit(after int)` | Closes the connection after receiving `after` messages |

### `FakeFooterKeySource`

Scripts keystroke sequences for footer pane tests.

| Method | Behaviour |
|---|---|
| `Press(key string)` | Appends a key to the queue |
| `Next() string` | Pops the next key (blocks if queue is empty) |
| `SetMode(mode int)` | Records the current mode |
| `Mode() int` | Returns the last recorded mode |
| `RecordIntent(kind IntentType)` | Records a forwarded intent |
| `LastForwardedIntent() IntentType` | Returns the most recently recorded intent |

## References

- [cmux mode feature doc](../features/cmux-mode.md)
- [Setting up cmux how-to](../how-to/setting-up-cmux.md)
- [Phase 2 implementation plan](../plans/cmux-rebuild/phase-2-real-workflow-runs/feature-implementation-plan.md)
- [Phase 2 implementation decision log](../plans/cmux-rebuild/phase-2-real-workflow-runs/artifacts/implementation-decision-log.md)
- [cmuxctl code package doc](cmuxctl.md)
