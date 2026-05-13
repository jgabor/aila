package app

import (
	"context"
	"fmt"
	"io"

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

	if _, err := tui.NewProgramWithStateAndPromptSubmit(input, output, state, newPromptSubmitter(FakePromptHandler{})).Run(); err != nil {
		return fmt.Errorf("run static tui: %w", err)
	}
	return nil
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

func newPromptSubmitter(handler FakePromptHandler) tui.PromptSubmitFunc {
	return func(text string) tui.TranscriptTurn {
		result := handler.Handle(PromptSubmission{Text: text})
		return tui.TranscriptTurn{
			UserText:      result.PromptText,
			AssistantText: result.AssistantText,
		}
	}
}
