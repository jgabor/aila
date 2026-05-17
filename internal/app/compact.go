package app

import (
	"fmt"
	"strings"

	ailacontext "github.com/jgabor/aila/internal/context"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

const compactSource = "app.compact"

func compactRequestFromView(view tui.ViewState) runtime.CompactContextRequest {
	built := builtContextFromView(view)
	request := runtimeCompactRequestFromBuiltContext(built)
	request.MaxBytes = 4096
	request.Source = runtime.CompactSourceMetadata{
		Caller:      compactSource,
		RequestID:   "manual-compact",
		Description: "manual /compact command",
	}
	return request
}

func builtContextFromView(view tui.ViewState) ailacontext.BuiltContext {
	if view.Context != nil {
		return builtContextFromContextView(view.Context)
	}
	var prompts []ailacontext.PromptInput
	var toolResults []ailacontext.ToolResultInput
	var commands []ailacontext.CommandOutputInput
	for index, turn := range view.Transcript {
		if strings.TrimSpace(turn.UserText) != "" {
			prompts = append(prompts, ailacontext.PromptInput{Text: turn.UserText})
		}
		if strings.TrimSpace(turn.AssistantText) != "" {
			toolResults = append(toolResults, ailacontext.ToolResultInput{
				ToolName: "assistant",
				Status:   "visible",
				Summary:  turn.AssistantText,
				SourceRefs: []ailacontext.SourceRef{{
					ID:      fmt.Sprintf("turn-%d-assistant", index+1),
					Kind:    ailacontext.SourceToolResult,
					Label:   "assistant transcript",
					Excerpt: turn.AssistantText,
				}},
			})
		}
		if turn.Command != nil {
			commands = append(commands, ailacontext.CommandOutputInput{
				Command:         strings.Join(turn.Command.Argv, " "),
				Status:          turn.Command.Status,
				ExitCode:        turn.Command.ExitCode,
				StdoutLines:     append([]string(nil), turn.Command.StdoutLines...),
				StderrLines:     append([]string(nil), turn.Command.StderrLines...),
				StdoutTruncated: turn.Command.StdoutTruncated,
				StderrTruncated: turn.Command.StderrTruncated,
				ErrorKind:       turn.Command.ErrorKind,
				ErrorMessage:    turn.Command.ErrorMessage,
			})
		}
	}
	return ailacontext.Build(ailacontext.BuildInput{Prompts: prompts, ToolResults: toolResults, Commands: commands})
}

func builtContextFromContextView(view *tui.ContextView) ailacontext.BuiltContext {
	if view == nil {
		return ailacontext.BuiltContext{}
	}
	built := ailacontext.BuiltContext{
		Warnings: append([]string(nil), view.Warnings...),
	}
	for _, block := range view.Blocks {
		built.Blocks = append(built.Blocks, ailacontext.ContextBlock{
			ID:           block.ID,
			Kind:         block.Kind,
			Title:        block.Title,
			Text:         block.Text,
			SourceRefIDs: append([]string(nil), block.SourceRefIDs...),
		})
		built.Budget.UsedBytes += len(block.Text)
	}
	for _, claim := range view.Claims {
		built.Claims = append(built.Claims, ailacontext.SourceBackedClaim{Text: claim.Text, SourceRefIDs: append([]string(nil), claim.SourceRefIDs...)})
	}
	for _, ref := range view.SourceRefs {
		built.SourceRefs = append(built.SourceRefs, ailacontext.SourceRef{
			ID:        ref.ID,
			Kind:      ailacontext.SourceKind(ref.Kind),
			Label:     ref.Label,
			Path:      ref.Path,
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   ref.Command,
			Stream:    ref.Stream,
			Excerpt:   ref.Excerpt,
		})
	}
	built.Budget.BlockCount = len(built.Blocks)
	built.Budget.SourceRefCount = len(built.SourceRefs)
	built.Budget.ClaimCount = len(built.Claims)
	return built
}

func dispatchCompactEffect(effect runtime.CompactContextEffect) runtime.Message {
	built := builtContextFromRuntimeCompactRequest(effect.Request)
	result := ailacontext.Compact(ailacontext.CompactInput{Context: built, MaxBytes: effect.Request.MaxBytes})
	return runtime.CompactContextCompleted{Operation: effect.Operation, Result: runtimeCompactResultFromContext(effect.Request, result)}
}

func builtContextFromRuntimeCompactRequest(request runtime.CompactContextRequest) ailacontext.BuiltContext {
	built := ailacontext.BuiltContext{Warnings: append([]string(nil), request.Warnings...)}
	for _, block := range request.Blocks {
		built.Blocks = append(built.Blocks, ailacontext.ContextBlock{
			ID:           block.ID,
			Kind:         block.Kind,
			Title:        block.Title,
			Text:         block.Text,
			SourceRefIDs: append([]string(nil), block.SourceRefIDs...),
		})
	}
	for _, claim := range request.Claims {
		built.Claims = append(built.Claims, ailacontext.SourceBackedClaim{Text: claim.Text, SourceRefIDs: append([]string(nil), claim.SourceRefIDs...)})
	}
	for _, ref := range request.SourceRefs {
		built.SourceRefs = append(built.SourceRefs, ailacontext.SourceRef{
			ID:        ref.ID,
			Kind:      ailacontext.SourceKind(ref.Kind),
			Label:     ref.Label,
			Path:      ref.Path,
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   ref.Command,
			Stream:    ref.Stream,
			Excerpt:   ref.Excerpt,
		})
	}
	built.Budget = ailacontext.ContextBudget{
		MaxBytes:       request.Budget.MaxBytes,
		UsedBytes:      request.Budget.UsedBytes,
		BlockCount:     request.Budget.BlockCount,
		SourceRefCount: request.Budget.SourceRefCount,
		ClaimCount:     request.Budget.ClaimCount,
		Truncated:      request.Budget.Truncated,
	}
	return built
}

func runtimeCompactRequestFromBuiltContext(built ailacontext.BuiltContext) runtime.CompactContextRequest {
	request := runtime.CompactContextRequest{
		Budget: runtime.CompactContextBudget{
			MaxBytes:       built.Budget.MaxBytes,
			UsedBytes:      built.Budget.UsedBytes,
			BlockCount:     built.Budget.BlockCount,
			SourceRefCount: built.Budget.SourceRefCount,
			ClaimCount:     built.Budget.ClaimCount,
			Truncated:      built.Budget.Truncated,
		},
		Warnings: append([]string(nil), built.Warnings...),
	}
	for _, block := range built.Blocks {
		request.Blocks = append(request.Blocks, runtime.CompactContextBlock{
			ID:           block.ID,
			Kind:         block.Kind,
			Title:        block.Title,
			Text:         block.Text,
			SourceRefIDs: append([]string(nil), block.SourceRefIDs...),
		})
	}
	for _, claim := range built.Claims {
		request.Claims = append(request.Claims, runtime.CompactContextClaim{Text: claim.Text, SourceRefIDs: append([]string(nil), claim.SourceRefIDs...)})
	}
	for _, ref := range built.SourceRefs {
		request.SourceRefs = append(request.SourceRefs, runtimeCompactSourceRef(ref))
	}
	return request
}

func runtimeCompactResultFromContext(request runtime.CompactContextRequest, result ailacontext.CompactResult) runtime.CompactContextResult {
	status := "completed"
	if len(result.Caveats) > 0 {
		status = "flagged"
	}
	if len(result.Context.Blocks) == 0 && len(result.Context.Claims) == 0 {
		status = "flagged"
	}
	out := runtimeCompactRequestFromBuiltContext(result.Context)
	return runtime.CompactContextResult{
		Status:         status,
		Summary:        fmt.Sprintf("manual compaction preserved %d source refs", len(result.Context.SourceRefs)),
		Blocks:         out.Blocks,
		SourceRefs:     out.SourceRefs,
		Claims:         out.Claims,
		OriginalBudget: runtimeCompactBudget(result.OriginalBudget),
		Budget:         runtimeCompactBudget(result.Context.Budget),
		Caveats:        append([]string(nil), result.Caveats...),
		Source:         request.Source,
	}
}

func runtimeCompactBudget(budget ailacontext.ContextBudget) runtime.CompactContextBudget {
	return runtime.CompactContextBudget{
		MaxBytes:       budget.MaxBytes,
		UsedBytes:      budget.UsedBytes,
		BlockCount:     budget.BlockCount,
		SourceRefCount: budget.SourceRefCount,
		ClaimCount:     budget.ClaimCount,
		Truncated:      budget.Truncated,
	}
}

func runtimeCompactSourceRef(ref ailacontext.SourceRef) runtime.CompactSourceRef {
	return runtime.CompactSourceRef{
		ID:        ref.ID,
		Kind:      string(ref.Kind),
		Label:     ref.Label,
		Path:      ref.Path,
		LineStart: ref.LineStart,
		LineEnd:   ref.LineEnd,
		Command:   ref.Command,
		Stream:    ref.Stream,
		Excerpt:   ref.Excerpt,
	}
}

func compactView(model runtime.Model) *tui.CompactView {
	if model.ActiveOperation.Kind == runtime.OperationCompact && model.Status == runtime.StatusActive {
		return &tui.CompactView{Source: compactSource, Status: "running", Summary: "manual context compaction running"}
	}
	if model.LastCompact.Status == "" && model.LastCompact.Summary == "" && len(model.LastCompact.Caveats) == 0 && len(model.LastCompact.SourceRefs) == 0 {
		return nil
	}
	return &tui.CompactView{
		Source:        defaultString(model.LastCompact.Source.Caller, compactSource),
		Status:        defaultString(model.LastCompact.Status, "completed"),
		Summary:       model.LastCompact.Summary,
		Meter:         compactMeter(model.LastCompact.Budget),
		OriginalMeter: compactMeter(model.LastCompact.OriginalBudget),
		Caveats:       append([]string(nil), model.LastCompact.Caveats...),
		SourceRefs:    compactSourceRefViews(model.LastCompact.SourceRefs),
	}
}

func compactContextView(result runtime.CompactContextResult) *tui.ContextView {
	if result.Status == "" && len(result.Blocks) == 0 && len(result.SourceRefs) == 0 {
		return nil
	}
	return contextViewFromBuiltContext(builtContextFromRuntimeCompactResult(result))
}

func builtContextFromRuntimeCompactResult(result runtime.CompactContextResult) ailacontext.BuiltContext {
	request := runtime.CompactContextRequest{
		Blocks:     result.Blocks,
		SourceRefs: result.SourceRefs,
		Claims:     result.Claims,
		Budget:     result.Budget,
	}
	built := builtContextFromRuntimeCompactRequest(request)
	built.Warnings = append(built.Warnings, result.Caveats...)
	return built
}

func compactSourceRefViews(refs []runtime.CompactSourceRef) []tui.ContextSourceRefView {
	views := make([]tui.ContextSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ContextSourceRefView{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Label:     ref.Label,
			Path:      ref.Path,
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   ref.Command,
			Stream:    ref.Stream,
			Excerpt:   ref.Excerpt,
		})
	}
	return views
}

func compactMeter(budget runtime.CompactContextBudget) string {
	label := fmt.Sprintf("%d blocks / %d refs / %d bytes", budget.BlockCount, budget.SourceRefCount, budget.UsedBytes)
	if budget.MaxBytes > 0 {
		label += fmt.Sprintf(" of %d", budget.MaxBytes)
	}
	if budget.Truncated {
		label += " truncated"
	}
	return label
}
