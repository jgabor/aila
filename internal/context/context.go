package context

import (
	"fmt"
	"strings"
)

// SourceKind identifies the origin family for a context reference.
type SourceKind string

const (
	SourcePrompt         SourceKind = "prompt"
	SourceToolResult     SourceKind = "tool_result"
	SourceDiff           SourceKind = "diff"
	SourceCommand        SourceKind = "command"
	SourceCommandStdout  SourceKind = "command_stdout"
	SourceCommandStderr  SourceKind = "command_stderr"
	SourceCommandFailure SourceKind = "command_failure"
	SourceUserConstraint SourceKind = "user_constraint"
)

// SourceRef preserves exact evidence for context that may be summarized later.
type SourceRef struct {
	ID        string
	Kind      SourceKind
	Label     string
	Path      string
	LineStart int
	LineEnd   int
	Command   string
	Stream    string
	Excerpt   string
}

// ContextBlock is one assembled context input with references to exact evidence.
type ContextBlock struct {
	ID           string
	Kind         string
	Title        string
	Text         string
	SourceRefIDs []string
}

// SourceBackedClaim is visible summary text plus the refs that support it.
type SourceBackedClaim struct {
	Text         string
	SourceRefIDs []string
}

// ContextBudget is a deterministic meter for context assembled from inputs.
type ContextBudget struct {
	MaxBytes       int
	UsedBytes      int
	BlockCount     int
	SourceRefCount int
	ClaimCount     int
	Truncated      bool
}

// BuiltContext is structured, compactable context with source refs intact.
type BuiltContext struct {
	Blocks     []ContextBlock
	SourceRefs []SourceRef
	Claims     []SourceBackedClaim
	Budget     ContextBudget
	Warnings   []string
}

// BuildInput collects the source families that can feed model context.
type BuildInput struct {
	Prompts         []PromptInput
	ToolResults     []ToolResultInput
	Diffs           []DiffInput
	Commands        []CommandOutputInput
	UserConstraints []UserConstraintInput
	MaxBytes        int
}

// PromptInput records one user prompt as context evidence.
type PromptInput struct {
	Text string
}

// ToolResultInput records one tool result summary with exact supporting refs.
type ToolResultInput struct {
	ToolName   string
	Status     string
	Summary    string
	ExactLines []string
	SourceRefs []SourceRef
}

// DiffInput records one diff or hunk summary with exact path evidence.
type DiffInput struct {
	Path       string
	Summary    string
	HunkLines  []string
	SourceRefs []SourceRef
}

// CommandOutputInput records one command result suitable for deterministic summarization.
type CommandOutputInput struct {
	Command         string
	Status          string
	ExitCode        int
	StdoutLines     []string
	StderrLines     []string
	StdoutTruncated bool
	StderrTruncated bool
	ErrorKind       string
	ErrorMessage    string
}

// UserConstraintInput records an exact user constraint.
type UserConstraintInput struct {
	Text string
}

// Build assembles context without reading files, executing commands, or calling providers.
func Build(input BuildInput) BuiltContext {
	builder := contextBuilder{maxBytes: input.MaxBytes, refIDs: map[string]int{}}
	builder.addPrompts(input.Prompts)
	builder.addToolResults(input.ToolResults)
	builder.addDiffs(input.Diffs)
	builder.addCommands(input.Commands)
	builder.addUserConstraints(input.UserConstraints)
	builder.finishBudget()
	return builder.context
}

// MeterLabel returns a compact user-facing context meter.
func (built BuiltContext) MeterLabel() string {
	label := fmt.Sprintf("%d blocks / %d refs / %d bytes", built.Budget.BlockCount, built.Budget.SourceRefCount, built.Budget.UsedBytes)
	if built.Budget.MaxBytes > 0 {
		label += fmt.Sprintf(" of %d", built.Budget.MaxBytes)
	}
	if built.Budget.Truncated {
		label += " truncated"
	}
	return label
}

type contextBuilder struct {
	context  BuiltContext
	maxBytes int
	refIDs   map[string]int
}

func (builder *contextBuilder) addPrompts(prompts []PromptInput) {
	for _, prompt := range prompts {
		text := cleanText(prompt.Text)
		if text == "" {
			continue
		}
		refID := builder.addRef(SourceRef{
			ID:      fmt.Sprintf("prompt-%d", len(builder.context.Blocks)+1),
			Kind:    SourcePrompt,
			Label:   "user prompt",
			Excerpt: text,
		})
		builder.addBlock("prompt", "User prompt", text, []string{refID})
	}
}

func (builder *contextBuilder) addToolResults(results []ToolResultInput) {
	for index, result := range results {
		summary := cleanText(result.Summary)
		lines := cleanLines(result.ExactLines)
		if summary == "" && len(lines) == 0 {
			continue
		}
		refIDs := builder.addRefsOrFallback(result.SourceRefs, SourceRef{
			ID:      fmt.Sprintf("tool-%d", index+1),
			Kind:    SourceToolResult,
			Label:   joinNonEmpty(result.ToolName, result.Status),
			Excerpt: firstNonEmpty(summary, strings.Join(lines, "\n")),
		})
		text := joinSections(summary, lines)
		builder.addBlock("tool_result", titleWithFallback(result.ToolName, "Tool result"), text, refIDs)
		if summary != "" {
			builder.addClaim(summary, refIDs)
		}
	}
}

func (builder *contextBuilder) addDiffs(diffs []DiffInput) {
	for index, diff := range diffs {
		summary := cleanText(diff.Summary)
		lines := cleanLines(diff.HunkLines)
		if diff.Path == "" && summary == "" && len(lines) == 0 {
			continue
		}
		refIDs := builder.addRefsOrFallback(diff.SourceRefs, SourceRef{
			ID:      fmt.Sprintf("diff-%d", index+1),
			Kind:    SourceDiff,
			Label:   firstNonEmpty(diff.Path, "workspace diff"),
			Path:    diff.Path,
			Excerpt: firstNonEmpty(summary, strings.Join(lines, "\n")),
		})
		text := joinSections(summary, lines)
		if text == "" {
			text = diff.Path
		}
		builder.addBlock("diff", titleWithFallback(diff.Path, "Diff"), text, refIDs)
		if summary != "" {
			builder.addClaim(summary, refIDs)
		}
	}
}

func (builder *contextBuilder) addCommands(commands []CommandOutputInput) {
	for index, command := range commands {
		commandText := cleanText(command.Command)
		if commandText == "" {
			continue
		}
		commandRef := builder.addRef(SourceRef{
			ID:      fmt.Sprintf("command-%d", index+1),
			Kind:    SourceCommand,
			Label:   "command",
			Command: commandText,
			Excerpt: commandText,
		})
		refIDs := []string{commandRef}
		stdout := cleanLines(command.StdoutLines)
		for lineIndex, line := range stdout {
			refIDs = append(refIDs, builder.addRef(SourceRef{
				ID:      fmt.Sprintf("command-%d-stdout-%d", index+1, lineIndex+1),
				Kind:    SourceCommandStdout,
				Label:   "stdout",
				Command: commandText,
				Stream:  "stdout",
				Excerpt: line,
			}))
		}
		stderr := cleanLines(command.StderrLines)
		for lineIndex, line := range stderr {
			refIDs = append(refIDs, builder.addRef(SourceRef{
				ID:      fmt.Sprintf("command-%d-stderr-%d", index+1, lineIndex+1),
				Kind:    SourceCommandStderr,
				Label:   "stderr",
				Command: commandText,
				Stream:  "stderr",
				Excerpt: line,
			}))
		}
		if command.ErrorKind != "" || command.ErrorMessage != "" {
			refIDs = append(refIDs, builder.addRef(SourceRef{
				ID:      fmt.Sprintf("command-%d-failure", index+1),
				Kind:    SourceCommandFailure,
				Label:   firstNonEmpty(command.ErrorKind, "command failure"),
				Command: commandText,
				Excerpt: firstNonEmpty(command.ErrorMessage, command.ErrorKind),
			}))
		}
		status := firstNonEmpty(command.Status, "completed")
		summary := fmt.Sprintf("command %s %s exit %d", commandText, status, command.ExitCode)
		if command.ErrorKind != "" && command.ErrorMessage != "" {
			summary = fmt.Sprintf("command %s failed: %s: %s", commandText, command.ErrorKind, command.ErrorMessage)
		}
		text := joinSections(summary, appendPrefixed("stdout: ", stdout), appendPrefixed("stderr: ", stderr))
		builder.addBlock("command_output", "Summarized shell output", text, refIDs)
		builder.addClaim(summary, refIDs)
		if command.StdoutTruncated || command.StderrTruncated {
			builder.context.Warnings = append(builder.context.Warnings, "command output was truncated: "+commandText)
		}
	}
}

func (builder *contextBuilder) addUserConstraints(constraints []UserConstraintInput) {
	for _, constraint := range constraints {
		text := cleanText(constraint.Text)
		if text == "" {
			continue
		}
		refID := builder.addRef(SourceRef{
			ID:      fmt.Sprintf("constraint-%d", len(builder.context.Blocks)+1),
			Kind:    SourceUserConstraint,
			Label:   "user constraint",
			Excerpt: text,
		})
		builder.addBlock("user_constraint", "User constraint", text, []string{refID})
		builder.addClaim("user constraint: "+text, []string{refID})
	}
}

func (builder *contextBuilder) addBlock(kind string, title string, text string, refIDs []string) {
	text = cleanText(text)
	if text == "" {
		return
	}
	id := fmt.Sprintf("block-%d", len(builder.context.Blocks)+1)
	builder.context.Blocks = append(builder.context.Blocks, ContextBlock{
		ID:           id,
		Kind:         kind,
		Title:        titleWithFallback(title, kind),
		Text:         text,
		SourceRefIDs: uniqueNonEmpty(refIDs),
	})
	builder.context.Budget.UsedBytes += len(text)
}

func (builder *contextBuilder) addClaim(text string, refIDs []string) {
	text = cleanText(text)
	if text == "" {
		return
	}
	builder.context.Claims = append(builder.context.Claims, SourceBackedClaim{
		Text:         text,
		SourceRefIDs: uniqueNonEmpty(refIDs),
	})
}

func (builder *contextBuilder) addRefsOrFallback(refs []SourceRef, fallback SourceRef) []string {
	var ids []string
	for _, ref := range refs {
		id := builder.addRef(ref)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, builder.addRef(fallback))
	}
	return uniqueNonEmpty(ids)
}

func (builder *contextBuilder) addRef(ref SourceRef) string {
	ref.ID = cleanID(ref.ID)
	if ref.ID == "" {
		ref.ID = fmt.Sprintf("source-%d", len(builder.context.SourceRefs)+1)
	}
	ref.Label = cleanText(ref.Label)
	ref.Path = cleanText(ref.Path)
	ref.Command = cleanText(ref.Command)
	ref.Stream = cleanText(ref.Stream)
	ref.Excerpt = cleanText(ref.Excerpt)
	baseID := ref.ID
	builder.refIDs[baseID]++
	if builder.refIDs[baseID] > 1 {
		ref.ID = fmt.Sprintf("%s-%d", baseID, builder.refIDs[baseID])
	}
	builder.context.SourceRefs = append(builder.context.SourceRefs, ref)
	return ref.ID
}

func (builder *contextBuilder) finishBudget() {
	builder.context.Budget.BlockCount = len(builder.context.Blocks)
	builder.context.Budget.SourceRefCount = len(builder.context.SourceRefs)
	builder.context.Budget.ClaimCount = len(builder.context.Claims)
	builder.context.Budget.MaxBytes = builder.maxBytes
	if builder.maxBytes > 0 && builder.context.Budget.UsedBytes > builder.maxBytes {
		builder.context.Budget.Truncated = true
		builder.context.Warnings = append(builder.context.Warnings, "context exceeds requested byte budget")
	}
}

func cleanID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func cleanText(value string) string {
	return strings.TrimSpace(value)
}

func cleanLines(lines []string) []string {
	var cleaned []string
	for _, line := range lines {
		if text := strings.TrimRight(line, "\r\n"); strings.TrimSpace(text) != "" {
			cleaned = append(cleaned, text)
		}
	}
	return cleaned
}

func joinSections(summary string, groups ...[]string) string {
	var sections []string
	if summary = cleanText(summary); summary != "" {
		sections = append(sections, summary)
	}
	for _, group := range groups {
		if len(group) > 0 {
			sections = append(sections, strings.Join(group, "\n"))
		}
	}
	return strings.Join(sections, "\n")
}

func appendPrefixed(prefix string, lines []string) []string {
	prefixed := make([]string, 0, len(lines))
	for _, line := range lines {
		prefixed = append(prefixed, prefix+line)
	}
	return prefixed
}

func uniqueNonEmpty(values []string) []string {
	var unique []string
	seen := map[string]bool{}
	for _, value := range values {
		value = cleanText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = cleanText(value); value != "" {
			return value
		}
	}
	return ""
}

func joinNonEmpty(values ...string) string {
	return strings.Join(uniqueNonEmpty(values), " ")
}

func titleWithFallback(value string, fallback string) string {
	if value = cleanText(value); value != "" {
		return value
	}
	return fallback
}
