package app

import (
	"context"
	"fmt"
	"io"

	"github.com/jgabor/aila/internal/tui"
)

// Run starts Aila's static terminal shell for the current M2 product slice.
func Run(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start aila: %w", err)
	}

	if _, err := tui.NewProgramWithPromptSubmit(input, output, newPromptSubmitter(FakePromptHandler{})).Run(); err != nil {
		return fmt.Errorf("run static tui: %w", err)
	}
	return nil
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
