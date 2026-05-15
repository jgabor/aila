package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

// Run starts Aila's static terminal shell for the current M2 product slice.
func Run(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start aila: %w", err)
	}

	state, err := initialDisplayState()
	if err != nil {
		return err
	}

	runner := newInputRunnerForEnvironment()
	if _, err := tui.NewProgramWithStatePromptSubmitAndCommandRoute(input, output, state, runner.submitPrompt, runner.routeCommand).Run(); err != nil {
		return fmt.Errorf("run static tui: %w", err)
	}
	return nil
}

func newInputRunnerForEnvironment() *inputRunner {
	if os.Getenv("AILA_FAKE_RUNTIME_HOLD_ACTIVE") == "1" {
		return newInputRunnerHoldingFakeWork()
	}
	return newInputRunner()
}

func initialDisplayState() (tui.ViewState, error) {
	config, _, err := LoadConfig()
	if err != nil {
		return tui.ViewState{}, fmt.Errorf("load startup config: %w", err)
	}
	base := tui.IdleEmptyState()
	base.Phase = workflow.PhaseIdle.DisplayLabel()
	base.PhaseSource = workflow.PhaseIdle.String()
	return NewDisplayState(base, DisplayConfigFromConfig(config)), nil
}
