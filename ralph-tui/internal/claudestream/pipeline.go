package claudestream

import (
	"sync/atomic"
	"time"
)

// sentinel is appended to the .jsonl file after each result event to allow
// downstream tools to detect truncated artifacts (D26).
var sentinel = []byte(`{"type":"ralph_end","ok":true,"schema":"v1"}`)

// Pipeline composes Parser + Renderer + Aggregator + RawWriter behind a
// single entry point. It owns the per-step .jsonl file and coordinates all
// writes from a single goroutine (the stdout-forwarding goroutine).
//
// Pipeline is not safe for concurrent use from multiple writers. The
// lastEventAt field is safe for concurrent reads from the heartbeat goroutine.
type Pipeline struct {
	parser     *Parser
	renderer   *Renderer
	aggregator *Aggregator
	rawWriter  *RawWriter
	// lastEventAt is updated atomically on every Observe call (even for
	// malformed lines) so the heartbeat reader gets accurate silence duration.
	lastEventAt atomic.Int64
}

// NewPipeline constructs a Pipeline that writes raw bytes to rawWriter.
// rawWriter may be nil (disables persistence, useful in tests that do not
// want to touch disk).
func NewPipeline(rawWriter *RawWriter) *Pipeline {
	return &Pipeline{
		parser:     &Parser{},
		renderer:   &Renderer{},
		aggregator: &Aggregator{},
		rawWriter:  rawWriter,
	}
}

// Observe processes one raw NDJSON line from claude's stdout:
//  1. Writes verbatim bytes to RawWriter (if set).
//  2. Updates lastEventAt (even for malformed lines — any activity counts).
//  3. Parses the line; returns nil on MalformedLineError (caller logs).
//  4. Folds the event into the Aggregator.
//  5. On ResultEvent: writes the sentinel line to RawWriter (D26).
//  6. Returns Renderer.Render output.
func (p *Pipeline) Observe(line []byte) []string {
	// Step 1: verbatim write before any parsing.
	if p.rawWriter != nil {
		_ = p.rawWriter.WriteLine(line)
	}

	// Step 2: stamp activity time before dispatch (D23).
	p.lastEventAt.Store(time.Now().UnixNano())

	// Step 3: parse.
	ev, err := p.parser.Parse(line)
	if err != nil {
		return nil
	}

	// Step 4: aggregate.
	p.aggregator.Observe(ev)

	// Step 5: sentinel after ResultEvent (D26).
	if _, ok := ev.(*ResultEvent); ok {
		if p.rawWriter != nil {
			_ = p.rawWriter.WriteLine(sentinel)
		}
	}

	// Step 6: render.
	return p.renderer.Render(ev)
}

// LastEventAt returns the wall-clock time of the most recently observed line.
// Returns the zero value if no line has been observed yet.
func (p *Pipeline) LastEventAt() time.Time {
	ns := p.lastEventAt.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// Aggregator returns the pipeline's Aggregator for post-step inspection.
func (p *Pipeline) Aggregator() *Aggregator {
	return p.aggregator
}

// Renderer returns the pipeline's Renderer for Finalize calls.
func (p *Pipeline) Renderer() *Renderer {
	return p.renderer
}

// Close flushes and closes the underlying RawWriter. Idempotent.
func (p *Pipeline) Close() error {
	if p.rawWriter == nil {
		return nil
	}
	return p.rawWriter.Close()
}
