// Package validator implements D13 config validation for config.json.
// It covers all ten validation categories from the UX corrections design plan
// and returns a collected slice of structured errors — one per problem found.
// Validation runs in a single pass so all errors are visible before exit 1.
package validator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/vars"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// Severity constants for Error entries.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Error represents a single config validation finding.
// Severity is "error" (fatal, default), "warning", or "info" (non-fatal).
// Category, Phase, and StepName identify where the problem was found;
// Problem is a human-readable description of what is wrong.
type Error struct {
	Severity string // "" or SeverityError = fatal; SeverityWarning/SeverityInfo = non-fatal
	Category string
	Phase    string
	StepName string
	Problem  string
}

// IsFatal reports whether this entry represents a fatal error that should
// block startup. Entries with empty or SeverityError severity are fatal.
func (e Error) IsFatal() bool {
	return e.Severity == "" || e.Severity == SeverityError
}

// FatalErrorCount returns the number of fatal errors in errs.
func FatalErrorCount(errs []Error) int {
	n := 0
	for _, e := range errs {
		if e.IsFatal() {
			n++
		}
	}
	return n
}

// Error implements the error interface. Non-error entries use "config warning:"
// or "config info:" as the prefix so callers can distinguish them in output.
// Step-level entries include the step name; file-level entries omit it.
func (e Error) Error() string {
	prefix := "config error"
	switch e.Severity {
	case SeverityWarning:
		prefix = "config warning"
	case SeverityInfo:
		prefix = "config info"
	}
	if e.StepName == "" {
		return fmt.Sprintf("%s: %s: %s: %s", prefix, e.Category, e.Phase, e.Problem)
	}
	return fmt.Sprintf("%s: %s: %s step %q: %s", prefix, e.Category, e.Phase, e.StepName, e.Problem)
}

// vStep is the strict per-step struct used during validation.
// IsClaude uses *bool to distinguish absent (nil → error) from explicit false.
// CaptureAs uses *string to distinguish absent (nil → not set) from explicit
// empty string (pointer to "" → error).
type vStep struct {
	Name               string   `json:"name"`
	Model              string   `json:"model,omitempty"`
	PromptFile         string   `json:"promptFile,omitempty"`
	IsClaude           *bool    `json:"isClaude"`
	Command            []string `json:"command,omitempty"`
	CaptureAs          *string  `json:"captureAs,omitempty"`
	CaptureMode        *string  `json:"captureMode,omitempty"`
	BreakLoopIfEmpty   bool     `json:"breakLoopIfEmpty,omitempty"`
	SkipIfCaptureEmpty *string  `json:"skipIfCaptureEmpty,omitempty"`
	TimeoutSeconds     *int     `json:"timeoutSeconds,omitempty"`
	OnTimeout          *string  `json:"onTimeout,omitempty"`
	ResumePrevious     *bool    `json:"resumePrevious,omitempty"`
}

// vStatusLine is the strict struct used when validating the optional statusLine block.
type vStatusLine struct {
	Type                   string `json:"type,omitempty"`
	Command                string `json:"command"`
	RefreshIntervalSeconds *int   `json:"refreshIntervalSeconds,omitempty"`
}

// vFile is the strict top-level struct.
// Each phase field uses *[]vStep so that a missing key (nil) is distinguished
// from an explicitly empty array (non-nil, len 0).
// Env uses *[]string; absent key (nil) is treated as empty list. A non-array
// value (e.g. "env": "FOO") or a non-string element (e.g. [123]) will fail
// JSON decode and be reported as a "malformed JSON" parse error.
// ContainerEnv uses *map[string]string so that absent (nil) is treated as empty.
type vFile struct {
	Env          *[]string          `json:"env"`
	ContainerEnv *map[string]string `json:"containerEnv,omitempty"`
	Initialize   *[]vStep           `json:"initialize"`
	Iteration    *[]vStep           `json:"iteration"`
	Finalize     *[]vStep           `json:"finalize"`
	StatusLine   *vStatusLine       `json:"statusLine,omitempty"`
}

// envNameRe is the regex all env passthrough names must match.
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envSandboxReserved are env var names the sandbox reserves for its own use.
// Keys are the name; values are human-readable reason for rejection.
var envSandboxReserved = map[string]string{
	"CLAUDE_CONFIG_DIR": "reserved by the sandbox for its own use",
	"HOME":              "reserved by the sandbox for its own use",
}

// envDenylist are env var names that would break container isolation (F11).
var envDenylist = map[string]string{
	"PATH":                  "denylisted: would break container isolation",
	"USER":                  "denylisted: would break container isolation",
	"LOGNAME":               "denylisted: would break container isolation",
	"SSH_AUTH_SOCK":         "denylisted: would break container isolation",
	"LD_PRELOAD":            "denylisted: would break container isolation",
	"LD_LIBRARY_PATH":       "denylisted: would break container isolation",
	"DYLD_INSERT_LIBRARIES": "denylisted: would break container isolation",
	"DYLD_LIBRARY_PATH":     "denylisted: would break container isolation",
}

// reservedBuiltins is the set of built-in variable names that captureAs bindings
// must not shadow.
var reservedBuiltins = map[string]bool{
	"WORKFLOW_DIR": true,
	"PROJECT_DIR":  true,
	"MAX_ITER":     true,
	"ITER":         true,
	"STEP_NUM":     true,
	"STEP_COUNT":   true,
	"STEP_NAME":    true,
}

// Validate loads config.json from workflowDir and validates all D13
// categories. It returns all errors found; an empty slice means valid.
// Validation collects every error before returning — it does not stop at the
// first failure.
func Validate(workflowDir string) []Error {
	return ValidateDoc(workflowmodel.WorkflowDoc{}, workflowDir, nil)
}

// ValidateDoc validates a WorkflowDoc against the D13 categories.
// When workflowDir contains a config.json, that file is used for phase
// structure and the doc parameter is ignored. When config.json is absent,
// doc.Steps are treated as a flat iteration-only workflow (scaffold fallback).
//
// Companion files keyed by path relative to workflowDir (e.g.,
// "prompts/step-1.md") override on-disk reads for existence checks and token
// scanning. Keys must use the full relative path — bare filenames like
// "step-1.md" are cache misses and fall through to disk (F-121).
func ValidateDoc(doc workflowmodel.WorkflowDoc, workflowDir string, companions map[string][]byte) []Error {
	path := filepath.Join(workflowDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Scaffold fallback: no config.json — build vFile from doc.
		if len(doc.Steps) == 0 {
			return []Error{cfgErr("file", "config", "", fmt.Sprintf("could not read %s: %v", path, err))}
		}
		vf := docToVFile(doc)
		return validateVFile(vf, workflowDir, companions)
	}

	// Category 1 — parseability and no unknown fields (V6 Option A).
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var vf vFile
	if err := dec.Decode(&vf); err != nil {
		return []Error{cfgErr("parse", "config", "", fmt.Sprintf("malformed JSON in %s: %v", path, err))}
	}
	return validateVFile(vf, workflowDir, companions)
}

// docToVFile converts a flat WorkflowDoc to a vFile with all steps in the
// iteration phase. Used as a scaffold fallback when config.json is absent.
func docToVFile(doc workflowmodel.WorkflowDoc) vFile {
	empty := []vStep{}
	var iterSteps []vStep
	for _, s := range doc.Steps {
		iterSteps = append(iterSteps, stepToVStep(s))
	}
	if iterSteps == nil {
		iterSteps = empty
	}
	vf := vFile{
		Initialize: &empty,
		Iteration:  &iterSteps,
		Finalize:   &empty,
	}
	if doc.StatusLine != nil {
		ri := doc.StatusLine.RefreshIntervalSeconds
		vf.StatusLine = &vStatusLine{
			Type:                   doc.StatusLine.Type,
			Command:                doc.StatusLine.Command,
			RefreshIntervalSeconds: &ri,
		}
	}
	return vf
}

// stepToVStep converts a workflowmodel.Step to an internal vStep.
func stepToVStep(s workflowmodel.Step) vStep {
	vs := vStep{
		Name:             s.Name,
		Model:            s.Model,
		PromptFile:       s.PromptFile,
		BreakLoopIfEmpty: s.BreakLoopIfEmpty,
	}
	if len(s.Command) > 0 {
		cmd := make([]string, len(s.Command))
		copy(cmd, s.Command)
		vs.Command = cmd
	}
	if s.IsClaudeSet {
		isClaude := s.Kind == workflowmodel.StepKindClaude
		vs.IsClaude = &isClaude
	} else if s.Kind == workflowmodel.StepKindShell {
		f := false
		vs.IsClaude = &f
	}
	if s.CaptureAs != "" {
		ca := s.CaptureAs
		vs.CaptureAs = &ca
	}
	if s.CaptureMode != "" {
		cm := s.CaptureMode
		vs.CaptureMode = &cm
	}
	if s.SkipIfCaptureEmpty != "" {
		sk := s.SkipIfCaptureEmpty
		vs.SkipIfCaptureEmpty = &sk
	}
	if s.TimeoutSeconds != 0 {
		ts := s.TimeoutSeconds
		vs.TimeoutSeconds = &ts
	}
	if s.OnTimeout != "" {
		ot := s.OnTimeout
		vs.OnTimeout = &ot
	}
	if s.ResumePrevious {
		rp := true
		vs.ResumePrevious = &rp
	}
	return vs
}

// validateVFile runs all D13 validation categories against a parsed vFile.
func validateVFile(vf vFile, workflowDir string, companions map[string][]byte) []Error {
	var errs []Error

	// Category 1 — required top-level array keys.
	if vf.Initialize == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "initialize"`))
	}
	if vf.Iteration == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "iteration"`))
	}
	if vf.Finalize == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "finalize"`))
	}

	// Category 3 — iteration must have at least 1 step.
	if vf.Iteration != nil && len(*vf.Iteration) < 1 {
		errs = append(errs, cfgErr("phase-size", "iteration", "", "iteration array must have at least 1 step"))
	}

	// Category 10 — env passthrough names.
	if vf.Env != nil {
		for _, name := range *vf.Env {
			if name == "" {
				errs = append(errs, cfgErr("env", "config", "", "env name must not be empty"))
				continue
			}
			if !envNameRe.MatchString(name) {
				errs = append(errs, cfgErr("env", "config", "", fmt.Sprintf("env name %q is not a valid identifier (must match ^[A-Za-z_][A-Za-z0-9_]*$)", name)))
				continue
			}
			if reason, ok := envSandboxReserved[name]; ok {
				errs = append(errs, cfgErr("env", "config", "", fmt.Sprintf("env name %q is %s", name, reason)))
				continue
			}
			if reason, ok := envDenylist[name]; ok {
				errs = append(errs, cfgErr("env", "config", "", fmt.Sprintf("env name %q is %s", name, reason)))
				continue
			}
		}
	}

	// containerEnv validation.
	if vf.ContainerEnv != nil {
		// Build a set of names already in the env allowlist for collision detection.
		envSet := make(map[string]bool)
		if vf.Env != nil {
			for _, name := range *vf.Env {
				envSet[name] = true
			}
		}
		for key, val := range *vf.ContainerEnv {
			// Reject reserved sandbox key.
			if key == "CLAUDE_CONFIG_DIR" {
				errs = append(errs, cfgErr("containerEnv", "config", "", `containerEnv key "CLAUDE_CONFIG_DIR" is reserved by the sandbox`))
				continue
			}
			// Reject keys containing "=" (would be parsed as key=val by the shell).
			if strings.Contains(key, "=") {
				errs = append(errs, cfgErr("containerEnv", "config", "", fmt.Sprintf("containerEnv key %q must not contain '='", key)))
				continue
			}
			// Reject values containing newline or NUL.
			if strings.ContainsAny(val, "\n\x00") {
				errs = append(errs, cfgErr("containerEnv", "config", "", fmt.Sprintf("containerEnv value for key %q must not contain newline or NUL characters", key)))
				continue
			}
			// Warn when the key looks like a secret committed to the repo.
			if strings.HasSuffix(key, "_TOKEN") || strings.HasSuffix(key, "_KEY") || strings.HasSuffix(key, "_SECRET") ||
				strings.HasSuffix(key, "_PASSWORD") || strings.HasSuffix(key, "_PASSPHRASE") ||
				strings.HasSuffix(key, "_CREDENTIAL") || strings.HasSuffix(key, "_APIKEY") {
				errs = append(errs, Error{
					Severity: SeverityWarning,
					Category: "containerEnv",
					Phase:    "config",
					Problem:  fmt.Sprintf("containerEnv key %q looks like a secret; literal values in config.json are committed to the repo — consider using the env allowlist to pass it from the host instead", key),
				})
			}
			// INFO notice when key also appears in the env allowlist (containerEnv wins).
			if envSet[key] {
				errs = append(errs, Error{
					Severity: SeverityInfo,
					Category: "containerEnv",
					Phase:    "config",
					Problem:  fmt.Sprintf("containerEnv key %q also appears in env allowlist; the literal containerEnv value wins (Docker last-wins)", key),
				})
			}
		}
	}

	// statusLine validation.
	if vf.StatusLine != nil {
		sl := vf.StatusLine
		if sl.Type != "" && sl.Type != "command" {
			errs = append(errs, cfgErr("statusline", "config", "", `type must be "command" (or omitted)`))
		}
		if sl.Command == "" {
			errs = append(errs, cfgErr("statusline", "config", "", "command must not be empty"))
		} else {
			if msg := validateCommandPath(workflowDir, sl.Command); msg != "" {
				errs = append(errs, cfgErr("statusline", "config", "", msg))
			}
		}
		if sl.RefreshIntervalSeconds != nil && *sl.RefreshIntervalSeconds < 0 {
			errs = append(errs, cfgErr("statusline", "config", "", "refreshIntervalSeconds must be >= 0 (0 disables the timer)"))
		}
	}

	// Without all three phases we cannot walk variable scopes.
	if vf.Initialize == nil || vf.Iteration == nil || vf.Finalize == nil {
		return errs
	}

	// Build the initialize-phase scope: WORKFLOW_DIR, PROJECT_DIR, MAX_ITER,
	// STEP_NUM, STEP_COUNT, STEP_NAME.  ITER is deliberately excluded — it is a
	// validation error if any initialize or finalize step references it.
	initScope := map[string]bool{
		"WORKFLOW_DIR": true,
		"PROJECT_DIR":  true,
		"MAX_ITER":     true,
		"STEP_NUM":     true,
		"STEP_COUNT":   true,
		"STEP_NAME":    true,
	}

	// Validate initialize; collect captureAs names for the persistent scope.
	initCaptures := validatePhase(workflowDir, vars.Initialize, "initialize", *vf.Initialize, initScope, &errs, companions)

	// Persistent scope = initialize seeds + all captureAs from initialize.
	persistentScope := copyScope(initScope)
	for _, name := range initCaptures {
		persistentScope[name] = true
	}

	// Iteration scope = persistent + ITER.
	iterScope := copyScope(persistentScope)
	iterScope["ITER"] = true

	validatePhase(workflowDir, vars.Iteration, "iteration", *vf.Iteration, iterScope, &errs, companions)

	// Finalize scope = persistent only (no ITER, no iteration captures).
	validatePhase(workflowDir, vars.Finalize, "finalize", *vf.Finalize, persistentScope, &errs, companions)

	return errs
}

// validatePhase validates all steps in one phase and returns the captureAs names
// introduced by that phase (for persistent scope building).
func validatePhase(
	workflowDir string,
	phase vars.Phase,
	phaseName string,
	steps []vStep,
	initialScope map[string]bool,
	errs *[]Error,
	companions map[string][]byte,
) []string {
	seenNames := make(map[string]bool)
	seenCaptureAs := make(map[string]bool)
	scope := copyScope(initialScope)
	// ownCaptures tracks only captureAs names bound within this phase (not
	// inherited from initialScope). skipIfCaptureEmpty must reference one of
	// these so the runtime captureStates map — which is populated per-iteration
	// — can always resolve the source step.
	ownCaptures := make(map[string]bool)
	var captures []string

	for i, step := range steps {
		stepName := step.Name
		if stepName == "" {
			stepName = fmt.Sprintf("<unnamed step %d>", i)
		}

		at := func(category, problem string) Error {
			return cfgErr(category, phaseName, stepName, problem)
		}

		// Schema 2 — name must be non-empty.
		if step.Name == "" {
			*errs = append(*errs, at("schema", "name must not be empty"))
		}

		// Schema 6.1 — no duplicate names within the phase.
		if step.Name != "" {
			if seenNames[step.Name] {
				*errs = append(*errs, at("schema", fmt.Sprintf("duplicate step name %q in phase", step.Name)))
			}
			seenNames[step.Name] = true
		}

		// Schema 2 — isClaude is required; missing is an error (V6 Option A).
		if step.IsClaude == nil {
			*errs = append(*errs, at("schema", "isClaude is required"))
		}

		isClaude := step.IsClaude != nil && *step.IsClaude

		// Schema 2 — exactly one of {command, promptFile} must match isClaude.
		if step.IsClaude != nil {
			if isClaude {
				if step.PromptFile == "" {
					*errs = append(*errs, at("schema", "claude step must have a non-empty promptFile"))
				}
				if step.Model == "" {
					*errs = append(*errs, at("schema", "claude step must have a non-empty model"))
				}
				if len(step.Command) > 0 {
					*errs = append(*errs, at("schema", "claude step must not have command"))
				}
			} else {
				if len(step.Command) == 0 {
					*errs = append(*errs, at("schema", "non-claude step must have a non-empty command array"))
				}
				if step.PromptFile != "" {
					*errs = append(*errs, at("schema", "non-claude step must not have promptFile"))
				}
			}
		}

		// Schema 2 — captureAs: if set, must be non-empty and not shadow reserved names.
		// Schema 6.2 — no duplicate captureAs within the phase.
		if step.CaptureAs != nil {
			ca := *step.CaptureAs
			if ca == "" {
				*errs = append(*errs, at("schema", "captureAs must not be empty when set"))
			} else {
				if reservedBuiltins[ca] {
					*errs = append(*errs, at("schema", fmt.Sprintf("captureAs %q shadows reserved built-in variable", ca)))
				}
				if seenCaptureAs[ca] {
					*errs = append(*errs, at("schema", fmt.Sprintf("duplicate captureAs %q in phase", ca)))
				}
				seenCaptureAs[ca] = true
			}
		}

		// Schema 2a — captureMode: only valid on non-claude steps; value must be
		// "", "lastLine", or "fullStdout".
		if step.CaptureMode != nil {
			cm := *step.CaptureMode
			switch cm {
			case "", "lastLine", "fullStdout":
				// valid
			default:
				*errs = append(*errs, at("schema", fmt.Sprintf("captureMode %q is not valid; use \"lastLine\", \"fullStdout\", or omit the field", cm)))
			}
			if step.IsClaude != nil && *step.IsClaude {
				*errs = append(*errs, at("schema", "captureMode must not be set on claude steps"))
			}
		}

		// Schema 2b — breakLoopIfEmpty requires captureAs AND iteration phase.
		if step.BreakLoopIfEmpty {
			if step.CaptureAs == nil || *step.CaptureAs == "" {
				*errs = append(*errs, at("schema", "breakLoopIfEmpty requires captureAs to be set"))
			}
			if phase != vars.Iteration {
				*errs = append(*errs, at("schema", "breakLoopIfEmpty is only valid in the iteration phase"))
			}
		}

		// Schema 2c — skipIfCaptureEmpty must reference a capture bound by a
		// strictly earlier step in the same phase (not initialize-phase captures),
		// and is valid in the iteration and finalize phases. The runtime
		// captureStates map is populated per-phase, so initialize-phase captures
		// are never present there and the skip would silently never fire.
		if step.SkipIfCaptureEmpty != nil {
			ref := *step.SkipIfCaptureEmpty
			if ref == "" {
				*errs = append(*errs, at("schema", "skipIfCaptureEmpty must not be empty when set"))
			} else {
				if !ownCaptures[ref] {
					*errs = append(*errs, at("schema", fmt.Sprintf("skipIfCaptureEmpty %q is not bound by any earlier captureAs in this phase", ref)))
				}
				if phase != vars.Iteration && phase != vars.Finalize {
					*errs = append(*errs, at("schema", "skipIfCaptureEmpty is only valid in the iteration or finalize phase"))
				}
			}
		}

		// Schema 2d — timeoutSeconds: must be a positive integer when set, and
		// must not exceed 86400 (24 h). Values above ~9.2e9 overflow time.Duration
		// when multiplied by time.Second, causing the timer to fire immediately
		// and kill every step on start-up (SEC-001 / WARN-005).
		if step.TimeoutSeconds != nil {
			if *step.TimeoutSeconds <= 0 {
				*errs = append(*errs, at("schema", "timeoutSeconds must be a positive integer when set"))
			} else if *step.TimeoutSeconds > 86400 {
				*errs = append(*errs, at("schema", "timeoutSeconds must not exceed 86400 (24 hours)"))
			}
		}

		// Schema 2d2 — onTimeout: must be "", "fail", or "continue". Warn when set
		// without a positive timeoutSeconds (the field is inert in that case).
		if step.OnTimeout != nil {
			ot := *step.OnTimeout
			switch ot {
			case "", "fail", "continue":
				// valid
			default:
				*errs = append(*errs, at("schema", fmt.Sprintf("onTimeout %q is not valid; use \"fail\", \"continue\", or omit the field", ot)))
			}
			if ot != "" && (step.TimeoutSeconds == nil || *step.TimeoutSeconds <= 0) {
				*errs = append(*errs, Error{
					Severity: SeverityWarning,
					Category: "schema",
					Phase:    phaseName,
					StepName: stepName,
					Problem:  "onTimeout is set but timeoutSeconds is not; the field has no effect without a positive timeoutSeconds",
				})
			}
			// Warn when onTimeout=continue precedes a step with resumePrevious=true:
			// the soft-timeout leaves the current step in StepTimedOutContinuing
			// (stepStatus -> "failed"), so the next step's G2 gate will reject the
			// resume and fall through to a fresh session. Not a bug, just a gotcha
			// worth flagging at config time.
			if ot == "continue" && i+1 < len(steps) {
				next := steps[i+1]
				if next.ResumePrevious != nil && *next.ResumePrevious {
					nextName := next.Name
					if nextName == "" {
						nextName = fmt.Sprintf("<unnamed step %d>", i+1)
					}
					*errs = append(*errs, Error{
						Severity: SeverityWarning,
						Category: "schema",
						Phase:    phaseName,
						StepName: stepName,
						Problem:  fmt.Sprintf("onTimeout=\"continue\" precedes step %q which uses resumePrevious=true; on a soft-timeout path the resume will fall through G2 and start a fresh session", nextName),
					})
				}
			}
		}

		// Schema 2e — resumePrevious: only valid on claude steps. Warn when set
		// on the first step of a phase (no previous step to resume from). Warn
		// when the previous step uses a different model (cross-model chains are
		// technically supported but outside the initial same-model rollout).
		if step.ResumePrevious != nil && *step.ResumePrevious {
			if step.IsClaude != nil && !*step.IsClaude {
				*errs = append(*errs, at("schema", "resumePrevious is only valid on claude steps"))
			}
			if i == 0 {
				*errs = append(*errs, Error{
					Severity: SeverityWarning,
					Category: "schema",
					Phase:    phaseName,
					StepName: stepName,
					Problem:  "resumePrevious on the first step of a phase has no previous step to resume from; the runtime will always start a fresh session",
				})
			} else if i > 0 {
				prevModel := steps[i-1].Model
				if prevModel == "" {
					*errs = append(*errs, Error{
						Severity: SeverityWarning,
						Category: "schema",
						Phase:    phaseName,
						StepName: stepName,
						Problem:  "resumePrevious: previous step is non-claude and has no session ID; the runtime will always fall through G1 and start a fresh session",
					})
				} else if step.Model != "" && prevModel != step.Model {
					*errs = append(*errs, Error{
						Severity: SeverityWarning,
						Category: "schema",
						Phase:    phaseName,
						StepName: stepName,
						Problem:  fmt.Sprintf("resumePrevious: previous step uses model %q but this step uses model %q; cross-model resume is supported but outside the validated same-model rollout", prevModel, step.Model),
					})
				}
			}
		}

		// Category 4 — referenced files must exist.
		if step.IsClaude != nil {
			if isClaude && step.PromptFile != "" {
				absPath, pathErr := safePromptPath(workflowDir, step.PromptFile)
				if pathErr != nil {
					*errs = append(*errs, at("file", pathErr.Error()))
				} else {
					relKey := filepath.Join("prompts", step.PromptFile)
					if !statCompanionOrDisk(companions, relKey, absPath) {
						*errs = append(*errs, at("file", fmt.Sprintf("prompt file %q not found", step.PromptFile)))
					}
				}
			}
			if !isClaude && len(step.Command) > 0 {
				if msg := validateCommandPath(workflowDir, step.Command[0]); msg != "" {
					*errs = append(*errs, at("file", msg))
				}
			}
		}

		// Rule B — prompt-token ban.
		// Scan prompt files referenced by claude steps for {{WORKFLOW_DIR}} and
		// {{PROJECT_DIR}}. These tokens expand to host paths that do not exist
		// inside the sandbox. Non-claude command steps are not scanned; both
		// tokens remain valid inside command argv.
		// Uses vars.ExtractReferences to correctly skip escaped sequences such
		// as {{{{WORKFLOW_DIR}}}} which should not be flagged.
		if isClaude && step.PromptFile != "" {
			if absPath, pathErr := safePromptPath(workflowDir, step.PromptFile); pathErr == nil {
				relKey := filepath.Join("prompts", step.PromptFile)
				if data, readErr := readCompanionOrDisk(companions, relKey, absPath); readErr == nil {
					refs := vars.ExtractReferences(string(data))
					hasWorkflowDir, hasProjectDir := false, false
					for _, ref := range refs {
						if ref == "WORKFLOW_DIR" {
							hasWorkflowDir = true
						}
						if ref == "PROJECT_DIR" {
							hasProjectDir = true
						}
					}
					if hasWorkflowDir || hasProjectDir {
						var banned []string
						if hasWorkflowDir {
							banned = append(banned, "{{WORKFLOW_DIR}}")
						}
						if hasProjectDir {
							banned = append(banned, "{{PROJECT_DIR}}")
						}
						*errs = append(*errs, at("sandbox", fmt.Sprintf(
							"prompt %s: %s are not valid inside prompt files — they expand to host paths that do not exist inside the sandbox. Use paths relative to the workspace root (claude's cwd is the target repo root inside the container).",
							step.PromptFile,
							strings.Join(banned, " and "),
						)))
					}
				}
			}
		}

		// Rule C — captureAs-indirection bypass.
		// Reject any command step that BOTH references {{WORKFLOW_DIR}} or
		// {{PROJECT_DIR}} in argv AND sets captureAs. A command could capture a
		// host path into a var that a later claude prompt then uses — forwarding
		// the stale host path into the sandbox.
		if !isClaude && step.CaptureAs != nil && *step.CaptureAs != "" {
			for _, arg := range step.Command {
				if strings.Contains(arg, "{{WORKFLOW_DIR}}") || strings.Contains(arg, "{{PROJECT_DIR}}") {
					*errs = append(*errs, at("sandbox",
						"captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed: the captured host path would be forwarded to later prompt files as a stale value inside the sandbox",
					))
					break
				}
			}
		}

		// Category 5 — variable references must be in scope.
		if step.IsClaude != nil {
			refs := extractStepRefs(workflowDir, step, isClaude, companions)
			for _, ref := range refs {
				if !scope[ref] {
					*errs = append(*errs, at("variable", fmt.Sprintf("unresolved variable reference {{%s}}", ref)))
				}
			}
		}

		// Extend scope with this step's captureAs for subsequent steps.
		// Add to scope even if invalid (to reduce cascading errors), but only
		// track non-reserved first-time names in captures and ownCaptures.
		if step.CaptureAs != nil && *step.CaptureAs != "" {
			ca := *step.CaptureAs
			if !scope[ca] {
				scope[ca] = true
				ownCaptures[ca] = true
				if !reservedBuiltins[ca] {
					captures = append(captures, ca)
				}
			}
		}
	}

	return captures
}

// validateCommandPath checks that cmd (command[0]) is resolvable.
// A path containing "/" is treated as relative (resolved under workflowDir) or
// absolute. A bare name is looked up via exec.LookPath.
// OI-1: relative paths are additionally checked via EvalSymlinks + containment
// against the resolved parent directory to reject file-level symlink escapes.
func validateCommandPath(workflowDir, cmd string) string {
	// Uses "/" as path separator; assumes Unix. Revise if Windows support is added.
	if strings.Contains(cmd, "/") {
		var resolved string
		if filepath.IsAbs(cmd) {
			resolved = cmd
		} else {
			resolved = filepath.Join(workflowDir, cmd)
		}

		if filepath.IsAbs(cmd) {
			// Absolute paths: existence check only.
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Sprintf("command %q not found at %s", cmd, resolved)
			}
			return ""
		}

		// Relative paths: OI-1 EvalSymlinks + containment check.
		realCmd, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			// File does not exist.
			return fmt.Sprintf("command %q not found at %s", cmd, resolved)
		}
		// Resolve the command's immediate parent directory via symlinks.
		parentDir := filepath.Dir(resolved)
		realParent, err := filepath.EvalSymlinks(parentDir)
		if err != nil {
			realParent = parentDir
		}
		// Check that the resolved command is within its resolved parent directory.
		// realCmd == realParent cannot happen (LookPath/Abs always produces a
		// longer path), but the HasPrefix check alone makes a reader wonder.
		if !strings.HasPrefix(realCmd, realParent+string(filepath.Separator)) {
			return fmt.Sprintf("command %q escapes its directory", cmd)
		}
		return ""
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Sprintf("command %q not found in PATH", cmd)
	}
	return ""
}

// extractStepRefs returns the variable names referenced by {{VAR}} tokens in
// the step's prompt file (for claude steps) or command arguments (for non-claude
// steps). If the prompt file cannot be read, nil is returned — a missing file
// is already reported by category 4.
func extractStepRefs(workflowDir string, step vStep, isClaude bool, companions map[string][]byte) []string {
	if isClaude {
		if step.PromptFile == "" {
			return nil
		}
		absPath, err := safePromptPath(workflowDir, step.PromptFile)
		if err != nil {
			return nil
		}
		relKey := filepath.Join("prompts", step.PromptFile)
		data, err := readCompanionOrDisk(companions, relKey, absPath)
		if err != nil {
			return nil
		}
		return vars.ExtractReferences(string(data))
	}
	var refs []string
	for _, arg := range step.Command {
		refs = append(refs, vars.ExtractReferences(arg)...)
	}
	return refs
}

// safePromptPath resolves the named prompt file under workflowDir/prompts and
// returns its absolute path. It returns an error if the resolved path escapes
// the resolved prompts directory (OI-1: EvalSymlinks on both sides prevents
// file-level symlink traversal attacks while allowing directory-level symlinks
// like the test bundle pattern <tempdir>/prompts → <repo>/workflow/prompts).
func safePromptPath(workflowDir, promptFile string) (string, error) {
	promptPath := filepath.Join(workflowDir, "prompts", promptFile)
	absPath, err := filepath.Abs(promptPath)
	if err != nil {
		return "", fmt.Errorf("could not resolve prompt path: %w", err)
	}
	absPrompts, err := filepath.Abs(filepath.Join(workflowDir, "prompts"))
	if err != nil {
		return "", fmt.Errorf("could not resolve prompts directory: %w", err)
	}

	// OI-1: resolve symlinks on both sides before the containment check.
	// When the candidate file does not exist yet (EvalSymlinks returns ENOENT),
	// walk back to the nearest existing ancestor and rejoin the missing suffix
	// so directory-level symlinks (e.g. macOS /var → /private/var) are honored
	// even when the prompt file is only present in the in-memory companion map.
	resolvedPath, err := resolvePathWithWalkback(absPath)
	if err != nil {
		resolvedPath = absPath
	}
	resolvedPrompts, err := filepath.EvalSymlinks(absPrompts)
	if err != nil {
		resolvedPrompts = absPrompts
	}

	if !strings.HasPrefix(resolvedPath, resolvedPrompts+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt path escapes prompts directory: %s", promptFile)
	}
	return absPath, nil
}

// resolvePathWithWalkback resolves path via EvalSymlinks; on ENOENT walks toward
// the root until an existing ancestor is found, then appends the missing suffix
// to the ancestor's resolved form. This lets containment checks honor
// directory-level symlinks (e.g. macOS /var → /private/var) even when the
// terminal path component does not exist on disk yet.
func resolvePathWithWalkback(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	remaining := filepath.Base(path)
	dir := filepath.Dir(path)
	for {
		resolved, err = filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(resolved, remaining), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		remaining = filepath.Join(filepath.Base(dir), remaining)
		dir = parent
	}
}

// statCompanionOrDisk reports whether the file at relKey (companion map key,
// path relative to workflowDir, e.g. "prompts/step-1.md") exists. When the key
// is present in the companion map the file is considered to exist in memory.
// Otherwise os.Stat(diskPath) is used.
func statCompanionOrDisk(companions map[string][]byte, relKey, diskPath string) bool {
	if companions != nil {
		if _, ok := companions[relKey]; ok {
			return true
		}
	}
	_, err := os.Stat(diskPath)
	return err == nil
}

// readCompanionOrDisk returns the contents of the file at relKey. When the key
// is present in the companion map the in-memory bytes are returned directly.
// Otherwise os.ReadFile(diskPath) is used.
func readCompanionOrDisk(companions map[string][]byte, relKey, diskPath string) ([]byte, error) {
	if companions != nil {
		if data, ok := companions[relKey]; ok {
			return data, nil
		}
	}
	return os.ReadFile(diskPath)
}

// cfgErr constructs a validation Error.
func cfgErr(category, phase, stepName, problem string) Error {
	return Error{Category: category, Phase: phase, StepName: stepName, Problem: problem}
}

// copyScope returns a shallow copy of a scope map.
func copyScope(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	maps.Copy(dst, src)
	return dst
}
