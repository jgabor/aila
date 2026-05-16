package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jgabor/aila/internal/app"
)

const version = "dev"

var cliCommands = []string{"run", "continue", "config", "models", "help"}

var cliFlags = []string{"--model", "-m", "--continue", "-c", "--version", "-V", "--debug"}

type cliRunner struct {
	input   io.Reader
	output  io.Writer
	errors  io.Writer
	version string
	start   func(context.Context, io.Reader, io.Writer) error
	resume  func(context.Context, io.Reader, io.Writer) error
	runCmd  func(context.Context, app.NonInteractiveRunRequest) (string, error)
	config  func(bool) (string, error)
	models  func(string, []string) (string, error)
	debug   func(context.Context) (string, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{
		input:   os.Stdin,
		output:  os.Stdout,
		errors:  os.Stderr,
		version: version,
		start:   app.Run,
		resume:  app.RunContinue,
		runCmd:  app.NonInteractiveRunCommandOutput,
		config:  app.ConfigCommandOutput,
		models:  app.ModelsCommandOutput,
		debug:   app.DebugDiagnosticsCommandOutput,
	}
	if code := exitCodeForError(runner.run(ctx, os.Args[1:]), runner.errors); code != 0 {
		os.Exit(code)
	}
}

func exitCodeForError(err error, errorsOut io.Writer) int {
	if err == nil {
		return 0
	}
	var shutdown app.ShutdownError
	if errors.As(err, &shutdown) {
		_, _ = fmt.Fprintf(errorsOut, "shutdown: %v\n", shutdown)
		return 0
	}
	_, _ = fmt.Fprintln(errorsOut, err)
	return 1
}

func (r cliRunner) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return r.start(ctx, r.input, r.output)
	}

	parsed, err := parseCLI(args)
	if err != nil {
		return err
	}
	if parsed.debug {
		debugOutput := r.debug
		if debugOutput == nil {
			debugOutput = app.DebugDiagnosticsCommandOutput
		}
		line, err := debugOutput(ctx)
		if err != nil {
			return fmt.Errorf("collect debug diagnostics: %w", err)
		}
		if _, err := fmt.Fprint(r.output, line); err != nil {
			_, _ = fmt.Fprintf(r.errors, "write CLI output: %v\n", err)
			return err
		}
		return nil
	}
	if parsed.command == "continue" {
		resume := r.resume
		if resume == nil {
			resume = app.RunContinue
		}
		return resume(ctx, r.input, r.output)
	}

	line := commandOutput(r.version, parsed)
	if parsed.command == "run" {
		runOutput := r.runCmd
		if runOutput == nil {
			runOutput = app.NonInteractiveRunCommandOutput
		}
		line, err = runOutput(ctx, app.NonInteractiveRunRequest{Version: r.version, Prompt: strings.Join(parsed.arguments, " ")})
		if err != nil {
			return fmt.Errorf("run non-interactive command: %w", err)
		}
	} else if parsed.command == "config" {
		configOutput := r.config
		if configOutput == nil {
			configOutput = app.ConfigCommandOutput
		}
		line, err = configOutput(parsed.all)
		if err != nil {
			return fmt.Errorf("load config command: %w", err)
		}
	} else if parsed.command == "models" {
		modelsOutput := r.models
		if modelsOutput == nil {
			modelsOutput = app.ModelsCommandOutput
		}
		line, err = modelsOutput(r.version, parsed.arguments)
		if err != nil {
			return err
		}
	} else if parsed.version {
		line = fmt.Sprintf("aila %s\n", r.version)
	}
	if _, err := fmt.Fprint(r.output, line); err != nil {
		_, _ = fmt.Fprintf(r.errors, "write CLI output: %v\n", err)
		return err
	}
	return nil
}

type parsedCLI struct {
	command   string
	arguments []string
	model     string
	all       bool
	version   bool
	debug     bool
}

func parseCLI(args []string) (parsedCLI, error) {
	parsed := parsedCLI{}
	continuation := false

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--version" || arg == "-V":
			parsed.version = true
		case arg == "--debug":
			parsed.debug = true
		case arg == "--continue" || arg == "-c":
			continuation = true
		case arg == "--model" || arg == "-m":
			index++
			if index >= len(args) || strings.HasPrefix(args[index], "-") {
				return parsedCLI{}, fmt.Errorf("missing value for %s; valid CLI flags: %s", arg, strings.Join(cliFlags, ", "))
			}
			parsed.model = args[index]
		case strings.HasPrefix(arg, "--model="):
			parsed.model = strings.TrimPrefix(arg, "--model=")
			if parsed.model == "" {
				return parsedCLI{}, fmt.Errorf("missing value for --model; valid CLI flags: %s", strings.Join(cliFlags, ", "))
			}
		case arg == "--all":
			if parsed.command != "config" {
				return parsedCLI{}, fmt.Errorf("unsupported CLI flag %q for %s; valid config shape: config [--all]", arg, commandName(parsed.command))
			}
			parsed.all = true
		case strings.HasPrefix(arg, "-"):
			return parsedCLI{}, fmt.Errorf("unknown flag %q; valid CLI flags: %s", arg, strings.Join(cliFlags, ", "))
		case isCLICommand(arg):
			if parsed.command != "" {
				return parsedCLI{}, fmt.Errorf("incompatible CLI commands %q and %q; valid CLI commands: %s", parsed.command, arg, strings.Join(cliCommands, ", "))
			}
			parsed.command = arg
		default:
			if acceptsPositionals(parsed.command) {
				parsed.arguments = append(parsed.arguments, arg)
				continue
			}
			return parsedCLI{}, fmt.Errorf("unknown command %q; valid CLI commands: %s", arg, strings.Join(cliCommands, ", "))
		}
	}

	if parsed.version || (parsed.debug && parsed.command == "") {
		return parsed, nil
	}
	if continuation {
		if parsed.command == "" {
			parsed.command = "continue"
		}
		if parsed.command == "run" {
			return parsedCLI{}, fmt.Errorf("incompatible CLI continuation shape %q with --continue; use continue or --continue, not run --continue", parsed.command)
		}
	}
	if parsed.command == "" {
		return parsedCLI{}, fmt.Errorf("missing command; valid CLI commands: %s", strings.Join(cliCommands, ", "))
	}
	return parsed, nil
}

func isCLICommand(arg string) bool {
	for _, command := range cliCommands {
		if arg == command {
			return true
		}
	}
	return false
}

func acceptsPositionals(command string) bool {
	return command == "run" || command == "models"
}

func commandName(command string) string {
	if command == "" {
		return "missing command"
	}
	return command
}

func commandOutput(version string, parsed parsedCLI) string {
	switch parsed.command {
	case "run":
		return fmt.Sprintf("aila %s\ncommand: run\nstatus: run handler unavailable\naccepted: run [prompt...] [--model MODEL]\ndeferred: provider model turns, write tools, workflow transitions\n", version)
	case "continue":
		return fmt.Sprintf("aila %s\ncommand: continue\nstatus: deferred-continuation stub\naccepted: continue | --continue | -c\ndeferred: session discovery, state lookup, persistence IO, continuation execution\n", version)
	case "config":
		return fmt.Sprintf("aila %s\ncommand: config\nstatus: deferred-config-ui\naccepted: config [--all]\ndeferred: interactive config UI\n", version)
	case "help":
		return fmt.Sprintf("aila %s\naccepted command shape:\n  aila run [prompt...] [--model MODEL] [--debug]\n  aila continue | aila --continue | aila -c\n  aila config [--all] [--debug]\n  aila models [filter...] [--debug]\n  aila help\n  aila --version | aila -V\n  aila --debug\nDeferred beyond current read-only run: stdin review, session discovery UI, credentials, provider model turns, write tools, workflow transitions, and mutation persistence.\n", version)
	default:
		return fmt.Sprintf("aila %s: %s command behavior deferred\n", version, parsed.command)
	}
}
