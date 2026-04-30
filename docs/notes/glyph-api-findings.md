> Superseded by the Bubble Tea migration — see [`docs/adr/20260411070907-bubble-tea-tui-framework.md`](../adr/20260411070907-bubble-tea-tui-framework.md).

# Glyph API Findings — Issue #46

**Module:** `github.com/kungfusheep/glyph v0.0.0-20260405220257-50a1ab7d0a9b`
**Verified via:** `go doc`, source inspection, and scratch app compilation (scratch app, since removed)

---

## Summary

The design shape from V1 survives: pointer-bound strings, VBox/HBox layout tree, `Log` widget fed by `io.Reader`. Only the keyboard binding name changes. All other API elements exist with the names assumed in the plan, with one nuance on VBox/HBox call syntax.

---

## Findings per assumed API element

### `Text(&stringField)` — pointer-bound text widget

**Status: exists, name matches.**

`Text(content any)` accepts any value including `*string`. Glyph reads the current dereferenced value on every render tick. No adjustments needed.

### `HBox(...children)` and `VBox(...children)` — layout containers

**Status: exists, name matches. Call syntax differs from some plan sketches.**

Both are function-typed vars (`VBoxFn`, `HBoxFn`). The modifier methods chain on the **function**, not on the result — you configure first, then call with children:

```go
// Correct — modifiers before children
glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(child1, child2)
glyph.HBox(child1, child2)   // no modifiers needed
```

Any plan sketch that chains modifiers after the child call needs adjusting. `Grow`, `Border`, `Title`, `Width`, `Height`, `Gap`, `Padding` are all available on `VBoxFn`/`HBoxFn`.

### `Log(io.Reader)` — streaming log panel

**Status: exists, name matches.**

`Log(r io.Reader) *LogC` — reads from the reader in a background goroutine, ring-buffers lines, auto-scrolls. Returns `*LogC` which supports all the chained modifiers below.

### `.Border(...).Title("Ralph")` — decoration

**Status: exists, name matches.**

`VBox.Border(glyph.BorderRounded).Title("Ralph")` works exactly as assumed. `BorderRounded` is a package-level var. `BorderSingle` and `BorderDouble` also available.

### `.Grow(1)` — flex-grow modifier

**Status: exists, name matches.**

Available on both `VBoxFn` and `*LogC` (and `HBoxFn`). `.Grow(1)` causes the widget to expand and fill remaining space.

### `.MaxLines(n)` — ring buffer cap on Log

**Status: exists, name matches.**

`(*LogC).MaxLines(n int) *LogC` — limits the in-memory line buffer to `n` lines (ring buffer). Plan uses `.MaxLines(500)`.

### `.BindVimNav()` — vim navigation on Log

**Status: exists, name matches.**

`(*LogC).BindVimNav() *LogC` — enables `j`/`k`/`g`/`G` navigation on the log panel. No adjustments needed.

### Keyboard registration — `app.BindKey` / `app.OnKey`

**Status: name does NOT match — adjustment required.**

The plan assumed either `app.BindKey("q", fn)` (per-key) or `app.OnKey(fn)` (catch-all). **Neither exists.**

The actual API is:

```go
app.Handle(pattern string, handler any) *App
```

`Handle` is the per-key binding method. Example:

```go
app.Handle("q", func() { app.Stop() })
app.Handle("<Ctrl+C>", func() { app.Stop() })
```

For a catch-all, the plan should use `Handle` with appropriate patterns or the `riffkey` input layer directly via `app.Input()`.

**Action for #47:** Replace every reference to `app.BindKey(...)` or `app.OnKey(...)` in design sketches with `app.Handle(...)`.

### `app.Run()` — main event loop

**Status: exists, name matches.**

`(*App).Run() error` — blocks until the app exits. Returns an error if initialization fails. No adjustments needed.

---

## Other API notes for #47+

- `glyph.NewApp()` creates the app. `glyph.NewInlineApp()` creates a non-fullscreen variant.
- `app.Stop()` triggers a clean shutdown.
- `app.SetView(view any) *App` sets the root widget tree. Call once before `Run()`.
- `app.RequestRender()` triggers a re-render from outside the render loop (useful from goroutines updating pointer-bound fields).
- `(*LogC).OnUpdate(f func())` fires after each new line is appended — useful for triggering header re-renders when subprocess output arrives.
- No `app.BindKey` or `app.OnKey` exist anywhere in the package.
