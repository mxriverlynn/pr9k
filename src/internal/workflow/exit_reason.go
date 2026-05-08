package workflow

// ExitReason describes how a Run call terminated.
type ExitReason string

const (
	// ExitReasonCompleted means all configured phases ran to completion.
	ExitReasonCompleted ExitReason = "completed"
	// ExitReasonLoopBroken means the iteration loop exited early via
	// breakLoopIfEmpty; finalization still ran afterward.
	ExitReasonLoopBroken ExitReason = "loop_broken"
	// ExitReasonUserQuit means the user confirmed quit (q+y) during any phase.
	ExitReasonUserQuit ExitReason = "user_quit"
)
