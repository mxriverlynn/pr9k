package preflight

// Result holds the collected warnings and errors from a preflight run.
// All checks run before returning — no short-circuit on failure.
type Result struct {
	Warnings []string
	Errors   []error
}

// Run performs all preflight checks against profileDir using p as the
// docker prober. All errors and warnings are collected before returning.
//
// Sequence:
//  1. CheckProfileDir(profileDir)
//  2. CheckDocker(p)
//  3. CheckCredentials(profileDir) — warnings only, not fatal
func Run(profileDir string, p Prober) Result {
	var result Result

	if err := CheckProfileDir(profileDir); err != nil {
		result.Errors = append(result.Errors, err)
	}

	result.Errors = append(result.Errors, CheckDocker(p)...)

	// CheckCredentials is called even when CheckProfileDir fails. This is
	// intentionally safe: CheckCredentials treats ErrNotExist as benign, so
	// a missing parent directory simply returns no warning. If CheckCredentials
	// ever adds logic that distinguishes a missing file from a missing parent,
	// this call should be gated on CheckProfileDir succeeding.
	if w, err := CheckCredentials(profileDir); err != nil {
		result.Errors = append(result.Errors, err)
	} else if w != "" {
		result.Warnings = append(result.Warnings, w)
	}

	return result
}
