package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tools"
	"github.com/jgabor/aila/internal/tui"
)

// ShutdownError reports a clean, signal-triggered shutdown outcome with bounded diagnostics.
type ShutdownError struct {
	Diagnostics []diagnostic.Diagnostic
}

func (e ShutdownError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "aila shutdown requested"
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, string(diagnostic.Category)+": "+diagnostic.BoundedMessage)
	}
	return strings.Join(parts, "; ")
}

// NewShutdownError returns a bounded shutdown error suitable for CLI diagnostics.
func NewShutdownError(diagnostics []diagnostic.Diagnostic) ShutdownError {
	output := diagnostics
	if len(output) > MaxDebugDiagnostics {
		output = output[:MaxDebugDiagnostics]
	}
	return ShutdownError{Diagnostics: append([]diagnostic.Diagnostic(nil), output...)}
}

func runProgramWithShutdown(ctx context.Context, input io.Reader, output io.Writer, state tui.ViewState, controller *sessionController) error {
	program := tui.NewProgramWithContextStatePromptSubmitCommandRouteInterruptApprovalAndFileReference(ctx, input, output, state, controller.submitPrompt, controller.routeCommand, controller.requestInterrupt, controller.decideApproval, controller.discoverPromptFileReferences)
	shutdown := make(chan tui.TranscriptTurn, 1)
	go func() {
		<-ctx.Done()
		shutdown <- controller.requestShutdown(ctx.Err())
		program.Quit()
	}()

	if _, err := program.Run(); err != nil {
		if ctx.Err() != nil && errors.Is(err, tea.ErrProgramKilled) {
			<-shutdown
			return NewShutdownError(mergeShutdownDiagnostics(state.Diagnostics, controller.runner.model.Diagnostics))
		}
		return fmt.Errorf("run static tui: %w", err)
	}
	if ctx.Err() != nil {
		<-shutdown
		return NewShutdownError(mergeShutdownDiagnostics(state.Diagnostics, controller.runner.model.Diagnostics))
	}
	return nil
}

func newInputRunnerWithContext(ctx context.Context) *inputRunner {
	return newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		return runtime.DispatchContext(ctx, effects)
	})
}

func newInputRunnerWithReadContext(ctx context.Context, workspacePath string, autonomyLevel string) *inputRunner {
	return newInputRunnerWithDispatch(readDispatchContext(ctx, workspacePath, autonomyLevel))
}

func newInputRunnerWithReadContextAndFetchClient(ctx context.Context, workspacePath string, autonomyLevel string, fetchClient tools.FetchClient) *inputRunner {
	return newInputRunnerWithDispatch(readDispatchContextWithFetchClient(ctx, workspacePath, autonomyLevel, fetchClient))
}

func newInputRunnerWithAgentBuildContext(ctx context.Context, workspacePath string, autonomyLevel string) *inputRunner {
	return newInputRunnerWithDispatchAndAgentConfig(ctx, readDispatchContext(ctx, workspacePath, autonomyLevel), agent.FakeBuildRunner{Failure: agent.FailureMode(os.Getenv("AILA_AGENT_FAILURE"))}, "fake", "fake-build", []string{"read", "write"})
}

func readDispatchContext(ctx context.Context, workspacePath string, autonomyLevel string) runtimeDispatchFunc {
	return readDispatchContextWithFetchClient(ctx, workspacePath, autonomyLevel, http.DefaultClient)
}

func readDispatchContextWithFetchClient(ctx context.Context, workspacePath string, autonomyLevel string, fetchClient tools.FetchClient) runtimeDispatchFunc {
	return func(effects []runtime.Effect) []runtime.Message {
		if len(effects) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return []runtime.Message{runtime.CancellationMessage(diagnostic.SourceEffect, err)}
		}

		messages := make([]runtime.Message, 0, len(effects))
		for _, effect := range effects {
			switch typed := effect.(type) {
			case runtime.ReadToolEffect:
				messages = append(messages, dispatchReadEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), typed))
			case runtime.SearchToolEffect:
				messages = append(messages, dispatchSearchEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), typed))
			case runtime.BashToolEffect:
				messages = append(messages, dispatchBashEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), typed))
			case runtime.CompactContextEffect:
				messages = append(messages, dispatchCompactEffect(typed))
			case runtime.FetchToolEffect:
				messages = append(messages, dispatchFetchEffect(ctx, permission.AutonomyLevel(autonomyLevel), typed, fetchClient))
			case runtime.EditToolEffect:
				messages = append(messages, dispatchEditEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), typed))
			case runtime.WriteToolEffect:
				messages = append(messages, dispatchWriteEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), typed))
			default:
				messages = append(messages, runtime.DispatchContext(ctx, []runtime.Effect{effect})...)
			}
		}
		return messages
	}
}

func dispatchReadEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.ReadToolEffect) runtime.Message {
	operation := permission.NewReadOperation(effect.Request.Path)
	decisionRecord := permission.DecideRecord(autonomyLevel, operation)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.ReadToolCompleted{Operation: effect.Operation, Result: runtime.ReadToolResult{
			ToolName:      tools.ReadToolName,
			RequestedPath: effect.Request.Path,
			Error: runtime.ReadToolError{
				Kind:    runtime.ReadToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	validated, readErr := tools.ValidateReadRequest(workspacePath, tools.ReadRequest{
		Path:            effect.Request.Path,
		StartLine:       effect.Request.StartLine,
		LineLimit:       effect.Request.LineLimit,
		MaxPreviewBytes: effect.Request.MaxPreviewBytes,
		Source: tools.ReadSourceMetadata{
			Caller:      effect.Request.Source.Caller,
			RequestID:   effect.Request.Source.RequestID,
			Description: effect.Request.Source.Description,
		},
	})
	if readErr.Kind != "" {
		return runtime.ReadToolCompleted{Operation: effect.Operation, Result: runtime.ReadToolResult{
			ToolName:      tools.ReadToolName,
			RequestedPath: effect.Request.Path,
			Error: runtime.ReadToolError{
				Kind:    runtime.ReadToolErrorKind(readErr.Kind),
				Message: readErr.Message,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	result := tools.ExecuteRead(ctx, validated)
	mapped := runtimeReadResult(effect.Request.Path, result, decision)
	return runtime.ReadToolCompleted{Operation: effect.Operation, Result: mapped}
}

func runtimeReadResult(requestedPath string, result tools.ReadResult, decision runtime.ToolDecision) runtime.ReadToolResult {
	return runtime.ReadToolResult{
		ToolName:              result.ToolName,
		RequestedPath:         requestedPath,
		WorkspaceRelativePath: result.WorkspaceRelativePath,
		ResolvedPath:          result.ResolvedPath,
		ResolvedPathAvailable: result.ResolvedPathAvailable,
		RequestedRange: runtime.ReadLineRange{
			StartLine: result.RequestedRange.StartLine,
			EndLine:   result.RequestedRange.EndLine,
			Limit:     result.RequestedRange.Limit,
		},
		EffectiveRange: runtime.ReadLineRange{
			StartLine: result.EffectiveRange.StartLine,
			EndLine:   result.EffectiveRange.EndLine,
			Limit:     result.EffectiveRange.Limit,
		},
		PreviewText: result.PreviewText,
		Truncation: runtime.ReadTruncation{
			PreviewBytesLimit: result.Truncation.PreviewBytesLimit,
			PreviewTruncated:  result.Truncation.PreviewTruncated,
			LineLimitHit:      result.Truncation.LineLimitHit,
			Marker:            result.Truncation.Marker,
		},
		Error: runtime.ReadToolError{
			Kind:    runtime.ReadToolErrorKind(result.Error.Kind),
			Message: result.Error.Message,
		},
		Source: runtime.ReadSourceMetadata{
			Caller:      result.Source.Caller,
			RequestID:   result.Source.RequestID,
			Description: result.Source.Description,
		},
		Decision: decision,
	}
}

func dispatchSearchEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.SearchToolEffect) runtime.Message {
	operation := searchOperation(effect.Request)
	decisionRecord := permission.DecideRecord(autonomyLevel, operation)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.SearchToolCompleted{Operation: effect.Operation, Result: runtime.SearchToolResult{
			ToolName: string(effect.Request.ToolName),
			Pattern:  effect.Request.Pattern,
			Query:    effect.Request.Query,
			Error: runtime.SearchToolError{
				Kind:    runtime.SearchToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	if effect.Request.ToolName == runtime.SearchToolGrep {
		validated, searchErr := tools.ValidateGrepRequest(workspacePath, tools.GrepRequest{
			Query:           effect.Request.Query,
			Regex:           effect.Request.Regex,
			IncludePattern:  effect.Request.IncludePattern,
			MaxResults:      effect.Request.MaxResults,
			MaxPreviewBytes: effect.Request.MaxPreviewBytes,
			Source:          tools.SearchSourceMetadata(effect.Request.Source),
		})
		if searchErr.Kind != "" {
			return runtime.SearchToolCompleted{Operation: effect.Operation, Result: runtimeSearchFailure(effect.Request, searchErr, decision)}
		}
		return runtime.SearchToolCompleted{Operation: effect.Operation, Result: runtimeSearchResult(tools.ExecuteGrep(ctx, validated), decision)}
	}

	validated, searchErr := tools.ValidateFindRequest(workspacePath, tools.FindRequest{
		Pattern:         effect.Request.Pattern,
		MaxResults:      effect.Request.MaxResults,
		MaxPreviewBytes: effect.Request.MaxPreviewBytes,
		Source:          tools.SearchSourceMetadata(effect.Request.Source),
	})
	if searchErr.Kind != "" {
		return runtime.SearchToolCompleted{Operation: effect.Operation, Result: runtimeSearchFailure(effect.Request, searchErr, decision)}
	}
	return runtime.SearchToolCompleted{Operation: effect.Operation, Result: runtimeSearchResult(tools.ExecuteFind(ctx, validated), decision)}
}

func searchOperation(request runtime.SearchToolRequest) permission.ProposedOperation {
	if request.ToolName == runtime.SearchToolGrep {
		return permission.NewGrepOperation(request.Query, request.IncludePattern)
	}
	return permission.NewFindOperation(request.Pattern)
}

func runtimeSearchFailure(request runtime.SearchToolRequest, err tools.SearchError, decision runtime.ToolDecision) runtime.SearchToolResult {
	return runtime.SearchToolResult{
		ToolName:       string(request.ToolName),
		Pattern:        request.Pattern,
		Query:          request.Query,
		Regex:          request.Regex,
		IncludePattern: request.IncludePattern,
		Error: runtime.SearchToolError{
			Kind:    runtime.SearchToolErrorKind(err.Kind),
			Message: err.Message,
		},
		Source:   request.Source,
		Decision: decision,
	}
}

func runtimeSearchResult(result tools.SearchResult, decision runtime.ToolDecision) runtime.SearchToolResult {
	matches := make([]runtime.SearchToolMatch, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, runtime.SearchToolMatch{Path: match.Path, LineNumber: match.LineNumber, PreviewText: match.PreviewText})
	}
	return runtime.SearchToolResult{
		ToolName:       result.ToolName,
		Pattern:        result.Pattern,
		Query:          result.Query,
		Regex:          result.Regex,
		IncludePattern: result.IncludePattern,
		Matches:        matches,
		Truncation: runtime.SearchToolTruncation{
			MaxResults:        result.Truncation.MaxResults,
			MaxPreviewBytes:   result.Truncation.MaxPreviewBytes,
			OmittedResults:    result.Truncation.OmittedResults,
			OmittedFiles:      result.Truncation.OmittedFiles,
			PreviewTruncated:  result.Truncation.PreviewTruncated,
			ResultLimitHit:    result.Truncation.ResultLimitHit,
			FileSkipCount:     result.Truncation.FileSkipCount,
			TruncationMarkers: result.Truncation.TruncationMarkers,
		},
		Error: runtime.SearchToolError{
			Kind:    runtime.SearchToolErrorKind(result.Error.Kind),
			Message: result.Error.Message,
		},
		Source:   runtime.SearchSourceMetadata(result.Source),
		Decision: decision,
	}
}

func dispatchBashEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.BashToolEffect) runtime.Message {
	validated, bashErr := tools.ValidateBashRequest(workspacePath, tools.BashRequest{
		Argv:           effect.Request.Argv,
		WorkingDir:     effect.Request.WorkingDir,
		MaxOutputBytes: effect.Request.MaxOutputBytes,
		TimeoutMillis:  effect.Request.TimeoutMillis,
		Source: tools.BashSourceMetadata{
			Caller:      effect.Request.Source.Caller,
			RequestID:   effect.Request.Source.RequestID,
			Description: effect.Request.Source.Description,
		},
	})
	if bashErr.Kind != "" {
		return runtime.BashToolCompleted{Operation: effect.Operation, Result: runtimeBashFailure(effect.Request, bashErr)}
	}

	operation := permission.NewBashInspectionOperation(validated.EffectiveArgv, validated.WorkspaceRelativeWorkDir, validated.ExpectedEffect)
	decisionRecord := permission.DecideRecord(autonomyLevel, operation)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.BashToolCompleted{Operation: effect.Operation, Result: runtime.BashToolResult{
			ToolName:                 tools.BashToolName,
			RequestedArgv:            append([]string(nil), effect.Request.Argv...),
			EffectiveArgv:            append([]string(nil), validated.EffectiveArgv...),
			WorkspaceRelativeWorkDir: validated.WorkspaceRelativeWorkDir,
			CommandFamily:            validated.CommandFamily,
			ExpectedEffect:           validated.ExpectedEffect,
			ExitCode:                 -1,
			Status:                   "denied",
			Error: runtime.BashToolError{
				Kind:    runtime.BashToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	return runtime.BashToolCompleted{Operation: effect.Operation, Result: runtimeBashResult(tools.ExecuteBash(ctx, validated), decision)}
}

func runtimeBashFailure(request runtime.BashToolRequest, err tools.BashError) runtime.BashToolResult {
	return runtime.BashToolResult{
		ToolName:      tools.BashToolName,
		RequestedArgv: append([]string(nil), request.Argv...),
		ExitCode:      -1,
		Status:        "failed",
		Error: runtime.BashToolError{
			Kind:    runtime.BashToolErrorKind(err.Kind),
			Message: err.Message,
		},
		Source: request.Source,
	}
}

func runtimeBashResult(result tools.BashResult, decision runtime.ToolDecision) runtime.BashToolResult {
	return runtime.BashToolResult{
		ToolName:                 result.ToolName,
		RequestedArgv:            append([]string(nil), result.RequestedArgv...),
		EffectiveArgv:            append([]string(nil), result.EffectiveArgv...),
		WorkspaceRelativeWorkDir: result.WorkspaceRelativeWorkDir,
		CommandFamily:            result.CommandFamily,
		ExpectedEffect:           result.ExpectedEffect,
		ExitCode:                 result.ExitCode,
		Status:                   result.Status,
		Stdout: runtime.BashToolOutput{
			Text:      result.Stdout.Text,
			Bytes:     result.Stdout.Bytes,
			Truncated: result.Stdout.Truncated,
		},
		Stderr: runtime.BashToolOutput{
			Text:      result.Stderr.Text,
			Bytes:     result.Stderr.Bytes,
			Truncated: result.Stderr.Truncated,
		},
		DurationMillis: result.DurationMillis,
		Error: runtime.BashToolError{
			Kind:    runtime.BashToolErrorKind(result.Error.Kind),
			Message: result.Error.Message,
		},
		Source:   runtime.BashSourceMetadata(result.Source),
		Decision: decision,
	}
}

func dispatchEditEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.EditToolEffect) runtime.Message {
	initial := permission.NewEditOperation(effect.Request.Path, effect.Request.TargetVersion, mutationDiffPreview(effect.Request.OldText, effect.Request.NewText), effect.Request.ExpectedEffect)
	initial.RunID = effect.Operation.ID
	initial.Capability = effect.Request.Source.Caller
	decisionRecord := permission.DecideRecord(autonomyLevel, initial)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:       tools.EditToolName,
			RequestedPath:  effect.Request.Path,
			Status:         "denied",
			ExpectedEffect: effect.Request.ExpectedEffect,
			Error: runtime.MutationToolError{
				Kind:    runtime.MutationToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	validated, editErr := tools.ValidateEditRequest(workspacePath, tools.EditRequest{
		Path:           effect.Request.Path,
		TargetVersion:  effect.Request.TargetVersion,
		OldText:        effect.Request.OldText,
		NewText:        effect.Request.NewText,
		ExpectedEffect: effect.Request.ExpectedEffect,
		Source:         tools.MutationSourceMetadata(effect.Request.Source),
	})
	if editErr.Kind != "" {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:       tools.EditToolName,
			RequestedPath:  effect.Request.Path,
			Status:         "failed",
			ExpectedEffect: effect.Request.ExpectedEffect,
			Error:          runtime.MutationToolError{Kind: runtime.MutationToolErrorKind(editErr.Kind), Message: editErr.Message},
			Source:         effect.Request.Source,
			Decision:       decision,
		}}
	}

	recheckOperation := permission.NewEditOperation(validated.WorkspaceRelativePath, validated.TargetVersion, mutationDiffPreview(validated.OldText, validated.NewText), validated.ExpectedEffect)
	recheckOperation.RunID = effect.Operation.ID
	recheckOperation.Capability = effect.Request.Source.Caller
	recheckRecord := permission.DecideRecord(autonomyLevel, recheckOperation)
	decision = runtimeToolDecision(recheckRecord)
	if !recheckRecord.Allowed {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:              tools.EditToolName,
			RequestedPath:         effect.Request.Path,
			WorkspaceRelativePath: validated.WorkspaceRelativePath,
			ResolvedPath:          validated.ResolvedPath,
			ResolvedPathAvailable: true,
			Status:                "denied",
			ExpectedEffect:        validated.ExpectedEffect,
			Error:                 runtime.MutationToolError{Kind: runtime.MutationToolErrorPermission, Message: recheckRecord.Reason},
			Source:                effect.Request.Source,
			Decision:              decision,
		}}
	}

	result := tools.ExecuteEdit(ctx, validated)
	return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtimeMutationResult(effect.Request.Path, result, decision)}
}

func dispatchWriteEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.WriteToolEffect) runtime.Message {
	initial := permission.NewWriteOperation(effect.Request.Path, effect.Request.TargetVersion, mutationWritePreview(effect.Request.Content), effect.Request.ExpectedEffect)
	initial.RunID = effect.Operation.ID
	initial.Capability = effect.Request.Source.Caller
	decisionRecord := permission.DecideRecord(autonomyLevel, initial)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:       tools.WriteToolName,
			RequestedPath:  effect.Request.Path,
			Status:         "denied",
			ExpectedEffect: effect.Request.ExpectedEffect,
			Error: runtime.MutationToolError{
				Kind:    runtime.MutationToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	validated, writeErr := tools.ValidateWriteRequest(workspacePath, tools.WriteRequest{
		Path:           effect.Request.Path,
		TargetVersion:  effect.Request.TargetVersion,
		Content:        effect.Request.Content,
		ExpectedEffect: effect.Request.ExpectedEffect,
		Source:         tools.MutationSourceMetadata(effect.Request.Source),
	})
	if writeErr.Kind != "" {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:       tools.WriteToolName,
			RequestedPath:  effect.Request.Path,
			Status:         "failed",
			ExpectedEffect: effect.Request.ExpectedEffect,
			Error:          runtime.MutationToolError{Kind: runtime.MutationToolErrorKind(writeErr.Kind), Message: writeErr.Message},
			Source:         effect.Request.Source,
			Decision:       decision,
		}}
	}

	recheckOperation := permission.NewWriteOperation(validated.WorkspaceRelativePath, validated.TargetVersion, mutationWritePreview(validated.Content), validated.ExpectedEffect)
	recheckOperation.RunID = effect.Operation.ID
	recheckOperation.Capability = effect.Request.Source.Caller
	recheckRecord := permission.DecideRecord(autonomyLevel, recheckOperation)
	decision = runtimeToolDecision(recheckRecord)
	if !recheckRecord.Allowed {
		return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtime.MutationToolResult{
			ToolName:              tools.WriteToolName,
			RequestedPath:         effect.Request.Path,
			WorkspaceRelativePath: validated.WorkspaceRelativePath,
			ResolvedPath:          validated.ResolvedPath,
			ResolvedPathAvailable: true,
			Status:                "denied",
			ExpectedEffect:        validated.ExpectedEffect,
			Error:                 runtime.MutationToolError{Kind: runtime.MutationToolErrorPermission, Message: recheckRecord.Reason},
			Source:                effect.Request.Source,
			Decision:              decision,
		}}
	}

	result := tools.ExecuteWrite(ctx, validated)
	return runtime.MutationToolCompleted{Operation: effect.Operation, Result: runtimeMutationResult(effect.Request.Path, result, decision)}
}

func runtimeMutationResult(requestedPath string, result tools.MutationResult, decision runtime.ToolDecision) runtime.MutationToolResult {
	return runtime.MutationToolResult{
		ToolName:              result.ToolName,
		RequestedPath:         requestedPath,
		WorkspaceRelativePath: result.WorkspaceRelativePath,
		ResolvedPath:          result.ResolvedPath,
		ResolvedPathAvailable: result.ResolvedPathAvailable,
		Status:                result.Status,
		ExpectedEffect:        result.ExpectedEffect,
		PreviousVersion:       result.PreviousVersion,
		NewVersion:            result.NewVersion,
		PreviousExists:        result.PreviousExists,
		BytesWritten:          result.BytesWritten,
		ReplacementCount:      result.ReplacementCount,
		Error: runtime.MutationToolError{
			Kind:    runtime.MutationToolErrorKind(result.Error.Kind),
			Message: result.Error.Message,
		},
		Source:   runtime.MutationSourceMetadata(result.Source),
		Decision: decision,
	}
}

func mutationDiffPreview(oldText string, newText string) string {
	oldLine := strings.TrimRight(oldText, "\r\n")
	newLine := strings.TrimRight(newText, "\r\n")
	if len(oldLine) > 120 {
		oldLine = oldLine[:120] + "..."
	}
	if len(newLine) > 120 {
		newLine = newLine[:120] + "..."
	}
	return "-" + oldLine + "\n+" + newLine
}

func mutationWritePreview(content string) string {
	return fmt.Sprintf("write %d bytes", len([]byte(content)))
}

func dispatchFetchEffect(ctx context.Context, autonomyLevel permission.AutonomyLevel, effect runtime.FetchToolEffect, fetchClient tools.FetchClient) runtime.Message {
	validated, fetchErr := tools.ValidateFetchRequest(tools.FetchRequest{
		URL:             effect.Request.URL,
		Method:          effect.Request.Method,
		MaxPreviewBytes: effect.Request.MaxPreviewBytes,
		TimeoutMillis:   effect.Request.TimeoutMillis,
		Source:          tools.FetchSourceMetadata(effect.Request.Source),
	})
	if fetchErr.Kind != "" {
		return runtime.FetchToolCompleted{Operation: effect.Operation, Result: runtimeFetchFailure(effect.Request, fetchErr)}
	}

	operation := permission.NewFetchOperation(validated.EffectiveURL)
	decisionRecord := permission.DecideRecord(autonomyLevel, operation)
	decision := runtimeToolDecision(decisionRecord)
	if !decisionRecord.Allowed {
		return runtime.FetchToolCompleted{Operation: effect.Operation, Result: runtime.FetchToolResult{
			ToolName:       tools.FetchToolName,
			RequestedURL:   effect.Request.URL,
			EffectiveURL:   validated.EffectiveURL,
			Method:         validated.EffectiveMethod,
			ExpectedEffect: validated.ExpectedEffect,
			Status:         "denied",
			Error: runtime.FetchToolError{
				Kind:    runtime.FetchToolErrorPermission,
				Message: decisionRecord.Reason,
			},
			Source:   effect.Request.Source,
			Decision: decision,
		}}
	}

	return runtime.FetchToolCompleted{Operation: effect.Operation, Result: runtimeFetchResult(tools.ExecuteFetchWithClient(ctx, validated, fetchClient), decision)}
}

func runtimeFetchFailure(request runtime.FetchToolRequest, err tools.FetchError) runtime.FetchToolResult {
	return runtime.FetchToolResult{
		ToolName:     tools.FetchToolName,
		RequestedURL: request.URL,
		Status:       "failed",
		Error: runtime.FetchToolError{
			Kind:    runtime.FetchToolErrorKind(err.Kind),
			Message: err.Message,
		},
		Source: request.Source,
	}
}

func runtimeFetchResult(result tools.FetchResult, decision runtime.ToolDecision) runtime.FetchToolResult {
	return runtime.FetchToolResult{
		ToolName:       result.ToolName,
		RequestedURL:   result.RequestedURL,
		EffectiveURL:   result.EffectiveURL,
		Method:         result.Method,
		ExpectedEffect: result.ExpectedEffect,
		Status:         result.Status,
		HTTPStatusCode: result.HTTPStatusCode,
		HTTPStatus:     result.HTTPStatus,
		ContentType:    result.ContentType,
		PreviewText:    result.PreviewText,
		Truncation: runtime.FetchToolTruncation{
			PreviewBytesLimit: result.Truncation.PreviewBytesLimit,
			PreviewTruncated:  result.Truncation.PreviewTruncated,
			OmittedBytesKnown: result.Truncation.OmittedBytesKnown,
			OmittedBytes:      result.Truncation.OmittedBytes,
			Marker:            result.Truncation.Marker,
		},
		DurationMillis: result.DurationMillis,
		Error: runtime.FetchToolError{
			Kind:    runtime.FetchToolErrorKind(result.Error.Kind),
			Message: result.Error.Message,
		},
		Source:   runtime.FetchSourceMetadata(result.Source),
		Decision: decision,
	}
}

func runtimeToolDecision(record permission.DecisionRecord) runtime.ToolDecision {
	return runtime.ToolDecision{
		Present:          true,
		Autonomy:         string(record.Autonomy),
		Source:           record.Source,
		Allowed:          record.Allowed,
		Automatic:        record.Automatic,
		ApprovalRequired: record.ApprovalRequired,
		Reason:           record.Reason,
		OperationKind:    string(record.OperationKind),
		Tool:             record.Tool,
		Target:           record.TargetPath,
		Command:          append([]string(nil), record.Command...),
		WorkingDir:       record.WorkingDir,
		ExpectedEffect:   record.ExpectedEffect,
		Reversible:       record.Reversible,
		RunID:            record.RunID,
		Capability:       record.Capability,
	}
}

func newInputRunnerHoldingFakeWorkWithContext(ctx context.Context) *inputRunner {
	return newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		if ctx.Err() != nil {
			return runtime.DispatchContext(ctx, effects)
		}
		return nil
	})
}

func newInputRunnerHoldingFakeWorkWithSecondInterruptResolutionContext(ctx context.Context) *inputRunner {
	interrupts := 0
	return newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		if ctx.Err() != nil {
			return runtime.DispatchContext(ctx, effects)
		}
		for _, effect := range effects {
			if _, ok := effect.(runtime.FakeInterruptEffect); ok {
				interrupts++
				if interrupts >= 2 {
					return runtime.Dispatch(effects)
				}
			}
		}
		return nil
	})
}

func signalShutdownDiagnostic(err error) diagnostic.Diagnostic {
	message := "signal-triggered shutdown requested"
	if err != nil {
		message += ": " + err.Error()
	}
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategorySignalShutdown,
		Source:           diagnostic.SourceSignal,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactRuntimeEffect,
		RecoveryAction:   diagnostic.RecoveryIgnoreForRun,
		UserInputNeeded:  false,
	})
}

func mergeShutdownDiagnostics(startup []tui.DiagnosticView, runtimeDiagnostics []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	diagnostics := make([]diagnostic.Diagnostic, 0, len(startup)+len(runtimeDiagnostics))
	for _, view := range startup {
		diagnostics = append(diagnostics, diagnosticFromView(view))
	}
	diagnostics = append(diagnostics, runtimeDiagnostics...)
	return diagnostics
}

func diagnosticFromView(view tui.DiagnosticView) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryStartup,
		Source:           diagnostic.Source(view.Source),
		Severity:         diagnostic.Severity(view.Severity),
		Message:          view.BoundedMessage,
		AffectedArtifact: diagnostic.AffectedArtifact(view.AffectedArtifact),
		RecoveryAction:   diagnostic.RecoveryAction(view.RecoveryAction),
		UserInputNeeded:  view.UserInputNeeded,
	})
}
