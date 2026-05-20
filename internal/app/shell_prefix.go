package app

import (
	"strconv"
	"strings"

	ailacontext "github.com/jgabor/aila/internal/context"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

const shellPrefixSource = "shell.prefix"

func (controller *sessionController) submitShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	switch recommendation.Kind {
	case policy.ShellPrefixExecutable:
		return controller.submitExecutableShellPrefix(recommendation)
	case policy.ShellPrefixSummarized:
		return controller.submitSummarizedShellPrefix(recommendation)
	default:
		return tui.TranscriptTurn{}
	}
}

func (controller *sessionController) submitExecutableShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	turn := controller.runner.proposeBashTool(runtime.BashToolRequest{
		Argv:       shellPrefixArgv(recommendation.CommandText),
		WorkingDir: ".",
		Source: runtime.BashSourceMetadata{
			Caller:      shellPrefixSource,
			RequestID:   shellPrefixRequestID(recommendation),
			Description: "user shell prefix command",
		},
	})
	turn.UserText = recommendation.ExactInput
	turn.AssistantText = formatCommandOutput(turn.Command)
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	diagnostics := controller.persistShellPrefixHistory(recommendation, turn)
	turn.Diagnostics = append(turn.Diagnostics, diagnostics...)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, turn.Diagnostics)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) submitSummarizedShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	turn := controller.runner.proposeBashTool(runtime.BashToolRequest{
		Argv:       shellPrefixArgv(recommendation.CommandText),
		WorkingDir: ".",
		Source: runtime.BashSourceMetadata{
			Caller:      shellPrefixSource,
			RequestID:   shellPrefixRequestID(recommendation),
			Description: "user summarized shell prefix command",
		},
	})
	turn.UserText = recommendation.ExactInput
	turn.AssistantText = formatCommandOutput(turn.Command)
	if turn.Command != nil {
		turn.Command.CommandFamily = "summarized shell"
		turn.Command.ExpectedEffect = "summarize shell output for context with source refs"
	}
	built := buildShellPrefixContext(recommendation, turn)
	turn.Context = contextViewFromBuiltContext(built)
	turn.StatusDetail = "summarized shell output added to context with source refs"
	turn.RuntimeResult = summarizedShellRuntimeResult(turn)

	// Store context result in model.LastCompact
	controller.runner.model.LastCompact = mapBuiltContextToLastCompact(built)

	// Feed summarized output to the agent for reasoning by appending to model.Transcript
	for _, block := range built.Blocks {
		controller.runner.model.Transcript = append(controller.runner.model.Transcript, runtime.TranscriptEntry{
			Kind: "prompt",
			Text: "Context Block (" + block.Title + "):\n" + block.Text,
		})
	}

	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	diagnostics := controller.persistShellPrefixHistory(recommendation, turn)
	turn.Diagnostics = append(turn.Diagnostics, diagnostics...)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, turn.Diagnostics)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) persistShellPrefixHistory(recommendation policy.ShellPrefixRecommendation, turn tui.TranscriptTurn) []tui.DiagnosticView {
	status := "submitted"
	if turn.Command != nil && turn.Command.Status != "" {
		status = turn.Command.Status
	}
	display := "shell-prefix " + string(recommendation.Kind) + " " + status + " " + recommendation.ExactInput
	if turn.Command != nil && turn.Command.ExitCode != 0 {
		display += " exit=" + strconv.Itoa(turn.Command.ExitCode)
	}
	return controller.persistHistoryEvent(history.EventKindCommand, shellPrefixSource, shellPrefixSource, display)
}

func shellPrefixRequestID(recommendation policy.ShellPrefixRecommendation) string {
	return string(recommendation.Kind) + ":" + recommendation.CommandText
}

func shellPrefixArgv(command string) []string {
	var args []string
	var current strings.Builder
	inDoubleQuotes := false
	inSingleQuotes := false
	escaped := false

	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			if inSingleQuotes {
				current.WriteRune(r)
			} else {
				escaped = true
			}
			continue
		}

		if r == '"' {
			if inSingleQuotes {
				current.WriteRune(r)
			} else {
				inDoubleQuotes = !inDoubleQuotes
			}
			continue
		}

		if r == '\'' {
			if inDoubleQuotes {
				current.WriteRune(r)
			} else {
				inSingleQuotes = !inSingleQuotes
			}
			continue
		}

		if (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inDoubleQuotes && !inSingleQuotes {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

func buildShellPrefixContext(recommendation policy.ShellPrefixRecommendation, turn tui.TranscriptTurn) ailacontext.BuiltContext {
	command := recommendation.CommandText
	input := ailacontext.BuildInput{
		Prompts: []ailacontext.PromptInput{{Text: recommendation.ExactInput}},
		Commands: []ailacontext.CommandOutputInput{{
			Command:      command,
			Status:       "unknown",
			ExitCode:     0,
			ErrorKind:    "",
			ErrorMessage: "",
		}},
		MaxBytes: 4096,
	}
	if turn.Command != nil {
		input.Commands[0] = ailacontext.CommandOutputInput{
			Command:         strings.Join(turn.Command.Argv, " "),
			Status:          turn.Command.Status,
			ExitCode:        turn.Command.ExitCode,
			StdoutLines:     turn.Command.StdoutLines,
			StderrLines:     turn.Command.StderrLines,
			StdoutTruncated: turn.Command.StdoutTruncated,
			StderrTruncated: turn.Command.StderrTruncated,
			ErrorKind:       turn.Command.ErrorKind,
			ErrorMessage:    turn.Command.ErrorMessage,
		}
		if strings.TrimSpace(input.Commands[0].Command) == "" {
			input.Commands[0].Command = command
		}
	}
	return ailacontext.Build(input)
}

func contextViewFromBuiltContext(built ailacontext.BuiltContext) *tui.ContextView {
	view := &tui.ContextView{
		Source:   "app.context",
		Status:   "ready",
		Meter:    built.MeterLabel(),
		Warnings: append([]string(nil), built.Warnings...),
	}
	view.Blocks = make([]tui.ContextBlockView, 0, len(built.Blocks))
	for _, block := range built.Blocks {
		view.Blocks = append(view.Blocks, tui.ContextBlockView{
			ID:           block.ID,
			Kind:         block.Kind,
			Title:        block.Title,
			Text:         block.Text,
			SourceRefIDs: append([]string(nil), block.SourceRefIDs...),
		})
	}
	view.Claims = make([]tui.ContextClaimView, 0, len(built.Claims))
	for _, claim := range built.Claims {
		view.Claims = append(view.Claims, tui.ContextClaimView{Text: claim.Text, SourceRefIDs: append([]string(nil), claim.SourceRefIDs...)})
	}
	view.SourceRefs = make([]tui.ContextSourceRefView, 0, len(built.SourceRefs))
	for _, ref := range built.SourceRefs {
		view.SourceRefs = append(view.SourceRefs, tui.ContextSourceRefView{
			ID:        ref.ID,
			Kind:      string(ref.Kind),
			Label:     ref.Label,
			Path:      ref.Path,
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   ref.Command,
			Stream:    ref.Stream,
			Excerpt:   ref.Excerpt,
		})
	}
	return view
}

func summarizedShellRuntimeResult(turn tui.TranscriptTurn) string {
	if turn.Command == nil {
		return "summarized shell output added to context"
	}
	command := strings.Join(turn.Command.Argv, " ")
	if command == "" {
		command = "requested command"
	}
	if turn.Command.ErrorKind != "" && turn.Command.ErrorKind != string(runtime.BashToolErrorNone) {
		return "summarized shell command " + command + " failed with source refs"
	}
	return "summarized shell command " + command + " added to context with source refs"
}

func formatCommandOutput(command *tui.CommandView) string {
	if command == nil {
		return ""
	}
	var output strings.Builder
	if len(command.StdoutLines) > 0 {
		output.WriteString(strings.Join(command.StdoutLines, "\n"))
	}
	if len(command.StderrLines) > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString(strings.Join(command.StderrLines, "\n"))
	}
	if command.ErrorMessage != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("error: " + command.ErrorMessage)
	}
	return output.String()
}

func mapBuiltContextToLastCompact(built ailacontext.BuiltContext) runtime.CompactContextResult {
	res := runtime.CompactContextResult{
		Status:  "completed",
		Caveats: append([]string(nil), built.Warnings...),
		OriginalBudget: runtime.CompactContextBudget{
			MaxBytes:       built.Budget.MaxBytes,
			UsedBytes:      built.Budget.UsedBytes,
			BlockCount:     built.Budget.BlockCount,
			SourceRefCount: built.Budget.SourceRefCount,
			ClaimCount:     built.Budget.ClaimCount,
			Truncated:      built.Budget.Truncated,
		},
		Budget: runtime.CompactContextBudget{
			MaxBytes:       built.Budget.MaxBytes,
			UsedBytes:      built.Budget.UsedBytes,
			BlockCount:     built.Budget.BlockCount,
			SourceRefCount: built.Budget.SourceRefCount,
			ClaimCount:     built.Budget.ClaimCount,
			Truncated:      built.Budget.Truncated,
		},
	}

	var summaryBuilder strings.Builder
	for i, block := range built.Blocks {
		if i > 0 {
			summaryBuilder.WriteString("\n\n")
		}
		summaryBuilder.WriteString(block.Text)
	}
	res.Summary = summaryBuilder.String()

	res.Blocks = make([]runtime.CompactContextBlock, 0, len(built.Blocks))
	for _, block := range built.Blocks {
		res.Blocks = append(res.Blocks, runtime.CompactContextBlock{
			ID:           block.ID,
			Kind:         block.Kind,
			Title:        block.Title,
			Text:         block.Text,
			SourceRefIDs: append([]string(nil), block.SourceRefIDs...),
		})
	}

	res.Claims = make([]runtime.CompactContextClaim, 0, len(built.Claims))
	for _, claim := range built.Claims {
		res.Claims = append(res.Claims, runtime.CompactContextClaim{
			Text:         claim.Text,
			SourceRefIDs: append([]string(nil), claim.SourceRefIDs...),
		})
	}

	res.SourceRefs = make([]runtime.CompactSourceRef, 0, len(built.SourceRefs))
	for _, ref := range built.SourceRefs {
		res.SourceRefs = append(res.SourceRefs, runtime.CompactSourceRef{
			ID:        ref.ID,
			Kind:      string(ref.Kind),
			Label:     ref.Label,
			Path:      ref.Path,
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   ref.Command,
			Stream:    ref.Stream,
			Excerpt:   ref.Excerpt,
		})
	}

	return res
}
