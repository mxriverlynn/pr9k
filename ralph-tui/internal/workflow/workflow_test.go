package workflow

import (
	"testing"
)

func TestResolveCommand_ScriptPathAndIssueID(t *testing.T) {
	projectDir := "/home/user/project"
	cmd := []string{"ralph-bash/scripts/close_gh_issue", "{{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "42")
	want := []string{"/home/user/project/ralph-bash/scripts/close_gh_issue", "42"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_BareCommandPassthrough(t *testing.T) {
	projectDir := "/home/user/project"
	cmd := []string{"git", "push"}
	got := ResolveCommand(projectDir, cmd, "99")
	want := []string{"git", "push"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_MultipleIssueIDOccurrences(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"ralph-bash/scripts/foo", "{{ISSUE_ID}}", "--label={{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "7")
	want := []string{"/proj/ralph-bash/scripts/foo", "7", "--label=7"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_RelativeScriptPathResolved(t *testing.T) {
	projectDir := "/base"
	cmd := []string{"ralph-bash/scripts/foo", "arg"}
	got := ResolveCommand(projectDir, cmd, "1")
	wantExe := "/base/ralph-bash/scripts/foo"
	if got[0] != wantExe {
		t.Errorf("exe: got %q, want %q", got[0], wantExe)
	}
}

func TestResolveCommand_AbsolutePathUnchanged(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"/usr/bin/env", "{{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "3")
	if got[0] != "/usr/bin/env" {
		t.Errorf("exe: got %q, want /usr/bin/env", got[0])
	}
	if got[1] != "3" {
		t.Errorf("arg: got %q, want 3", got[1])
	}
}

func TestResolveCommand_NoTemplateVars_Passthrough(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"git", "commit", "-m", "fix things"}
	got := ResolveCommand(projectDir, cmd, "10")
	want := []string{"git", "commit", "-m", "fix things"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
