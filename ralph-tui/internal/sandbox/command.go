package sandbox

import (
	"fmt"
	"os"
)

// HostUIDGID returns the current process's UID and GID.
func HostUIDGID() (int, int) {
	return os.Getuid(), os.Getgid()
}

// BuildRunArgs constructs the full `docker run ...` argv for a claude
// step. uid/gid are inputs (pure function, no syscalls) so tests can
// assert a deterministic argv shape. envAllowlist is the union of
// BuiltinEnvAllowlist and the per-workflow env list from ralph-steps.json.
// Duplicate names are emitted once (de-dup by string equality, first-seen order).
// Unset host vars (os.LookupEnv returns ok=false) are silently skipped.
// CLAUDE_CONFIG_DIR is always set to the container mount point; callers
// must NOT include it in envAllowlist.
func BuildRunArgs(
	projectDir, profileDir string,
	uid, gid int,
	cidfile string,
	envAllowlist []string,
	model, prompt string,
) []string {
	args := []string{
		"docker", "run",
		"--rm",
		"-i",
		"--init",
		"--cidfile", cidfile,
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, ContainerRepoPath),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", profileDir, ContainerProfilePath),
		"-w", ContainerRepoPath,
		"-e", "CLAUDE_CONFIG_DIR=" + ContainerProfilePath,
	}

	// De-duplicate envAllowlist preserving first-seen order.
	seen := make(map[string]bool)
	for _, name := range envAllowlist {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := os.LookupEnv(name); ok {
			args = append(args, "-e", name)
		}
	}

	args = append(args,
		ImageTag,
		"claude",
		"--permission-mode", "bypassPermissions",
		"--model", model,
		"-p", prompt,
	)

	return args
}
