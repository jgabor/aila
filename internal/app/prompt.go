package app

import (
	"strings"

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

func (runner *inputRunner) proposeReadTool(request runtime.ReadToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.ReadToolProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeSearchTool(request runtime.SearchToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.SearchToolProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeBashTool(request runtime.BashToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.BashToolProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeFetchTool(request runtime.FetchToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.FetchToolProposed{Request: request})
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
	turn.Read = readView(runner.model)
	turn.Search = searchView(runner.model)
	turn.Command = commandView(runner.model)
	turn.Fetch = fetchView(runner.model)
	if turn.Read != nil {
		turn.StatusDetail = "read tool dispatch"
	}
	if turn.Search != nil {
		turn.StatusDetail = "search tool dispatch"
	}
	if turn.Command != nil {
		turn.StatusDetail = "bash tool dispatch"
	}
	if turn.Fetch != nil {
		turn.StatusDetail = "fetch tool dispatch"
	}
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

func readView(model runtime.Model) *tui.ReadView {
	if model.ActiveOperation.Kind == runtime.OperationRead && model.Status == runtime.StatusActive {
		request := model.ActiveRead
		return &tui.ReadView{
			Name:           "read",
			Status:         "running",
			ReadOnly:       true,
			Path:           request.Path,
			RequestedRange: readRangeViewFromRequest(request),
		}
	}
	if model.LastRead.ToolName == "" && model.LastRead.RequestedPath == "" && model.LastRead.WorkspaceRelativePath == "" {
		return nil
	}
	status := "completed"
	if model.LastRead.Error.Kind != "" && model.LastRead.Error.Kind != runtime.ReadToolErrorNone {
		status = "failed"
	}
	path := model.LastRead.WorkspaceRelativePath
	if path == "" {
		path = model.LastRead.RequestedPath
	}
	return &tui.ReadView{
		Name:             defaultString(model.LastRead.ToolName, "read"),
		Status:           status,
		ReadOnly:         true,
		Path:             path,
		RequestedRange:   readRangeView(model.LastRead.RequestedRange),
		EffectiveRange:   readRangeView(model.LastRead.EffectiveRange),
		PreviewLines:     readPreviewLines(model.LastRead.PreviewText),
		PreviewTruncated: model.LastRead.Truncation.PreviewTruncated,
		LineLimitHit:     model.LastRead.Truncation.LineLimitHit,
		TruncationMarker: model.LastRead.Truncation.Marker,
		ErrorKind:        string(model.LastRead.Error.Kind),
		ErrorMessage:     model.LastRead.Error.Message,
	}
}

func readRangeViewFromRequest(request runtime.ReadToolRequest) tui.ReadLineRangeView {
	return tui.ReadLineRangeView{StartLine: request.StartLine, Limit: request.LineLimit}
}

func readRangeView(lineRange runtime.ReadLineRange) tui.ReadLineRangeView {
	return tui.ReadLineRangeView{StartLine: lineRange.StartLine, EndLine: lineRange.EndLine, Limit: lineRange.Limit}
}

func readPreviewLines(preview string) []string {
	preview = strings.TrimRight(preview, "\n")
	if preview == "" {
		return nil
	}
	return strings.Split(preview, "\n")
}

func searchView(model runtime.Model) *tui.SearchView {
	if (model.ActiveOperation.Kind == runtime.OperationFind || model.ActiveOperation.Kind == runtime.OperationGrep) && model.Status == runtime.StatusActive {
		request := model.ActiveSearch
		return &tui.SearchView{
			Name:           string(request.ToolName),
			Status:         "running",
			ReadOnly:       true,
			Pattern:        request.Pattern,
			Query:          request.Query,
			Regex:          request.Regex,
			IncludePattern: request.IncludePattern,
		}
	}
	if model.LastSearch.ToolName == "" && model.LastSearch.Pattern == "" && model.LastSearch.Query == "" {
		return nil
	}
	status := "completed"
	if model.LastSearch.Error.Kind != "" && model.LastSearch.Error.Kind != runtime.SearchToolErrorNone {
		status = "failed"
	}
	return &tui.SearchView{
		Name:              defaultString(model.LastSearch.ToolName, "search"),
		Status:            status,
		ReadOnly:          true,
		Pattern:           model.LastSearch.Pattern,
		Query:             model.LastSearch.Query,
		Regex:             model.LastSearch.Regex,
		IncludePattern:    model.LastSearch.IncludePattern,
		Matches:           searchMatchViews(model.LastSearch.Matches),
		OmittedResults:    model.LastSearch.Truncation.OmittedResults,
		OmittedFiles:      model.LastSearch.Truncation.OmittedFiles,
		PreviewTruncated:  model.LastSearch.Truncation.PreviewTruncated,
		ResultLimitHit:    model.LastSearch.Truncation.ResultLimitHit,
		TruncationMarkers: model.LastSearch.Truncation.TruncationMarkers,
		ErrorKind:         string(model.LastSearch.Error.Kind),
		ErrorMessage:      model.LastSearch.Error.Message,
	}
}

func searchMatchViews(matches []runtime.SearchToolMatch) []tui.SearchMatchView {
	if len(matches) == 0 {
		return nil
	}
	views := make([]tui.SearchMatchView, 0, len(matches))
	for _, match := range matches {
		views = append(views, tui.SearchMatchView{Path: match.Path, LineNumber: match.LineNumber, PreviewText: match.PreviewText})
	}
	return views
}

func commandView(model runtime.Model) *tui.CommandView {
	if model.ActiveOperation.Kind == runtime.OperationBash && model.Status == runtime.StatusActive {
		request := model.ActiveBash
		return &tui.CommandView{
			Name:       "bash",
			Status:     "running",
			ReadOnly:   true,
			Argv:       append([]string(nil), request.Argv...),
			WorkingDir: defaultString(request.WorkingDir, "."),
		}
	}
	if model.LastBash.ToolName == "" && len(model.LastBash.RequestedArgv) == 0 && len(model.LastBash.EffectiveArgv) == 0 {
		return nil
	}
	status := defaultString(model.LastBash.Status, "completed")
	if model.LastBash.Error.Kind != "" && model.LastBash.Error.Kind != runtime.BashToolErrorNone {
		status = "failed"
	}
	return &tui.CommandView{
		Name:            defaultString(model.LastBash.ToolName, "bash"),
		Status:          status,
		ReadOnly:        true,
		Argv:            append([]string(nil), model.LastBash.RequestedArgv...),
		WorkingDir:      defaultString(model.LastBash.WorkspaceRelativeWorkDir, "."),
		CommandFamily:   model.LastBash.CommandFamily,
		ExpectedEffect:  model.LastBash.ExpectedEffect,
		ExitCode:        model.LastBash.ExitCode,
		StdoutLines:     commandOutputLines(model.LastBash.Stdout.Text),
		StderrLines:     commandOutputLines(model.LastBash.Stderr.Text),
		StdoutTruncated: model.LastBash.Stdout.Truncated,
		StderrTruncated: model.LastBash.Stderr.Truncated,
		DurationMillis:  model.LastBash.DurationMillis,
		ErrorKind:       string(model.LastBash.Error.Kind),
		ErrorMessage:    model.LastBash.Error.Message,
	}
}

func fetchView(model runtime.Model) *tui.FetchView {
	if model.ActiveOperation.Kind == runtime.OperationFetch && model.Status == runtime.StatusActive {
		request := model.ActiveFetch
		return &tui.FetchView{
			Name:     "fetch",
			Status:   "running",
			ReadOnly: true,
			URL:      request.URL,
			Method:   defaultString(request.Method, "GET"),
		}
	}
	if model.LastFetch.ToolName == "" && model.LastFetch.RequestedURL == "" && model.LastFetch.EffectiveURL == "" {
		return nil
	}
	status := defaultString(model.LastFetch.Status, "completed")
	if model.LastFetch.Error.Kind != "" && model.LastFetch.Error.Kind != runtime.FetchToolErrorNone {
		status = "failed"
		if model.LastFetch.Status != "" {
			status = model.LastFetch.Status
		}
	}
	url := model.LastFetch.EffectiveURL
	if url == "" {
		url = model.LastFetch.RequestedURL
	}
	return &tui.FetchView{
		Name:              defaultString(model.LastFetch.ToolName, "fetch"),
		Status:            status,
		ReadOnly:          true,
		URL:               url,
		Method:            defaultString(model.LastFetch.Method, "GET"),
		ExpectedEffect:    model.LastFetch.ExpectedEffect,
		HTTPStatusCode:    model.LastFetch.HTTPStatusCode,
		HTTPStatus:        model.LastFetch.HTTPStatus,
		ContentType:       model.LastFetch.ContentType,
		PreviewLines:      fetchPreviewLines(model.LastFetch.PreviewText),
		PreviewTruncated:  model.LastFetch.Truncation.PreviewTruncated,
		OmittedBytesKnown: model.LastFetch.Truncation.OmittedBytesKnown,
		OmittedBytes:      model.LastFetch.Truncation.OmittedBytes,
		TruncationMarker:  model.LastFetch.Truncation.Marker,
		DurationMillis:    model.LastFetch.DurationMillis,
		ErrorKind:         string(model.LastFetch.Error.Kind),
		ErrorMessage:      model.LastFetch.Error.Message,
	}
}

func fetchPreviewLines(preview string) []string {
	preview = strings.TrimRight(preview, "\n")
	if preview == "" {
		return nil
	}
	return strings.Split(preview, "\n")
}

func commandOutputLines(output string) []string {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
