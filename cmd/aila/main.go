package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jgabor/aila/internal/app"
)

const version = "dev"

var m7Commands = []string{"run", "continue", "config", "models", "help"}

var m7Flags = []string{"--model", "-m", "--continue", "-c", "--version", "-V"}

type cliRunner struct {
	input   io.Reader
	output  io.Writer
	errors  io.Writer
	version string
	start   func(context.Context, io.Reader, io.Writer) error
	config  func(bool) (string, error)
}

func main() {
	runner := cliRunner{
		input:   os.Stdin,
		output:  os.Stdout,
		errors:  os.Stderr,
		version: version,
		start:   app.Run,
		config:  app.ConfigCommandOutput,
	}
	if err := runner.run(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(runner.errors, err)
		os.Exit(1)
	}
}

func (r cliRunner) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return r.start(ctx, r.input, r.output)
	}

	parsed, err := parseM7CLI(args)
	if err != nil {
		return err
	}

	line := m7StubOutput(r.version, parsed)
	if parsed.command == "config" {
		configOutput := r.config
		if configOutput == nil {
			configOutput = app.ConfigCommandOutput
		}
		line, err = configOutput(parsed.all)
		if err != nil {
			return fmt.Errorf("load config command: %w", err)
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

type m7CLI struct {
	command   string
	arguments []string
	all       bool
	version   bool
}

func parseM7CLI(args []string) (m7CLI, error) {
	parsed := m7CLI{}
	continuation := false

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--version" || arg == "-V":
			parsed.version = true
		case arg == "--continue" || arg == "-c":
			continuation = true
		case arg == "--model" || arg == "-m":
			index++
			if index >= len(args) || strings.HasPrefix(args[index], "-") {
				return m7CLI{}, fmt.Errorf("missing value for %s; valid M7 flags: %s", arg, strings.Join(m7Flags, ", "))
			}
		case strings.HasPrefix(arg, "--model="):
			if strings.TrimPrefix(arg, "--model=") == "" {
				return m7CLI{}, fmt.Errorf("missing value for --model; valid M7 flags: %s", strings.Join(m7Flags, ", "))
			}
		case arg == "--all":
			if parsed.command != "config" {
				return m7CLI{}, fmt.Errorf("unsupported M7 flag %q for %s; valid config shape: config [--all]", arg, commandName(parsed.command))
			}
			parsed.all = true
		case strings.HasPrefix(arg, "-"):
			return m7CLI{}, fmt.Errorf("unknown flag %q; valid M7 flags: %s", arg, strings.Join(m7Flags, ", "))
		case isM7Command(arg):
			if parsed.command != "" {
				return m7CLI{}, fmt.Errorf("incompatible M7 commands %q and %q; valid M7 commands: %s", parsed.command, arg, strings.Join(m7Commands, ", "))
			}
			parsed.command = arg
		default:
			if acceptsM7Positionals(parsed.command) {
				parsed.arguments = append(parsed.arguments, arg)
				continue
			}
			return m7CLI{}, fmt.Errorf("unknown command %q; valid M7 commands: %s", arg, strings.Join(m7Commands, ", "))
		}
	}

	if parsed.version {
		return parsed, nil
	}
	if continuation {
		if parsed.command == "" {
			parsed.command = "continue"
		}
		if parsed.command == "run" {
			return m7CLI{}, fmt.Errorf("incompatible M7 continuation shape %q with --continue; use continue or --continue, not run --continue", parsed.command)
		}
	}
	if parsed.command == "" {
		return m7CLI{}, fmt.Errorf("missing command; valid M7 commands: %s", strings.Join(m7Commands, ", "))
	}
	return parsed, nil
}

func isM7Command(arg string) bool {
	for _, command := range m7Commands {
		if arg == command {
			return true
		}
	}
	return false
}

func acceptsM7Positionals(command string) bool {
	return command == "run" || command == "models"
}

func commandName(command string) string {
	if command == "" {
		return "missing command"
	}
	return command
}

func m7StubOutput(version string, parsed m7CLI) string {
	switch parsed.command {
	case "run":
		return fmt.Sprintf("aila %s\ncommand: run\nstatus: deferred-run stub\naccepted: run [prompt...] [--model MODEL]\ndeferred: prompt execution, stdin review, model turns, tool execution, workflow transitions\n", version)
	case "continue":
		return fmt.Sprintf("aila %s\ncommand: continue\nstatus: deferred-continuation stub\naccepted: continue | --continue | -c\ndeferred: session discovery, state lookup, persistence IO, continuation execution\n", version)
	case "config":
		return fmt.Sprintf("aila %s\ncommand: config\nstatus: deferred-config-ui\naccepted: config [--all]\ndeferred: interactive config UI\n", version)
	case "models":
		return fmt.Sprintf("aila %s\ncommand: models\nstatus: deferred-models stub\naccepted: models [filter...]\ndeferred: provider metadata, filtering behavior, credential lookup, degraded status, provider errors\n", version)
	case "help":
		return fmt.Sprintf("aila %s\nM7 accepted shape:\n  aila run [prompt...] [--model MODEL]\n  aila continue | aila --continue | aila -c\n  aila config [--all]\n  aila models [filter...]\n  aila help\n  aila --version | aila -V\nDeferred in M7: prompt execution, stdin review, session discovery, config IO, XDG/env reads, provider metadata, credentials, model turns, tools, workflow transitions, persistence.\n", version)
	default:
		return fmt.Sprintf("aila %s: M7 %s command stub; behavior deferred\n", version, parsed.command)
	}
}
