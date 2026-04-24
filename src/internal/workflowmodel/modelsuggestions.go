package workflowmodel

// DefaultScaffoldModel is the model used for the placeholder step created by Empty().
// It is the first entry in ModelSuggestions and is kept in sync with that list.
const DefaultScaffoldModel = "claude-sonnet-4-6"

// ModelSuggestions is a D58 hardcoded snapshot of recommended Claude model names
// for the TUI's model selection. This list may go stale between pr9k releases;
// any model name is valid in config.json — this list is a convenience hint only.
var ModelSuggestions = []string{
	"claude-sonnet-4-6",
	"claude-opus-4-7",
	"claude-haiku-4-5",
}
