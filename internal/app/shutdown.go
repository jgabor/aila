package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
	program := tui.NewProgramWithContextStatePromptSubmitCommandRouteAndInterrupt(ctx, input, output, state, controller.submitPrompt, controller.routeCommand, controller.requestInterrupt)
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

func readDispatchContext(ctx context.Context, workspacePath string, autonomyLevel string) runtimeDispatchFunc {
	return func(effects []runtime.Effect) []runtime.Message {
		if len(effects) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return []runtime.Message{runtime.CancellationMessage(diagnostic.SourceEffect, err)}
		}

		messages := make([]runtime.Message, 0, len(effects))
		for _, effect := range effects {
			readEffect, ok := effect.(runtime.ReadToolEffect)
			if !ok {
				messages = append(messages, runtime.DispatchContext(ctx, []runtime.Effect{effect})...)
				continue
			}
			messages = append(messages, dispatchReadEffect(ctx, workspacePath, permission.AutonomyLevel(autonomyLevel), readEffect))
		}
		return messages
	}
}

func dispatchReadEffect(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, effect runtime.ReadToolEffect) runtime.Message {
	operation := permission.NewReadOperation(effect.Request.Path)
	decision := permission.Decide(autonomyLevel, operation)
	if !decision.Allowed {
		return runtime.ReadToolCompleted{Operation: effect.Operation, Result: runtime.ReadToolResult{
			ToolName:      tools.ReadToolName,
			RequestedPath: effect.Request.Path,
			Error: runtime.ReadToolError{
				Kind:    runtime.ReadToolErrorPermission,
				Message: decision.Reason,
			},
			Source: effect.Request.Source,
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
			Source: effect.Request.Source,
		}}
	}

	result := tools.ExecuteRead(ctx, validated)
	mapped := runtimeReadResult(effect.Request.Path, result)
	return runtime.ReadToolCompleted{Operation: effect.Operation, Result: mapped}
}

func runtimeReadResult(requestedPath string, result tools.ReadResult) runtime.ReadToolResult {
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
