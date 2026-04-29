package sandbox

import (
	"fmt"
	"os"
	"sort"
)

// HostUIDGID returns the current process's UID and GID.
func HostUIDGID() (int, int) {
	return os.Getuid(), os.Getgid()
}

// BuildRunArgs constructs the full `docker run ...` argv for a claude
// step. uid/gid are inputs (pure function, no syscalls) so tests can
// assert a deterministic argv shape. envAllowlist is the union of
// BuiltinEnvAllowlist and the per-workflow env list from config.json.
// Duplicate names are emitted once (de-dup by string equality, first-seen order).
// Unset host vars (os.LookupEnv returns ok=false) are silently skipped.
// CLAUDE_CONFIG_DIR is always set to the container mount point; callers
// must NOT include it in envAllowlist.
// containerEnv entries are injected as literal -e KEY=VALUE pairs in sorted
// key order (deterministic argv). They are emitted AFTER the envAllowlist
// entries so Docker's last-wins rule means containerEnv wins on collision.
func BuildRunArgs(
	projectDir, profileDir string,
	uid, gid int,
	cidfile string,
	envAllowlist []string,
	containerEnv map[string]string,
	resumeSessionID string, // non-empty → appends --resume <id> before -p
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

	// containerEnv: literal key=value injection in sorted key order.
	// Sorted for deterministic argv (testable). Emitted after envAllowlist so
	// Docker's last-wins rule means containerEnv wins when the same name appears
	// in both (e.g. a host-passthrough var that is also given a literal value).
	if len(containerEnv) > 0 {
		keys := make([]string, 0, len(containerEnv))
		for k := range containerEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "CLAUDE_CONFIG_DIR" {
				continue
			}
			args = append(args, "-e", k+"="+containerEnv[k])
		}
	}

	args = append(args,
		ImageTag,
		"claude",
		"--permission-mode", "bypassPermissions",
		"--model", model,
	)
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}
	args = append(args,
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	)

	return args
}

// BuildInteractiveArgs constructs the `docker run -it ...` argv for an
// interactive `claude` REPL session used by `pr9k sandbox --interactive`.
// The user runs `/login` inside the REPL to write `.credentials.json` to
// the bind-mounted profile directory. No project directory is mounted —
// authentication is an auth-only operation and exposing host files is
// accidental attack surface.
func BuildInteractiveArgs(profileDir string, uid, gid int) []string {
	args := []string{
		"docker", "run",
		"-it",
		"--rm",
		"--init",
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", profileDir, ContainerProfilePath),
		"-e", "CLAUDE_CONFIG_DIR=" + ContainerProfilePath,
	}

	// Forward TERM so the inner claude REPL sees the host's terminal
	// capabilities — without it, Docker's pty defaults TERM to a bare
	// "xterm" and bracketed-paste sequences can be silently dropped.
	if _, ok := os.LookupEnv("TERM"); ok {
		args = append(args, "-e", "TERM")
	}

	args = append(args, ImageTag, "claude")
	return args
}

// BuildShellArgs constructs the `docker run -it ...` argv for an interactive
// bash session inside the sandbox, used by `pr9k sandbox shell`. The project
// directory is bind-mounted read-write at the standard workspace path so the
// user can poke around the same filesystem the workflow runner sees, and the
// profile directory is bind-mounted so `claude` is usable inside the shell.
// `--rm` ensures the container is removed when the user exits the shell.
func BuildShellArgs(projectDir, profileDir string, uid, gid int) []string {
	args := []string{
		"docker", "run",
		"-it",
		"--rm",
		"--init",
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, ContainerRepoPath),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", profileDir, ContainerProfilePath),
		"-w", ContainerRepoPath,
		"-e", "CLAUDE_CONFIG_DIR=" + ContainerProfilePath,
	}

	// Forward TERM so bash sees the host's terminal capabilities; without
	// this Docker's pty defaults TERM to a bare "xterm" and modern terminals'
	// bracketed-paste / color handling can degrade silently.
	if _, ok := os.LookupEnv("TERM"); ok {
		args = append(args, "-e", "TERM")
	}

	args = append(args, ImageTag, "bash")
	return args
}
