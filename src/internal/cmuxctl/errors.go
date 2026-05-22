package cmuxctl

import (
	"fmt"
	"time"
)

// TimeoutError is returned by RealClient when a queued RPC call does not
// receive a response within the per-call timeout. Callers classify it with
// errors.As — do not test with errors.Is(err, context.DeadlineExceeded),
// which does not match queue-level timeouts.
type TimeoutError struct {
	Method   string
	Duration time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("cmuxctl: %s timed out after %s", e.Method, e.Duration)
}
