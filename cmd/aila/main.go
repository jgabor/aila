package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jgabor/aila/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
