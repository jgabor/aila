package policy

import "strings"

// ShellPrefixKind identifies Aila's fixed shell-prefix input family.
type ShellPrefixKind string

const (
	ShellPrefixNone       ShellPrefixKind = ""
	ShellPrefixExecutable ShellPrefixKind = "shell"
	ShellPrefixSummarized ShellPrefixKind = "summarized_shell"
)

// ShellPrefixRecommendation preserves the exact shell-prefix input chosen by policy.
type ShellPrefixRecommendation struct {
	Kind        ShellPrefixKind
	ExactInput  string
	CommandText string
}

// RecommendShellPrefix maps README-promised shell prefixes to closed input paths.
func RecommendShellPrefix(input string) (ShellPrefixRecommendation, bool) {
	exact := strings.TrimSpace(input)
	if !strings.HasPrefix(exact, "!") {
		return ShellPrefixRecommendation{}, false
	}
	kind := ShellPrefixExecutable
	command := strings.TrimSpace(strings.TrimPrefix(exact, "!"))
	if strings.HasPrefix(exact, "!!") {
		kind = ShellPrefixSummarized
		command = strings.TrimSpace(strings.TrimPrefix(exact, "!!"))
	}
	if command == "" {
		return ShellPrefixRecommendation{}, false
	}
	return ShellPrefixRecommendation{Kind: kind, ExactInput: exact, CommandText: command}, true
}
