package app

import (
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

type runtimeDispatchFunc func([]runtime.Effect) []runtime.Message

type inputRunner struct {
	model    runtime.Model
	dispatch runtimeDispatchFunc
}

func newInputRunnerWithDispatch(dispatch runtimeDispatchFunc) *inputRunner {
	return &inputRunner{
		model:    runtime.Model{Status: runtime.StatusIdle},
		dispatch: dispatch,
	}
}

func (runner *inputRunner) submitPrompt(text string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.PromptSubmitted{Text: text})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) requestInterrupt(reason string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.InterruptRequested{Reason: reason})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) requestShutdown(err error) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.RuntimeDiagnostic{Diagnostic: signalShutdownDiagnostic(err)})
	if runner.model.Status == runtime.StatusActive || runner.model.Status == runtime.StatusCanceling {
		runner.apply(runtime.InterruptRequested{Reason: "signal shutdown"})
	}
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) applyRuntimeState(turn *tui.TranscriptTurn) {
	turn.RuntimeStatus = string(runner.model.Status)
	turn.StatusSource = "runtime.dispatch"
	turn.StatusDetail = "fake in-memory runtime loop"
	turn.RuntimeActive = runner.model.Status == runtime.StatusActive || runner.model.Status == runtime.StatusCanceling
	turn.RuntimeResult = runner.model.Result
	turn.QueuedCount = len(runner.model.Queued)
	turn.QueuedText = queuedText(runner.model.Queued)
	turn.Diagnostics = diagnosticViews(runner.model.Diagnostics)
}

func (runner *inputRunner) routeCommand(recommendation policy.CommandRecommendation) {
	if recommendation.Route != policy.CommandRouteStatus {
		return
	}
	runner.apply(runtime.CommandSelected{Name: string(recommendation.Route)})
}

func (runner *inputRunner) apply(message runtime.Message) {
	var effects []runtime.Effect
	runner.model, effects = runner.update(message)
	for _, result := range runner.dispatchEffects(effects) {
		runner.model, _ = runner.update(result)
	}
}

func (runner *inputRunner) update(message runtime.Message) (model runtime.Model, effects []runtime.Effect) {
	defer func() {
		if recovered := recover(); recovered != nil {
			model, effects = runtime.Update(runner.model, runtime.PanicMessage(diagnostic.SourceRuntime, recovered))
		}
	}()
	return runtime.Update(runner.model, message)
}

func (runner *inputRunner) dispatchEffects(effects []runtime.Effect) (messages []runtime.Message) {
	defer func() {
		if recovered := recover(); recovered != nil {
			messages = []runtime.Message{runtime.PanicMessage(diagnostic.SourceEffect, recovered)}
		}
	}()
	return runner.dispatch(effects)
}

func queuedText(entries []runtime.QueuedEntry) []string {
	if len(entries) == 0 {
		return nil
	}

	text := make([]string, 0, len(entries))
	for _, entry := range entries {
		text = append(text, entry.Text)
	}
	return text
}

func transcriptTurn(entries []runtime.TranscriptEntry) tui.TranscriptTurn {
	var turn tui.TranscriptTurn
	for _, entry := range entries {
		switch entry.Kind {
		case "prompt":
			turn.UserText = entry.Text
		case "result", "failure":
			turn.AssistantText = entry.Text
		}
	}
	return turn
}
