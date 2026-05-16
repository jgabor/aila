package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

// Run starts Aila's static terminal shell for the current M2 product slice.
func Run(ctx context.Context, input io.Reader, output io.Writer) error {
	return run(ctx, input, output, false)
}

// RunContinue starts Aila's terminal shell with current-session memory when available.
func RunContinue(ctx context.Context, input io.Reader, output io.Writer) error {
	return run(ctx, input, output, true)
}

func run(ctx context.Context, input io.Reader, output io.Writer, resumeCurrent bool) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start aila: %w", err)
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve startup workspace: %w", err)
	}

	state, err := initialDisplayStateWithResume(ctx, workspace, resumeCurrent)
	if err != nil {
		return err
	}

	runner := newInputRunnerForEnvironment(ctx, workspace, state.Autonomy)
	controller := newController(ctx, workspace, state, runner)
	return runProgramWithShutdown(ctx, input, output, state, controller)
}

func initialDisplayState(ctx context.Context, workspacePath string) (tui.ViewState, error) {
	return initialDisplayStateWithResume(ctx, workspacePath, false)
}

func newInputRunnerForEnvironment(ctx context.Context, workspacePath string, autonomyLevel string) *inputRunner {
	if os.Getenv("AILA_FAKE_RUNTIME_HOLD_ACTIVE") == "1" {
		if os.Getenv("AILA_FAKE_RUNTIME_RESOLVE_SECOND_INTERRUPT") == "1" {
			return newInputRunnerHoldingFakeWorkWithSecondInterruptResolutionContext(ctx)
		}
		return newInputRunnerHoldingFakeWorkWithContext(ctx)
	}
	if os.Getenv("AILA_AGENT_READONLY") == "1" {
		return newInputRunnerWithAgentReadOnlyContext(ctx, workspacePath, autonomyLevel)
	}
	return newInputRunnerWithReadContext(ctx, workspacePath, autonomyLevel)
}

func initialDisplayStateWithResume(ctx context.Context, workspacePath string, resumeCurrent bool) (tui.ViewState, error) {
	config, _, err := LoadConfig()
	if err != nil {
		return tui.ViewState{}, fmt.Errorf("load startup config: %w", err)
	}
	storeStatus := openStoreDisplayStatus(ctx, workspacePath)
	base := tui.IdleEmptyState()
	base.Phase = workflow.PhaseIdle.DisplayLabel()
	base.PhaseSource = workflow.PhaseIdle.String()
	base = NewDisplayState(base, DisplayConfigFromConfig(config))
	base = NewStoreDisplayState(base, storeStatus)
	if resumeCurrent {
		base = resumeCurrentSessionSnapshot(ctx, workspacePath, base)
	}
	return base, nil
}

func openStoreDisplayStatus(ctx context.Context, workspacePath string) StoreDisplayStatus {
	result, err := state.OpenProjectStoreWithStatus(ctx, workspacePath)
	if err != nil {
		detail := "project store unavailable: " + boundedStoreError(err)
		return StoreDisplayStatus{
			Status:      "degraded",
			Source:      "state.open",
			Detail:      detail,
			Diagnostics: []diagnostic.Diagnostic{storeOpenUnavailableDiagnostic(detail)},
		}
	}
	if result.Status.State == state.OpenStateRecoveryNeeded {
		detail := "project metadata requires recovery"
		if len(result.Status.Diagnostics) > 0 {
			detail = result.Status.Diagnostics[0].BoundedMessage
			if summary := diagnosticSummary(result.Status.Diagnostics); summary != "" {
				detail += " (" + summary + ")"
			}
		}
		return StoreDisplayStatus{
			Status:      string(result.Status.State),
			Source:      "state.open",
			Detail:      detail,
			Diagnostics: result.Status.Diagnostics,
		}
	}
	return StoreDisplayStatus{
		Status: "initialized",
		Source: "state.open",
		Detail: "project store ready",
	}
}
