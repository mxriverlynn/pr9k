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
//  3. CheckCredentials(profileDir) — warnings only, not fatal; only run
//     when CheckProfileDir succeeds, so that a missing profile directory
//     produces a single clear error rather than both an error and a
//     redundant "credentials file missing" warning.
func Run(profileDir string, p Prober) Result {
	var result Result

	profileErr := CheckProfileDir(profileDir)
	if profileErr != nil {
		result.Errors = append(result.Errors, profileErr)
	}

	result.Errors = append(result.Errors, CheckDocker(p)...)

	if profileErr == nil {
		if w, err := CheckCredentials(profileDir); err != nil {
			result.Errors = append(result.Errors, err)
		} else if w != "" {
			result.Warnings = append(result.Warnings, w)
		}
	}

	return result
}
