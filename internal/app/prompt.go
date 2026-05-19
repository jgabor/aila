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
	model                    runtime.Model
	dispatch                 runtimeDispatchFunc
	agent                    *agentPromptRunner
	activeAgentCancel        func()
	pendingMutationApprovals map[string]runtime.MutationToolRequest
}

func newInputRunnerWithDispatch(dispatch runtimeDispatchFunc) *inputRunner {
	return &inputRunner{
		model:    runtime.Model{Status: runtime.StatusIdle},
		dispatch: dispatch,
	}
}

func (runner *inputRunner) submitPrompt(text string) tui.TranscriptTurn {
	if runner.agent != nil {
		return runner.submitAgentPrompt(text)
	}
	before := len(runner.model.Transcript)
	runner.apply(runtime.PromptSubmitted{Text: text})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) requestInterrupt(reason string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	if runner.activeAgentCancel != nil {
		runner.activeAgentCancel()
	}
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

func (runner *inputRunner) proposeCompactContext(request runtime.CompactContextRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.CompactContextProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeBackgroundCompactContext(request runtime.CompactContextRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.BackgroundCompactContextProposed{Request: request})
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

func (runner *inputRunner) proposeEditTool(request runtime.MutationToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.EditToolProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeWriteTool(request runtime.MutationToolRequest) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.WriteToolProposed{Request: request})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) proposeApproval(proposal runtime.ApprovalProposal) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.ApprovalProposed{Proposal: proposal})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	return turn
}

func (runner *inputRunner) decideApproval(decision tui.ApprovalDecisionInput) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	runner.apply(runtime.ApprovalDecisionSelected{ProposalID: decision.ProposalID, Action: runtime.ApprovalAction(decision.Action)})
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	if turn.ApprovalDecision == nil || turn.ApprovalDecision.Stale {
		return turn
	}
	if request, ok := runner.takeMutationApproval(turn.ApprovalDecision.ProposalID); ok {
		if runtime.ApprovalAction(turn.ApprovalDecision.Action) != runtime.ApprovalActionApprove {
			return buildAgentEvidenceTurn(turn)
		}
		mutationTurn := runner.proposeWriteTool(request)
		if request.ToolName == runtime.MutationToolEdit {
			mutationTurn = runner.proposeEditTool(request)
		}
		mutationTurn.ApprovalDecision = turn.ApprovalDecision
		return buildAgentEvidenceTurn(mutationTurn)
	}
	if decision.ProposalID == fakeApprovalWriteProposalID && runtime.ApprovalAction(decision.Action) == runtime.ApprovalActionApprove {
		mutationTurn := runner.proposeWriteTool(fakeApprovalWriteRequest())
		mutationTurn.ApprovalDecision = turn.ApprovalDecision
		return mutationTurn
	}
	return turn
}

func (runner *inputRunner) rememberMutationApproval(proposalID string, request runtime.MutationToolRequest) {
	proposalID = strings.TrimSpace(proposalID)
	if proposalID == "" {
		return
	}
	if runner.pendingMutationApprovals == nil {
		runner.pendingMutationApprovals = make(map[string]runtime.MutationToolRequest)
	}
	runner.pendingMutationApprovals[proposalID] = request
}

func (runner *inputRunner) takeMutationApproval(proposalID string) (runtime.MutationToolRequest, bool) {
	if len(runner.pendingMutationApprovals) == 0 {
		return runtime.MutationToolRequest{}, false
	}
	proposalID = strings.TrimSpace(proposalID)
	request, ok := runner.pendingMutationApprovals[proposalID]
	if ok {
		delete(runner.pendingMutationApprovals, proposalID)
	}
	return request, ok
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
	turn.RuntimeActive = runtimeActive(runner.model)
	turn.RuntimeResult = runner.model.Result
	if runner.model.AssistantDraft != "" && turn.AssistantText == "" {
		turn.AssistantText = runner.model.AssistantDraft
		turn.AssistantStreaming = runner.model.Status == runtime.StatusActive
	}
	if runner.model.CapabilityDraft != "" && turn.AssistantText == "" {
		turn.AssistantText = runner.model.CapabilityDraft
		turn.AssistantStreaming = runner.model.Status == runtime.StatusActive
	}
	turn.QueuedCount = len(runner.model.Queued)
	turn.QueuedText = queuedText(runner.model.Queued)
	turn.Diagnostics = diagnosticViews(runner.model.Diagnostics)
	turn.Subagents = subagentViews(runner.model)
	runner.applyAgentState(turn)
	turn.Read = readView(runner.model)
	turn.Search = searchView(runner.model)
	turn.Command = commandView(runner.model)
	turn.Utility = utilityView(runner.model)
	turn.Compact = compactView(runner.model)
	if turn.Compact != nil {
		turn.Context = compactContextView(runner.model.LastCompact)
	}
	turn.Fetch = fetchView(runner.model)
	turn.Mutation = mutationView(runner.model)
	turn.Approval = approvalView(runner.model.PendingApproval)
	turn.ApprovalDecision = approvalDecisionView(runner.model.LastApprovalDecision)
	if turn.Read != nil {
		turn.StatusDetail = "read tool dispatch"
	}
	if turn.Search != nil {
		turn.StatusDetail = "search tool dispatch"
	}
	if turn.Command != nil {
		turn.StatusDetail = "bash tool dispatch"
	}
	if turn.Utility != nil {
		turn.StatusDetail = "utility worker status"
	}
	if turn.Compact != nil {
		turn.StatusDetail = "manual context compaction"
		if turn.Compact.Mode == string(runtime.CompactModeBackground) {
			turn.StatusDetail = "background context compaction"
		}
	}
	if turn.Fetch != nil {
		turn.StatusDetail = "fetch tool dispatch"
	}
	if turn.Mutation != nil {
		turn.StatusDetail = "mutation tool dispatch"
	}
	if turn.Approval != nil {
		turn.StatusDetail = "approval pending"
	}
	if len(turn.Subagents) > 0 && turn.StatusDetail == "fake in-memory runtime loop" {
		turn.StatusDetail = "subagent supervision"
	}
	if runner.model.LastAgentPause.Resumable {
		turn.StatusDetail = "agent paused at step budget"
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
	if runner.agent != nil {
		return runner.dispatchAgentOwnedEffects(effects)
	}
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
		case "result", "failure", "paused":
			turn.AssistantText = entry.Text
		}
	}
	return turn
}

func approvalView(proposal runtime.ApprovalProposal) *tui.ApprovalProposalView {
	if proposal.ID == "" && proposal.Target == "" && proposal.Path == "" && len(proposal.Command) == 0 {
		return nil
	}
	return &tui.ApprovalProposalView{
		ID:             proposal.ID,
		OperationKind:  proposal.OperationKind,
		Target:         proposal.Target,
		RiskSummary:    proposal.RiskSummary,
		PreviewLines:   append([]string(nil), proposal.Preview...),
		DefaultAction:  string(proposal.DefaultAction),
		Path:           proposal.Path,
		Command:        append([]string(nil), proposal.Command...),
		WorkingDir:     proposal.WorkingDir,
		ExpectedEffect: proposal.ExpectedEffect,
		DiffPreview:    append([]string(nil), proposal.DiffPreview...),
		Reversible:     proposal.Reversible,
		RunID:          proposal.RunID,
		Capability:     proposal.Capability,
	}
}

func approvalDecisionView(decision runtime.ApprovalDecision) *tui.ApprovalDecisionView {
	if decision.ProposalID == "" && decision.Action == "" {
		return nil
	}
	return &tui.ApprovalDecisionView{ProposalID: decision.ProposalID, Action: string(decision.Action), Stale: decision.Stale}
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
		Decision:         decisionView(model.LastRead.Decision),
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
		Decision:          decisionView(model.LastSearch.Decision),
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
		Decision:        decisionView(model.LastBash.Decision),
	}
}

func mutationView(model runtime.Model) *tui.MutationView {
	if (model.ActiveOperation.Kind == runtime.OperationEdit || model.ActiveOperation.Kind == runtime.OperationWrite) && model.Status == runtime.StatusActive {
		request := model.ActiveMutation
		return &tui.MutationView{
			Name:           string(request.ToolName),
			Status:         "running",
			Path:           request.Path,
			ExpectedEffect: request.ExpectedEffect,
		}
	}
	if model.LastMutation.ToolName == "" && model.LastMutation.RequestedPath == "" && model.LastMutation.WorkspaceRelativePath == "" {
		return nil
	}
	status := model.LastMutation.Status
	if status == "" {
		status = "completed"
	}
	if model.LastMutation.Error.Kind != "" && model.LastMutation.Error.Kind != runtime.MutationToolErrorNone {
		status = "failed"
		if model.LastMutation.Status != "" {
			status = model.LastMutation.Status
		}
	}
	path := model.LastMutation.WorkspaceRelativePath
	if path == "" {
		path = model.LastMutation.RequestedPath
	}
	return &tui.MutationView{
		Name:                  defaultString(model.LastMutation.ToolName, "mutation"),
		Status:                status,
		Path:                  path,
		ExpectedEffect:        model.LastMutation.ExpectedEffect,
		PreviousVersion:       model.LastMutation.PreviousVersion,
		NewVersion:            model.LastMutation.NewVersion,
		PreviousExists:        model.LastMutation.PreviousExists,
		BytesWritten:          model.LastMutation.BytesWritten,
		ReplacementCount:      model.LastMutation.ReplacementCount,
		ResolvedPathAvailable: model.LastMutation.ResolvedPathAvailable,
		ErrorKind:             string(model.LastMutation.Error.Kind),
		ErrorMessage:          model.LastMutation.Error.Message,
		Decision:              decisionView(model.LastMutation.Decision),
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
		Decision:          decisionView(model.LastFetch.Decision),
	}
}

func decisionView(decision runtime.ToolDecision) *tui.DecisionView {
	if !decision.Present {
		return nil
	}
	return &tui.DecisionView{
		Autonomy:         decision.Autonomy,
		Source:           decision.Source,
		Allowed:          decision.Allowed,
		Automatic:        decision.Automatic,
		ApprovalRequired: decision.ApprovalRequired,
		Reason:           decision.Reason,
		OperationKind:    decision.OperationKind,
		Name:             decision.Tool,
		Target:           decision.Target,
		Command:          append([]string(nil), decision.Command...),
		WorkingDir:       decision.WorkingDir,
		ExpectedEffect:   decision.ExpectedEffect,
		Reversible:       decision.Reversible,
		RunID:            decision.RunID,
		Capability:       decision.Capability,
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

const fakeApprovalWriteProposalID = "fake-approval-write-001"

var fakeApprovalWriteOverride struct {
	path    string
	content string
}

func configureFakeApprovalWrite(path string, content string) {
	fakeApprovalWriteOverride.path = path
	fakeApprovalWriteOverride.content = content
}

func fakeApprovalProposal() runtime.ApprovalProposal {
	return runtime.ApprovalProposal{
		ID:             "fake-approval-001",
		OperationKind:  "file_mutation",
		Target:         "internal/demo.txt",
		RiskSummary:    "Would update a workspace file if mutation execution existed.",
		Preview:        []string{"write requested by fake PTY proposal", "default is deny until write classes exist"},
		DefaultAction:  runtime.ApprovalActionDeny,
		Path:           "internal/demo.txt",
		Command:        []string{"write", "internal/demo.txt"},
		WorkingDir:     ".",
		ExpectedEffect: "preview only; no mutation execution in display-only approval proposal",
		DiffPreview:    []string{"--- internal/demo.txt", "+++ internal/demo.txt", "@@", "-old", "+new"},
		Reversible:     true,
		RunID:          "run-fake-approval",
		Capability:     "m25-fixture",
	}
}

func fakeApprovalWriteProposal() runtime.ApprovalProposal {
	path := fakeApprovalWritePath()
	return runtime.ApprovalProposal{
		ID:             fakeApprovalWriteProposalID,
		OperationKind:  "mutation",
		Target:         path,
		RiskSummary:    "Will create a workspace file through the explicit write mutation effect.",
		Preview:        []string{"write requested by fake PTY approval path", "approval dispatches an app-owned write effect"},
		DefaultAction:  runtime.ApprovalActionDeny,
		Path:           path,
		Command:        []string{"write", path},
		WorkingDir:     ".",
		ExpectedEffect: "create fake approval write target through explicit mutation effect",
		DiffPreview:    []string{"--- " + path, "+++ " + path, "@@", "+" + strings.TrimRight(fakeApprovalWriteContent(), "\n")},
		Reversible:     false,
		RunID:          "run-fake-approval-write",
		Capability:     "approval-write",
	}
}

func fakeApprovalWriteRequest() runtime.MutationToolRequest {
	return runtime.MutationToolRequest{
		Path:           fakeApprovalWritePath(),
		TargetVersion:  "missing",
		Content:        fakeApprovalWriteContent(),
		ExpectedEffect: "create fake approval write target through explicit mutation effect",
		Source: runtime.MutationSourceMetadata{
			Caller:    "approval-write",
			RequestID: "fake-approval-write",
		},
	}
}

func fakeApprovalWritePath() string {
	if value := strings.TrimSpace(fakeApprovalWriteOverride.path); value != "" {
		return value
	}
	return "internal/fake-approval-write.txt"
}

func fakeApprovalWriteContent() string {
	if fakeApprovalWriteOverride.content != "" {
		return fakeApprovalWriteOverride.content
	}
	return "approved write\n"
}
