package scripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func projectCardPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "scripts", "project_card")
}

// runProjectCard runs scripts/project_card in workDir using the provided environment.
// Returns stdout and exit code.
func runProjectCard(t *testing.T, workDir string, env []string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command("bash", projectCardPath(t))
	cmd.Dir = workDir
	cmd.Env = env
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	err := cmd.Run()
	stdout = outBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// envReplaceOrAddPath returns the current environment with PATH replaced by newPath.
func envReplaceOrAddPath(newPath string) []string {
	result := []string{"PATH=" + newPath}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PATH=") {
			result = append(result, e)
		}
	}
	return result
}

// writeFile creates a file in dir with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TP-001 case 1: empty directory → exit 0, stdout is empty.
func TestProjectCard_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// TP-001 case 2: Go only → Language, Module, Test, Build lines.
func TestProjectCard_GoOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module foo\n\ngo 1.21\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	wantLines := []string{
		"Language: Go",
		"Module: foo",
		"Test: go test ./...",
		"Build: go build ./...",
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != len(wantLines) {
		t.Fatalf("output lines = %d, want %d\ngot:  %q\nwant: %v", len(lines), len(wantLines), stdout, wantLines)
	}
	for i, want := range wantLines {
		if lines[i] != want {
			t.Errorf("line[%d]: got %q, want %q", i, lines[i], want)
		}
	}
}

// TP-001 case 3: Makefile with recognized targets → Make targets line, no extraneous tokens.
func TestProjectCard_MakefileWithRecognizedTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "test:\n\tgo test ./...\nbuild:\n\tgo build ./...\nfmt:\n\tgofmt -w .\n\textra-indented: nonsense\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "Make targets:") {
		t.Fatalf("stdout missing 'Make targets:': %q", stdout)
	}
	for _, target := range []string{"test", "build", "fmt"} {
		if !strings.Contains(stdout, target) {
			t.Errorf("stdout missing target %q: %q", target, stdout)
		}
	}
	if strings.Contains(stdout, "extra-indented") {
		t.Errorf("stdout contains 'extra-indented' (indented rule mis-parsed as target): %q", stdout)
	}
}

// TP-001 case 4: Makefile with zero recognized targets → no "Make targets:" line emitted.
func TestProjectCard_MakefileNoRecognizedTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "all:\n\techo hello\ninstall:\n\techo install\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if strings.Contains(stdout, "Make targets:") {
		t.Errorf("stdout contains 'Make targets:' but no recognized targets exist: %q", stdout)
	}
}

// TP-001 case 5: Node.js with jq present → test/build/lint lines populated.
func TestProjectCard_NodeWithJq(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available in PATH")
	}
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"my-app","scripts":{"test":"jest","build":"webpack","lint":"eslint ."}}`)

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	checks := []string{"Language: Node.js", "Test: npm run test", "Build: npm run build", "Lint: npm run lint"}
	for _, want := range checks {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

// TP-001 case 6 / TP-009: Node.js with jq absent → "Language: Node.js" only, no script lines.
// This also pins the set-e + no-pipefail contract: the jq-absent branch exits 0 silently.
func TestProjectCard_NodeWithoutJq(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"app","scripts":{"test":"jest"}}`)

	// Empty PATH: jq not found. Only bash builtins are needed for the package.json
	// section when jq is absent (echo, command, [).
	stdout, exitCode := runProjectCard(t, dir, envReplaceOrAddPath(""))
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "Language: Node.js") {
		t.Errorf("stdout missing 'Language: Node.js': %q", stdout)
	}
	if strings.Contains(stdout, "Test:") || strings.Contains(stdout, "Build:") || strings.Contains(stdout, "Lint:") {
		t.Errorf("stdout contains script lines but jq is absent: %q", stdout)
	}
}

// TP-001 case 7a: pyproject.toml with [tool.poetry] → poetry package manager + test command.
func TestProjectCard_PyprojectPoetry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[tool.poetry]\nname = \"my-lib\"\n[build-system]\nrequires = [\"poetry-core\"]\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	checks := []string{"Language: Python", "Package manager: poetry", "Test: poetry run pytest"}
	for _, want := range checks {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

// TP-001 case 7b: pyproject.toml with [tool.hatch] → hatch package manager.
func TestProjectCard_PyprojectHatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[tool.hatch]\nname = \"my-lib\"\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "Language: Python") {
		t.Errorf("stdout missing 'Language: Python': %q", stdout)
	}
	if !strings.Contains(stdout, "Package manager: hatch") {
		t.Errorf("stdout missing 'Package manager: hatch': %q", stdout)
	}
}

// TP-001 case 7c: pyproject.toml with neither poetry nor hatch → pip (pyproject.toml).
func TestProjectCard_PyprojectPip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[build-system]\nrequires = [\"setuptools\"]\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "Language: Python") {
		t.Errorf("stdout missing 'Language: Python': %q", stdout)
	}
	if !strings.Contains(stdout, "Package manager: pip (pyproject.toml)") {
		t.Errorf("stdout missing 'Package manager: pip (pyproject.toml)': %q", stdout)
	}
}

// TP-001 case 8: setup.py (no pyproject.toml) → pip (setup.py) + python -m pytest.
func TestProjectCard_SetupPy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "setup.py", "from setuptools import setup\nsetup(name='foo')\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	checks := []string{"Language: Python", "Package manager: pip (setup.py)", "Test: python -m pytest"}
	for _, want := range checks {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

// TP-001 case 9: Cargo.toml → Rust, Crate, Build, Test, Lint lines.
func TestProjectCard_CargoToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"my-crate\"\nversion = \"0.1.0\"\n")

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	checks := []string{"Language: Rust", "Crate: my-crate", "Build: cargo build", "Test: cargo test", "Lint: cargo clippy"}
	for _, want := range checks {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

// TP-001 case 10: Polyglot (Go + Makefile + package.json) → all three sections emit.
func TestProjectCard_Polyglot(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available in PATH")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module mymodule\n\ngo 1.21\n")
	writeFile(t, dir, "Makefile", "test:\n\tgo test ./...\nbuild:\n\tgo build ./...\n")
	writeFile(t, dir, "package.json", `{"name":"ui","scripts":{"test":"jest"}}`)

	stdout, exitCode := runProjectCard(t, dir, os.Environ())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	checks := []string{"Language: Go", "Module: mymodule", "Make targets:", "Language: Node.js"}
	for _, want := range checks {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}
