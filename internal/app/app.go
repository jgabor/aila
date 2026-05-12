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

	if _, err := tui.NewProgram(input, output).Run(); err != nil {
		return fmt.Errorf("run static tui: %w", err)
	}
	return nil
}
