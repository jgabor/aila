package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

type optimizeArtifactPersistence struct {
	ObjectivePath  string
	ExperimentPath string
	Status         string
	Diagnostic     *diagnostic.Diagnostic
}

func (controller *sessionController) optimizeRequestFromView() capability.Request {
	phase := workflowPhaseFromView(controller.view)
	if phase == "" || phase == workflow.PhaseIdle {
		phase = workflow.PhaseBuild
	}
	contextSummary := sessionStateEvidence(controller.view)
	metadata := map[string]string{
		capability.OptimizeMetadataObjectiveID:       "current-metric-objective",
		capability.OptimizeMetadataObjective:         "Reduce evidence rendering latency without changing workflow authority.",
		capability.OptimizeMetadataExperimentID:      "experiment-current-render-evidence",
		capability.OptimizeMetadataExperimentStatus:  "improved",
		capability.OptimizeMetadataExperimentSummary: "Locked fixture comparison shows render evidence moved in the desired direction.",
		capability.OptimizeMetadataHarnessID:         "fixture-metric-harness",
		capability.OptimizeMetadataHarnessName:       "locked TUI fixture metric comparison",
		capability.OptimizeMetadataHarnessCommand:    "go test ./internal/tui -run TestOptimizeFixtureMetricResult",
		capability.OptimizeMetadataHarnessLocked:     "true",
		capability.OptimizeMetadataMetricName:        "render_evidence_seconds",
		capability.OptimizeMetadataMetricBaseline:    "1.50",
		capability.OptimizeMetadataMetricResult:      "1.20",
		capability.OptimizeMetadataMetricUnit:        "s",
		capability.OptimizeMetadataMetricDirection:   "lower",
		capability.OptimizeMetadataEvidence:          "objective selected from current BUILD context|locked harness command recorded before result comparison|metric result uses app-supplied measured evidence",
		capability.OptimizeMetadataCaveats:           "deterministic app-supplied metric evidence only|provider-backed benchmark execution deferred",
		capability.OptimizeMetadataNextAction:        "Audit the measured optimization result before continuing.",
		capability.OptimizeMetadataDurable:           "true",
	}
	return capability.Request{
		ID:         "command-optimize",
		Capability: capability.NameOptimize,
		Input:      metadata[capability.OptimizeMetadataObjective],
		Phase:      phase,
		SourceRefs: []capability.SourceRef{
			{ID: "optimize-command", Kind: "command", Command: "/optimize", Excerpt: "app-owned optimize command"},
			{ID: "optimize-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 266, LineEnd: 275, Excerpt: "optimize is BUILD-owned metric-driven work"},
			{ID: "optimize-session-state", Kind: "session_state", Excerpt: contextSummary},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openOptimizeView() []diagnostic.Diagnostic {
	request := controller.optimizeRequestFromView()
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistOptimizePayload(controller.runner.model.LastCapability)
	turn.Optimize = optimizeView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Optimize != nil {
		turn.StatusDetail = "optimize capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model, controller.workspacePath)
	if persistence.Diagnostic == nil {
		return nil
	}
	return []diagnostic.Diagnostic{*persistence.Diagnostic}
}

func (controller *sessionController) persistOptimizePayload(payload capability.ExitPayload) optimizeArtifactPersistence {
	if payload.Optimize == nil || (strings.TrimSpace(payload.Optimize.ObjectiveDocument) == "" && strings.TrimSpace(payload.Optimize.ExperimentDocument) == "") {
		return optimizeArtifactPersistence{Status: "not_written"}
	}
	return writeOptimizeArtifacts(controller.ctx, controller.workspacePath, payload.Optimize.ObjectiveDocument, payload.Optimize.ExperimentDocument)
}

func writeOptimizeArtifacts(ctx context.Context, workspacePath string, objectiveDocument string, experimentDocument string) optimizeArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return optimizeArtifactPersistence{Status: "recovery_needed", Diagnostic: optimizeArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	result := optimizeArtifactPersistence{Status: "written"}
	if strings.TrimSpace(objectiveDocument) != "" {
		artifact, err := store.WriteArtifact(ctx, state.ArtifactObjective, state.OwnerApp, []byte(objectiveDocument))
		if err != nil {
			return optimizeArtifactPersistence{Status: "recovery_needed", Diagnostic: optimizeArtifactDiagnostic(err)}
		}
		result.ObjectivePath = artifact.Path
	}
	if strings.TrimSpace(experimentDocument) != "" {
		artifact, err := store.WriteArtifact(ctx, state.ArtifactExperiments, state.OwnerApp, []byte(experimentDocument))
		if err != nil {
			return optimizeArtifactPersistence{Status: "recovery_needed", Diagnostic: optimizeArtifactDiagnostic(err)}
		}
		result.ExperimentPath = artifact.Path
	}
	return result
}

func optimizeArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "optimize artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactExperiments,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func optimizeView(payload capability.ExitPayload, current workflow.Phase, persistence optimizeArtifactPersistence) *tui.OptimizeView {
	if payload.Capability != capability.NameOptimize {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.OptimizeOutput
	if payload.Optimize != nil {
		output = *payload.Optimize
	}
	objectivePath := valueOr(output.ObjectiveArtifactPath, ".aila/artifacts/objective.md")
	experimentPath := valueOr(output.ExperimentArtifactPath, ".aila/artifacts/experiments.md")
	if persistence.ObjectivePath != "" {
		objectivePath = persistence.ObjectivePath
	}
	if persistence.ExperimentPath != "" {
		experimentPath = persistence.ExperimentPath
	}
	caveats := append([]string(nil), output.Caveats...)
	if len(caveats) == 0 && payload.Optimize == nil {
		caveats = append([]string(nil), payload.Concerns...)
	}
	return &tui.OptimizeView{
		Source:                 "app.optimize",
		Capability:             string(payload.Capability),
		Signal:                 string(payload.Signal),
		CurrentPhase:           current.String(),
		Summary:                payload.Summary,
		RecommendedSuccessor:   string(payload.RecommendedSuccessor),
		SuccessorValid:         recommendation.SuccessorValid,
		TransitionClaimed:      false,
		DisplayOnly:            true,
		Objective:              tui.OptimizeObjectiveView{ID: output.Objective.ID, Text: output.Objective.Text},
		Experiment:             tui.OptimizeExperimentView{ID: output.Experiment.ID, Status: output.Experiment.Status, Summary: output.Experiment.Summary},
		Harness:                tui.OptimizeHarnessView{ID: output.Harness.ID, Name: output.Harness.Name, Command: output.Harness.Command, Locked: output.Harness.Locked},
		Metric:                 tui.OptimizeMetricView{Name: output.Metric.Name, Baseline: output.Metric.Baseline, Result: output.Metric.Result, Unit: output.Metric.Unit, Direction: output.Metric.Direction, Improvement: output.Metric.Improvement},
		Evidence:               optimizeEvidenceViews(output.Evidence),
		Caveats:                caveats,
		NeededInput:            payload.NeededInput,
		NextAction:             payload.NextAction,
		ObjectiveArtifactPath:  objectivePath,
		ExperimentArtifactPath: experimentPath,
		ArtifactStatus:         artifactStatus,
		ArtifactRefs:           optimizeArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:             optimizeSourceRefViews(payload.SourceRefs),
		BoundaryRequests:       optimizeBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func optimizeEvidenceViews(evidence []capability.OptimizeEvidence) []tui.OptimizeEvidenceView {
	views := make([]tui.OptimizeEvidenceView, 0, len(evidence))
	for _, item := range evidence {
		views = append(views, tui.OptimizeEvidenceView{ID: item.ID, Summary: item.Summary, SourceRefID: item.SourceRefID})
	}
	return views
}

func optimizeArtifactRefViews(refs []capability.ArtifactRef) []tui.OptimizeArtifactRefView {
	views := make([]tui.OptimizeArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.OptimizeArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func optimizeSourceRefViews(refs []capability.SourceRef) []tui.OptimizeSourceRefView {
	views := make([]tui.OptimizeSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.OptimizeSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func optimizeBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.OptimizeBoundaryRequestView {
	views := make([]tui.OptimizeBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.OptimizeBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
