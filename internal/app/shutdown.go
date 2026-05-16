package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/runtime"
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
