//go:build integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TP-003 (issue #145): make build produces a correct bin/.pr9k/workflow/ bundle.
// Tagged integration so it only runs when explicitly requested (go test -tags=integration).
func TestBundleLayout_MakeBuildProducesWorkflowBundle(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not on $PATH")
	}
	root := docTestRepoRoot(t)

	cmd := exec.Command("make", "build")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build failed: %v\n%s", err, out)
	}

	bundleDir := filepath.Join(root, "bin", ".pr9k", "workflow")

	// (a) config.json exists and is non-empty.
	configPath := filepath.Join(bundleDir, "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Errorf("bin/.pr9k/workflow/config.json missing: %v", err)
	} else if info.Size() == 0 {
		t.Error("bin/.pr9k/workflow/config.json is empty")
	}

	// (b) every .md in workflow/prompts/ is present under bin/.pr9k/workflow/prompts/.
	promptsSrc := filepath.Join(root, "workflow", "prompts")
	promptsDst := filepath.Join(bundleDir, "prompts")
	srcEntries, err := os.ReadDir(promptsSrc)
	if err != nil {
		t.Fatalf("ReadDir workflow/prompts: %v", err)
	}
	for _, e := range srcEntries {
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		if _, err := os.Stat(filepath.Join(promptsDst, e.Name())); err != nil {
			t.Errorf("bin/.pr9k/workflow/prompts/%s missing: %v", e.Name(), err)
		}
	}

	// (c) every file in workflow/scripts/ is present and has executable bits.
	scriptsSrc := filepath.Join(root, "workflow", "scripts")
	scriptsDst := filepath.Join(bundleDir, "scripts")
	scriptEntries, err := os.ReadDir(scriptsSrc)
	if err != nil {
		t.Fatalf("ReadDir workflow/scripts: %v", err)
	}
	for _, e := range scriptEntries {
		if e.IsDir() {
			continue
		}
		dstInfo, err := os.Stat(filepath.Join(scriptsDst, e.Name()))
		if err != nil {
			t.Errorf("bin/.pr9k/workflow/scripts/%s missing: %v", e.Name(), err)
			continue
		}
		if dstInfo.Mode()&0o111 == 0 {
			t.Errorf("bin/.pr9k/workflow/scripts/%s not executable (mode %o)", e.Name(), dstInfo.Mode())
		}
	}

	// (d) ralph-art.txt exists.
	if _, err := os.Stat(filepath.Join(bundleDir, "ralph-art.txt")); err != nil {
		t.Errorf("bin/.pr9k/workflow/ralph-art.txt missing: %v", err)
	}
}
