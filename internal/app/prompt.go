package app

import (
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

type runtimeDispatchFunc func([]runtime.Effect) []runtime.Message

type inputRunner struct {
	model    runtime.Model
	dispatch runtimeDispatchFunc
}

func newInputRunner() *inputRunner {
	return newInputRunnerWithDispatch(runtime.Dispatch)
}

func newInputRunnerWithDispatch(dispatch runtimeDispatchFunc) *inputRunner {
	return &inputRunner{
		model:    runtime.Model{Status: runtime.StatusIdle},
		dispatch: dispatch,
	}
}

func newInputRunnerHoldingFakeWork() *inputRunner {
	return newInputRunnerWithDispatch(func([]runtime.Effect) []runtime.Message { return nil })
}

func newInputRunnerHoldingFakeWorkWithSecondInterruptResolution() *inputRunner {
	interrupts := 0
	return newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		for _, effect := range effects {
			if _, ok := effect.(runtime.FakeInterruptEffect); ok {
				interrupts++
				if interrupts >= 2 {
					return runtime.Dispatch(effects)
				}
			}
		}
		return nil
	})
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

func (runner *inputRunner) applyRuntimeState(turn *tui.TranscriptTurn) {
	turn.RuntimeStatus = string(runner.model.Status)
	turn.StatusSource = "runtime.dispatch"
	turn.StatusDetail = "fake in-memory runtime loop"
	turn.RuntimeActive = runner.model.Status == runtime.StatusActive || runner.model.Status == runtime.StatusCanceling
	turn.RuntimeResult = runner.model.Result
	turn.QueuedCount = len(runner.model.Queued)
	turn.QueuedText = queuedText(runner.model.Queued)
}

func (runner *inputRunner) routeCommand(recommendation policy.CommandRecommendation) {
	if recommendation.Route != policy.CommandRouteStatus {
		return
	}
	runner.apply(runtime.CommandSelected{Name: string(recommendation.Route)})
}

func (runner *inputRunner) apply(message runtime.Message) {
	var effects []runtime.Effect
	runner.model, effects = runtime.Update(runner.model, message)
	for _, result := range runner.dispatch(effects) {
		runner.model, _ = runtime.Update(runner.model, result)
	}
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
