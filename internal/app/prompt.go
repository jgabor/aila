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

func (runner *inputRunner) submitPrompt(text string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.PromptSubmitted{Text: text})
	return transcriptTurn(runner.model.Transcript[before:])
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
