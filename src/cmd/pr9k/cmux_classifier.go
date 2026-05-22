package main

import (
	"errors"
	"fmt"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// AbortCause constants for mid-run cmux API errors (W-5). The vocabulary
// mirrors cmuxctl/preflight.go's classifyIdentifyError and classifyDialError.
const (
	// AbortCauseCmuxAccessDenied is set when a mid-run cmux call is rejected
	// with an access-denied PlaintextError (cmuxOnly mode, not a descendant).
	AbortCauseCmuxAccessDenied AbortCause = "cmux_access_denied"

	// AbortCauseCmuxAuth is set when a mid-run cmux call fails with an auth
	// error code (auth_required, auth_failed, auth_unconfigured).
	AbortCauseCmuxAuth AbortCause = "cmux_auth_error"

	// AbortCauseCmuxMethodNotFound is set when a mid-run cmux call fails with
	// method_not_found or unknown_method — a cmux build incompatibility.
	AbortCauseCmuxMethodNotFound AbortCause = "cmux_method_not_found"
)

// classifyMidRunCmuxError classifies a mid-run cmux RPC error into an
// AbortCause, reusing the vocabulary from cmuxctl/preflight.go. Recognized
// error patterns map to named causes; unrecognized *CmuxError codes produce an
// unclassified cause that preserves the raw code verbatim per
// docs/coding-standards/error-handling.md.
func classifyMidRunCmuxError(err error) AbortCause {
	// PlaintextError: cmux's non-JSON rejection (access-denied or unknown).
	var pt *cmuxctl.PlaintextError
	if errors.As(err, &pt) {
		if pt.IsAccessDenied() {
			return AbortCauseCmuxAccessDenied
		}
		return AbortCause(fmt.Sprintf("cmux_rejected: %s", pt.Raw))
	}

	// Structured CmuxError: classify on Code.
	var ce *cmuxctl.CmuxError
	if !errors.As(err, &ce) {
		return AbortCause(fmt.Sprintf("cmux_error: %s", err.Error()))
	}
	switch ce.Code {
	case "auth_required", "auth_failed", "auth_unconfigured":
		return AbortCauseCmuxAuth
	case "method_not_found", "unknown_method":
		return AbortCauseCmuxMethodNotFound
	default:
		// Unclassified: preserve the raw code verbatim.
		return AbortCause(fmt.Sprintf("cmux_error: %s", ce.Code))
	}
}
