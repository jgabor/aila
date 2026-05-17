package capability

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	BuildMetadataPlanItemID       = "build_plan_item_id"
	BuildMetadataPlanItemText     = "build_plan_item_text"
	BuildMetadataStepID           = "build_step_id"
	BuildMetadataStepText         = "build_step_text"
	BuildMetadataToolName         = "build_tool_name"
	BuildMetadataToolStatus       = "build_tool_status"
	BuildMetadataTargetPath       = "build_target_path"
	BuildMetadataExpectedEffect   = "build_expected_effect"
	BuildMetadataDecisionSource   = "build_decision_source"
	BuildMetadataDecisionAutonomy = "build_decision_autonomy"
	BuildMetadataDecisionAllowed  = "build_decision_allowed"
	BuildMetadataApprovalRequired = "build_approval_required"
	BuildMetadataBytesWritten     = "build_bytes_written"
	BuildMetadataErrorKind        = "build_error_kind"
	BuildMetadataErrorMessage     = "build_error_message"
	BuildMetadataFinalSummary     = "build_final_summary"
	BuildMetadataBlockers         = "build_blockers"
	BuildMetadataCaveats          = "build_caveats"
)

// BuildCapability summarizes one bounded build step executed by app-owned effects.
type BuildCapability struct{}

// BuildOutput is the typed build data carried by a build capability exit.
type BuildOutput struct {
	PlanItem     BuildPlanItem
	Step         BuildStep
	Tool         BuildTool
	ChangedPaths []string
	Blockers     []string
	Caveats      []string
	FinalSummary string
	SourceRefs   []SourceRef
}

// BuildPlanItem records the single plan item selected for a bounded build step.
type BuildPlanItem struct {
	ID     string
	Text   string
	Status string
}

// BuildStep records one bounded execution step and its held result.
type BuildStep struct {
	ID     string
	Text   string
	Status string
}

// BuildTool records app-owned tool, permission, and mutation evidence.
type BuildTool struct {
	Name             string
	Status           string
	Path             string
	ExpectedEffect   string
	DecisionSource   string
	DecisionAutonomy string
	DecisionAllowed  bool
	ApprovalRequired bool
	BytesWritten     int
	ErrorKind        string
	ErrorMessage     string
}

// Name returns the fixed capability identity.
func (BuildCapability) Name() Name {
	return NameBuild
}

// OwningPhase returns BUILD because the capability executes scoped work.
func (BuildCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseBuild
}

// Run emits one build payload. World-touching work must already have been handled by app/runtime effects.
func (BuildCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run build capability: %w", err)
	}
	request = normalizeBuildRequest(request)
	invocation := NewInvocation(request)

	if buildMetadata(request, BuildMetadataPlanItemText, "") == "" {
		payload := ExitPayload{
			Capability:       NameBuild,
			Signal:           ExitWaiting,
			Summary:          "Build needs an active plan item before it can execute a bounded step.",
			NeededInput:      "Create or select one plan item, then run build again.",
			NextAction:       "Run plan or select one pending plan item before build.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: buildBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	output := buildOutput(request)
	signal := ExitComplete
	if len(output.Blockers) > 0 || buildToolFailed(output.Tool) {
		signal = ExitFlagged
	}
	successor := workflow.Phase("")
	if signal == ExitComplete && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseAudit) == nil {
		successor = workflow.PhaseAudit
	} else if signal == ExitFlagged && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseBuild) == nil {
		successor = workflow.PhaseBuild
	}

	payload := ExitPayload{
		Capability:           NameBuild,
		Signal:               signal,
		Summary:              buildSummary(output, signal),
		Concerns:             append(append([]string(nil), output.Blockers...), output.Caveats...),
		Attempted:            output.Step.Status != "",
		NextAction:           buildNextAction(output, signal),
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{
			{ID: "plan-artifact", Kind: "state_artifact", Path: ".aila/artifacts/plan.md"},
			{ID: "history-log", Kind: "state_event_log", Path: ".aila/history/fake-events.jsonl"},
		},
		SourceRefs:       cloneSourceRefs(output.SourceRefs),
		BoundaryRequests: buildBoundaryRequests(request),
		Build:            &output,
	}
	return invocation.Emit(payload)
}

func normalizeBuildRequest(request Request) Request {
	request.Capability = NameBuild
	if request.Phase == "" {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildOutput(request Request) BuildOutput {
	tool := BuildTool{
		Name:             buildMetadata(request, BuildMetadataToolName, "write"),
		Status:           buildMetadata(request, BuildMetadataToolStatus, "pending"),
		Path:             buildMetadata(request, BuildMetadataTargetPath, ""),
		ExpectedEffect:   buildMetadata(request, BuildMetadataExpectedEffect, ""),
		DecisionSource:   buildMetadata(request, BuildMetadataDecisionSource, ""),
		DecisionAutonomy: buildMetadata(request, BuildMetadataDecisionAutonomy, ""),
		DecisionAllowed:  buildBoolMetadata(request, BuildMetadataDecisionAllowed),
		ApprovalRequired: buildBoolMetadata(request, BuildMetadataApprovalRequired),
		BytesWritten:     buildIntMetadata(request, BuildMetadataBytesWritten),
		ErrorKind:        buildMetadata(request, BuildMetadataErrorKind, ""),
		ErrorMessage:     buildMetadata(request, BuildMetadataErrorMessage, ""),
	}
	blockers := buildListMetadata(request, BuildMetadataBlockers)
	if tool.Status == "denied" && tool.ErrorMessage != "" {
		blockers = append(blockers, tool.ErrorMessage)
	}
	if tool.Status == "failed" && tool.ErrorMessage != "" {
		blockers = append(blockers, tool.ErrorMessage)
	}
	changedPaths := []string(nil)
	if tool.Status == "completed" && strings.TrimSpace(tool.Path) != "" {
		changedPaths = []string{tool.Path}
	}
	caveats := buildListMetadata(request, BuildMetadataCaveats)
	if len(caveats) == 0 {
		caveats = append(caveats, "bounded build executed one step and then held")
	}
	finalSummary := buildMetadata(request, BuildMetadataFinalSummary, "")
	if finalSummary == "" {
		finalSummary = buildDefaultFinalSummary(tool)
	}
	sourceRefs := buildSourceRefs(request)
	return BuildOutput{
		PlanItem: BuildPlanItem{
			ID:     buildMetadata(request, BuildMetadataPlanItemID, "plan-item"),
			Text:   buildMetadata(request, BuildMetadataPlanItemText, request.Input),
			Status: "active",
		},
		Step: BuildStep{
			ID:     buildMetadata(request, BuildMetadataStepID, "bounded-step"),
			Text:   buildMetadata(request, BuildMetadataStepText, buildMetadata(request, BuildMetadataExpectedEffect, "execute one bounded build step")),
			Status: tool.Status,
		},
		Tool:         tool,
		ChangedPaths: changedPaths,
		Blockers:     blockers,
		Caveats:      caveats,
		FinalSummary: finalSummary,
		SourceRefs:   sourceRefs,
	}
}

func buildToolFailed(tool BuildTool) bool {
	return tool.Status == "denied" || tool.Status == "failed" || tool.ErrorKind != "" || tool.ErrorMessage != ""
}

func buildSummary(output BuildOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Build held after %s for plan item %s with %d blocker(s).", output.Step.Status, output.PlanItem.ID, len(output.Blockers))
	}
	return fmt.Sprintf("Build completed one bounded step for plan item %s and held.", output.PlanItem.ID)
}

func buildNextAction(output BuildOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return "Review blockers and caveats before running another build step."
	}
	if len(output.ChangedPaths) > 0 {
		return "Review the changed path and audit the result before continuing."
	}
	return "Review the final summary before continuing."
}

func buildDefaultFinalSummary(tool BuildTool) string {
	switch tool.Status {
	case "completed":
		return fmt.Sprintf("Executed %s on %s and held.", tool.Name, tool.Path)
	case "denied", "failed":
		return fmt.Sprintf("Held after %s could not complete on %s.", tool.Name, tool.Path)
	default:
		return "Build step is ready but has not completed."
	}
}

func buildBoundaryRequests(request Request) []BoundaryRequest {
	target := buildMetadata(request, BuildMetadataTargetPath, "planned build target")
	return []BoundaryRequest{
		request.RequestStateAccess("plan.current", "build selects one active plan item from app-owned plan state"),
		request.RequestToolExecution("write", target, "build executes one bounded step through the runtime tool effect"),
		request.RequestPermissionCheck("write", target, "build requires the permission gate before workspace mutation"),
		request.RequestStateWrite("history", "build records mutation and runtime evidence through app-owned history state"),
		request.RequestArtifactAccess("plan", "build keeps plan artifact references visible for follow-up"),
	}
}

func buildSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	ensureRef := func(id, kind, excerpt string) {
		if strings.TrimSpace(excerpt) == "" || hasSourceRef(refs, id) {
			return
		}
		refs = append(refs, SourceRef{ID: id, Kind: kind, Excerpt: excerpt})
	}
	ensureRef("build-plan-item", "plan_item", buildMetadata(request, BuildMetadataPlanItemText, ""))
	ensureRef("build-tool-result", "tool_result", buildDefaultFinalSummary(BuildTool{
		Name:   buildMetadata(request, BuildMetadataToolName, "write"),
		Status: buildMetadata(request, BuildMetadataToolStatus, "pending"),
		Path:   buildMetadata(request, BuildMetadataTargetPath, ""),
	}))
	return refs
}

func buildMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func buildBoolMetadata(request Request, key string) bool {
	value := strings.ToLower(buildMetadata(request, key, ""))
	return value == "true" || value == "1" || value == "yes"
}

func buildIntMetadata(request Request, key string) int {
	value := buildMetadata(request, key, "")
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func buildListMetadata(request Request, key string) []string {
	value := buildMetadata(request, key, "")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "\n")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
