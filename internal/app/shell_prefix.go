package app

import (
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

const shellPrefixSource = "shell.prefix"

func (controller *sessionController) submitShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	switch recommendation.Kind {
	case policy.ShellPrefixExecutable:
		return controller.submitExecutableShellPrefix(recommendation)
	case policy.ShellPrefixSummarized:
		return controller.submitDeferredSummarizedShellPrefix(recommendation)
	default:
		return tui.TranscriptTurn{}
	}
}

func (controller *sessionController) submitExecutableShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	turn := controller.runner.proposeBashTool(runtime.BashToolRequest{
		Argv:       shellPrefixArgv(recommendation.CommandText),
		WorkingDir: ".",
		Source: runtime.BashSourceMetadata{
			Caller:      shellPrefixSource,
			RequestID:   shellPrefixRequestID(recommendation),
			Description: "user shell prefix command",
		},
	})
	turn.UserText = recommendation.ExactInput
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	diagnostics := controller.persistShellPrefixHistory(recommendation, turn)
	turn.Diagnostics = append(turn.Diagnostics, diagnostics...)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, turn.Diagnostics)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) submitDeferredSummarizedShellPrefix(recommendation policy.ShellPrefixRecommendation) tui.TranscriptTurn {
	message := "summarized shell output is deferred until Milestone 39 context builder"
	turn := tui.TranscriptTurn{
		UserText:      recommendation.ExactInput,
		RuntimeStatus: "idle",
		StatusSource:  shellPrefixSource,
		StatusDetail:  message,
		RuntimeResult: message,
		Command: &tui.CommandView{
			Name:           "bash",
			Status:         "deferred",
			ReadOnly:       true,
			Argv:           shellPrefixArgv(recommendation.CommandText),
			WorkingDir:     ".",
			CommandFamily:  "summarized shell",
			ExpectedEffect: "summarize shell output for context in Milestone 39",
			ErrorKind:      "deferred",
			ErrorMessage:   message,
		},
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	diagnostics := controller.persistShellPrefixHistory(recommendation, turn)
	turn.Diagnostics = append(turn.Diagnostics, diagnostics...)
	controller.view.Diagnostics = mergeTUIDiagnostics(controller.view.Diagnostics, turn.Diagnostics)
	return controller.persistCurrentSnapshot(turn)
}

func (controller *sessionController) persistShellPrefixHistory(recommendation policy.ShellPrefixRecommendation, turn tui.TranscriptTurn) []tui.DiagnosticView {
	status := "submitted"
	if turn.Command != nil && turn.Command.Status != "" {
		status = turn.Command.Status
	}
	display := "shell-prefix " + string(recommendation.Kind) + " " + status + " " + recommendation.ExactInput
	if turn.Command != nil && turn.Command.ExitCode != 0 {
		display += " exit=" + strconv.Itoa(turn.Command.ExitCode)
	}
	return controller.persistHistoryEvent(history.EventKindCommand, shellPrefixSource, shellPrefixSource, display)
}

func shellPrefixRequestID(recommendation policy.ShellPrefixRecommendation) string {
	return string(recommendation.Kind) + ":" + recommendation.CommandText
}

func shellPrefixArgv(command string) []string {
	return strings.Fields(command)
}
