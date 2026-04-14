package sandbox

const (
	ImageTag             = "docker/sandbox-templates:claude-code"
	ContainerRepoPath    = "/home/agent/workspace"
	ContainerProfilePath = "/home/agent/.claude"
)

// BuiltinEnvAllowlist is the sandbox-plumbing env var set ralph-tui
// always attempts to pass through (subject to the "set on host" check).
var BuiltinEnvAllowlist = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_BASE_URL",
	"HTTPS_PROXY",
	"HTTP_PROXY",
	"NO_PROXY",
}
