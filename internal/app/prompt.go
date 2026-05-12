package app

import "strings"

// PromptSubmission is the minimal app-level message for text submitted by the TUI.
type PromptSubmission struct {
	Text string
}

// PromptResult is the minimal app-level result returned to the TUI for rendering.
type PromptResult struct {
	PromptText    string
	AssistantText string
}

// FakePromptHandler returns deterministic M4 assistant text without external IO.
type FakePromptHandler struct{}

// Handle returns the fixed fake response for a submitted prompt.
func (FakePromptHandler) Handle(submission PromptSubmission) PromptResult {
	text := strings.TrimSpace(submission.Text)
	return PromptResult{
		PromptText:    text,
		AssistantText: "Fake Aila response: " + text,
	}
}
