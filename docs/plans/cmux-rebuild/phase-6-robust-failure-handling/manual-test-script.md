# Phase 6 Manual Test Script

This script documents the three W-7 scenarios that must be run against a real
cmux installation before merging Phase 6. It is the **go/no-go merge gate** for
the liveness wiring — if Scenario 1 fails (a quiet step false-aborts), the PR
does not merge.

## Pre-conditions

- cmux v0.64.7 or compatible installed and running.
- `pr9k` built from this branch (`make build`).
- A target repo with a real workflow that contains at least one claude step that
  can run quietly for more than 45 seconds (e.g. a long summarization task with
  no interim tool output).
- A terminal pane open inside a cmux session; `CMUX_SOCKET_PATH` resolvable.

---

## Scenario 1 — Quiet step does NOT false-abort (go/no-go gate)

**Purpose.** Verify that the 10-second liveness emitter keeps the 45-second
stall threshold from tripping on a healthy-but-quiet channel. A real workflow
with a long, output-silent step must complete without any abort.

**Pass criterion.** The run completes normally — all panes show their final
state, a completion notification fires, and **no** `run aborted` line appears
in any pane or in the per-run `.pr9k/logs/` file.

**Steps:**

1. Choose (or create) a workflow step that produces no output for at least
   60 seconds. A single-step workflow calling `sleep 90` via a shell step
   works. Alternatively, a Claude step given a large summarization task with
   no intermediate tool calls.

2. Launch pr9k in cmux mode against that workflow:
   ```
   pr9k --cmux --project-dir <repo> -n 1
   ```

3. Observe the log pane. During the quiet step:
   - The log pane must show no new lines from the step.
   - The footer pane must remain interactive (no freezing).
   - No `run aborted` line must appear.

4. Allow the step to complete. Confirm:
   - All three panes show their final content without any `run aborted` line.
   - A `pr9k run completed in <repo>` notification fires (visible in cmux's
     notification chrome).
   - The per-run `.pr9k/logs/<stamp>/` directory contains the expected JSONL
     artifacts with no abort diagnostic.

**Expected result:** PASS — the run completes normally, no false abort.

---

## Scenario 2 — Deliberate channel wedge causes a convergent abort

**Purpose.** Verify that a stalled channel aborts the run within the 45-second
stall threshold, all panes render `run aborted` and exit, exactly one
run-aborted notification fires, the sidebar entries clear, and the classified
diagnostic is in the log file. The workspace must remain open after the abort.

**Note.** Wedging the interaction channel requires inserting a `time.Sleep` (or
equivalent) in a test build, or deliberately killing a display pane process
after the handshake but while a step is running. The simplest approach is to
kill the log or header pane process mid-run using `kill <pid>`.

**Setup.** Build a test binary with a shortened stall threshold of 10 seconds
for a faster-paced test run (edit `interactionchannel.StallThreshold` to a
small value in a local test build, or use the injectable test path). Restore the
production constant before merging.

**Steps:**

1. Start a pr9k run in cmux mode against a workflow with a long step:
   ```
   pr9k --cmux --project-dir <repo> -n 1
   ```

2. Wait for the readiness handshake to complete (all three panes visible and
   showing initial content).

3. While a step is running, kill one of the display pane processes:
   ```
   kill <pid-of-log-pane-process>
   ```
   You can find the pid via `ps aux | grep "cmux-pane"`.

4. Observe within the stall window (≤45 seconds; ≤10 seconds with a shortened
   threshold):
   - The remaining panes render a final line containing `run aborted`.
   - All panes exit (cmux shows them as exited/closed).
   - The cmux workspace shows exactly **one** `pr9k run aborted in <repo>`
     notification.
   - The sidebar status pill and progress bar clear.

5. Check the per-run log file:
   ```
   cat <projectDir>/.pr9k/logs/<stamp>/run.log
   ```
   The classified abort diagnostic line must be present (e.g.
   `abort: run aborted: display_loss`).

6. Confirm the workspace stays open in cmux (does not auto-dismiss). Dismiss it
   manually when done.

**Expected result:** PASS — clean convergent abort within the stall window;
exactly one notification; diagnostic in log; workspace stays open.

---

## Scenario 3 — Near-simultaneous pane-death and cmux-timeout produce exactly one notification

**Purpose.** Verify that when two failure events fire near-simultaneously, the
abort gate absorbs the second detection and exactly one `FireRunAborted` call
reaches cmux — not two.

**Steps:**

1. Start a pr9k run in cmux mode against a workflow with a long step:
   ```
   pr9k --cmux --project-dir <repo> -n 1
   ```

2. Wait for a step to be running.

3. Near-simultaneously (within ~1 second):
   a. Kill a display pane process: `kill <pid-of-log-pane-process>`
   b. Interrupt the cmux socket connection, e.g. by briefly setting an invalid
      `CMUX_SOCKET_PATH` via a wrapper, or by pausing the cmux process
      (`kill -STOP <cmux-pid>`) for slightly longer than the cmux per-call
      timeout (5 s), then resuming it (`kill -CONT <cmux-pid>`).

   A simpler approximation: kill two pane processes in rapid succession:
   ```
   kill <pid-of-log-pane> <pid-of-header-pane>
   ```

4. Observe: exactly **one** `pr9k run aborted in <repo>` notification fires in
   cmux, not two. The notification chrome shows a single entry.

5. Confirm `run aborted` appears in both remaining panes and both exit.

**Expected result:** PASS — exactly one run-aborted notification regardless of
how many concurrent failure detections race to the abort gate.

---

## Recording results

After running all three scenarios, record the outcome of each in the PR
description or a comment on the tracking issue:

| Scenario | Result | Notes |
|---|---|---|
| 1 — quiet step non-abort | PASS / FAIL | |
| 2 — deliberate wedge | PASS / FAIL | Stall window observed: ___ s |
| 3 — near-simultaneous multi-failure | PASS / FAIL | |

If Scenario 1 fails (a quiet step false-aborts), the PR does not merge. Lower
`LivenessCadence` or raise `StallThreshold` and re-verify before proceeding.

Also record which `*CmuxError.Code` strings appeared in Scenario 2 or 3 against
the classifier (RAID A4 / I2 from the implementation plan): were they
`access-denied`, `auth_*`, `method_not_found`, or something unrecognized that
fell through to the raw-code path?
