package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

const buildDefaultTargetPath = "docs/aila-build-output.md"

type buildPlanSelection struct {
	ID   string
	Text string
	OK   bool
}

func (controller *sessionController) openBuildView() []tui.DiagnosticView {
	selection := selectBuildPlanItem(controller.view.Plan)
	var writeTurn tui.TranscriptTurn
	var mutation runtime.MutationToolResult
	var diagnostics []tui.DiagnosticView
	if selection.OK {
		writeTurn = controller.runner.proposeWriteTool(buildMutationRequest(selection))
		mutation = controller.runner.model.LastMutation
		diagnostics = append(diagnostics, controller.persistMutationHistory(writeTurn)...)
	}
	request := buildRequestFromSelectionAndMutation(selection, mutation, workflowPhaseFromView(controller.view))
	turn := controller.runner.proposeCapability(request)
	if writeTurn.Mutation != nil {
		turn.Mutation = writeTurn.Mutation
	}
	if writeTurn.Approval != nil {
		turn.Approval = writeTurn.Approval
	}
	if writeTurn.ApprovalDecision != nil {
		turn.ApprovalDecision = writeTurn.ApprovalDecision
	}
	if turn.Build != nil {
		turn.StatusDetail = "build capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return diagnostics
}

func selectBuildPlanItem(plan *tui.PlanView) buildPlanSelection {
	if plan == nil {
		return buildPlanSelection{}
	}
	for _, item := range plan.Items {
		if !item.Done {
			return buildPlanSelection{ID: item.ID, Text: item.Text, OK: true}
		}
	}
	if len(plan.Items) > 0 {
		item := plan.Items[0]
		return buildPlanSelection{ID: item.ID, Text: item.Text, OK: true}
	}
	return buildPlanSelection{}
}

func buildMutationRequest(selection buildPlanSelection) runtime.MutationToolRequest {
	content := strings.Join([]string{
		"# Aila Build Output",
		"",
		"Plan item: " + selection.Text,
		"",
		"Result: executed one bounded build step and held.",
		"",
	}, "\n")
	return runtime.MutationToolRequest{
		ToolName:       runtime.MutationToolWrite,
		Path:           buildDefaultTargetPath,
		TargetVersion:  "missing",
		Content:        content,
		ExpectedEffect: "create bounded build output for plan item " + defaultString(selection.ID, "plan-item"),
		Source: runtime.MutationSourceMetadata{
			Caller:      string(capability.NameBuild),
			RequestID:   "build-" + defaultString(selection.ID, "plan-item"),
			Description: "bounded build capability write step",
		},
	}
}

func buildRequestFromSelectionAndMutation(selection buildPlanSelection, mutation runtime.MutationToolResult, phase workflow.Phase) capability.Request {
	metadata := map[string]string{}
	if selection.OK {
		metadata[capability.BuildMetadataPlanItemID] = defaultString(selection.ID, "plan-item")
		metadata[capability.BuildMetadataPlanItemText] = selection.Text
		metadata[capability.BuildMetadataStepID] = "write-build-output"
		metadata[capability.BuildMetadataStepText] = "Write bounded build output and hold."
	}
	if mutation.ToolName != "" || mutation.RequestedPath != "" || mutation.WorkspaceRelativePath != "" || mutation.Status != "" {
		path := mutation.WorkspaceRelativePath
		if path == "" {
			path = mutation.RequestedPath
		}
		metadata[capability.BuildMetadataToolName] = defaultString(mutation.ToolName, string(runtime.MutationToolWrite))
		metadata[capability.BuildMetadataToolStatus] = defaultString(mutation.Status, "completed")
		metadata[capability.BuildMetadataTargetPath] = path
		metadata[capability.BuildMetadataExpectedEffect] = mutation.ExpectedEffect
		metadata[capability.BuildMetadataDecisionSource] = mutation.Decision.Source
		metadata[capability.BuildMetadataDecisionAutonomy] = mutation.Decision.Autonomy
		metadata[capability.BuildMetadataDecisionAllowed] = strconv.FormatBool(mutation.Decision.Allowed)
		metadata[capability.BuildMetadataApprovalRequired] = strconv.FormatBool(mutation.Decision.ApprovalRequired)
		metadata[capability.BuildMetadataBytesWritten] = strconv.Itoa(mutation.BytesWritten)
		if mutation.Error.Kind != "" && mutation.Error.Kind != runtime.MutationToolErrorNone {
			metadata[capability.BuildMetadataErrorKind] = string(mutation.Error.Kind)
		}
		metadata[capability.BuildMetadataErrorMessage] = mutation.Error.Message
		metadata[capability.BuildMetadataFinalSummary] = buildFinalSummary(selection, mutation)
	}
	metadata[capability.BuildMetadataCaveats] = "bounded build executed one step and then held"
	return capability.Request{
		ID:         "command-build",
		Capability: capability.NameBuild,
		Input:      selection.Text,
		Phase:      normalizeBuildPhase(phase),
		SourceRefs: []capability.SourceRef{
			{ID: "build-plan-item", Kind: "plan_item", Excerpt: selection.Text},
			{ID: "build-runtime", Kind: "runtime_state", Excerpt: "phase=" + normalizeBuildPhase(phase).String()},
		},
		Metadata: metadata,
	}
}

func normalizeBuildPhase(phase workflow.Phase) workflow.Phase {
	if phase == "" || phase == workflow.PhaseIdle {
		return workflow.PhaseBuild
	}
	return phase
}

func buildFinalSummary(selection buildPlanSelection, mutation runtime.MutationToolResult) string {
	path := mutation.WorkspaceRelativePath
	if path == "" {
		path = mutation.RequestedPath
	}
	status := defaultString(mutation.Status, "completed")
	if mutation.Error.Message != "" {
		return fmt.Sprintf("Held after %s on %s for plan item %s: %s", status, path, defaultString(selection.ID, "plan-item"), mutation.Error.Message)
	}
	return fmt.Sprintf("Executed one bounded %s step on %s for plan item %s and held.", defaultString(mutation.ToolName, "write"), path, defaultString(selection.ID, "plan-item"))
}

func buildView(payload capability.ExitPayload, current workflow.Phase) *tui.BuildView {
	if payload.Capability != capability.NameBuild || payload.Build == nil {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	build := payload.Build
	return &tui.BuildView{
		Source:               "app.build",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		Summary:              payload.Summary,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		PlanItem: tui.BuildPlanItemView{
			ID:     build.PlanItem.ID,
			Text:   build.PlanItem.Text,
			Status: build.PlanItem.Status,
		},
		Step: tui.BuildStepView{
			ID:     build.Step.ID,
			Text:   build.Step.Text,
			Status: build.Step.Status,
		},
		Operation: tui.BuildOperationView{
			Name:             build.Tool.Name,
			Status:           build.Tool.Status,
			Path:             build.Tool.Path,
			ExpectedEffect:   build.Tool.ExpectedEffect,
			DecisionSource:   build.Tool.DecisionSource,
			DecisionAutonomy: build.Tool.DecisionAutonomy,
			DecisionAllowed:  build.Tool.DecisionAllowed,
			ApprovalRequired: build.Tool.ApprovalRequired,
			BytesWritten:     build.Tool.BytesWritten,
			ErrorKind:        build.Tool.ErrorKind,
			ErrorMessage:     build.Tool.ErrorMessage,
		},
		ChangedPaths:     append([]string(nil), build.ChangedPaths...),
		Blockers:         append([]string(nil), build.Blockers...),
		Caveats:          append([]string(nil), build.Caveats...),
		FinalSummary:     build.FinalSummary,
		ArtifactRefs:     buildArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:       buildSourceRefViews(payload.SourceRefs),
		BoundaryRequests: buildBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func buildArtifactRefViews(refs []capability.ArtifactRef) []tui.BuildArtifactRefView {
	views := make([]tui.BuildArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.BuildArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func buildSourceRefViews(refs []capability.SourceRef) []tui.BuildSourceRefView {
	views := make([]tui.BuildSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.BuildSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func buildBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.BuildBoundaryRequestView {
	views := make([]tui.BuildBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.BuildBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
