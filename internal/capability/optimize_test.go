package capability

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestOptimizeCapabilityEmitsMetricPayloadWithLockedHarness(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "optimize-ready",
		Capability: NameOptimize,
		Input:      "Reduce render evidence latency.",
		Phase:      workflow.PhaseBuild,
		SourceRefs: []SourceRef{{ID: "objective-ref", Kind: "objective", Excerpt: "Reduce render evidence latency."}},
		Metadata: map[string]string{
			OptimizeMetadataObjectiveID:      "render-latency",
			OptimizeMetadataObjective:        "Reduce render evidence latency.",
			OptimizeMetadataExperimentID:     "experiment-render-latency",
			OptimizeMetadataExperimentStatus: "improved",
			OptimizeMetadataHarnessName:      "locked fixture comparison",
			OptimizeMetadataHarnessCommand:   "go test ./internal/tui -run TestOptimizeFixtureMetricResult",
			OptimizeMetadataHarnessLocked:    "true",
			OptimizeMetadataMetricName:       "render_seconds",
			OptimizeMetadataMetricBaseline:   "1.50",
			OptimizeMetadataMetricResult:     "1.20",
			OptimizeMetadataMetricUnit:       "s",
			OptimizeMetadataMetricDirection:  "lower",
			OptimizeMetadataEvidence:         "semantic snapshot exposes metric result|locked harness command recorded",
			OptimizeMetadataCaveats:          "deterministic app-supplied metric evidence only",
			OptimizeMetadataDurable:          "true",
		},
	}

	payload, err := OptimizeCapability{}.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run optimize capability: %v", err)
	}
	if payload.Capability != NameOptimize || payload.Signal != ExitComplete || !payload.Attempted || payload.Optimize == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Optimize.Objective.ID != "render-latency" || payload.Optimize.Experiment.Status != "improved" || !payload.Optimize.Harness.Locked || payload.Optimize.Metric.Name != "render_seconds" || payload.Optimize.Metric.Improvement == "" {
		t.Fatalf("optimize output = %+v", payload.Optimize)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit {
		t.Fatalf("recommended successor = %q, want audit", payload.RecommendedSuccessor)
	}
	if len(payload.ArtifactRefs) != 2 || len(payload.SourceRefs) == 0 || len(payload.BoundaryRequests) == 0 || payload.Optimize.ObjectiveDocument == "" || payload.Optimize.ExperimentDocument == "" {
		t.Fatalf("missing refs, boundaries, or documents: refs=%+v artifacts=%+v boundaries=%+v optimize=%+v", payload.SourceRefs, payload.ArtifactRefs, payload.BoundaryRequests, payload.Optimize)
	}
}

func TestOptimizeCapabilityWaitsForLockedMetricEvidenceWithoutInventingMeasurement(t *testing.T) {
	t.Parallel()

	payload, err := OptimizeCapability{}.Run(context.Background(), Request{ID: "optimize-wait", Capability: NameOptimize, Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("Run optimize capability: %v", err)
	}
	if payload.Signal != ExitWaiting || payload.NeededInput == "" || payload.Attempted || payload.Optimize != nil || payload.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", payload)
	}
	if !strings.Contains(payload.Summary, "locked harness") {
		t.Fatalf("waiting summary = %q", payload.Summary)
	}
}

func TestOptimizeCapabilityFlagsUnlockedHarnessAndHoldsInBuild(t *testing.T) {
	t.Parallel()

	payload, err := OptimizeCapability{}.Run(context.Background(), Request{
		ID:         "optimize-unlocked",
		Capability: NameOptimize,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			OptimizeMetadataObjective:        "Reduce prompt latency.",
			OptimizeMetadataExperimentStatus: "regressed",
			OptimizeMetadataHarnessName:      "ad hoc timing",
			OptimizeMetadataHarnessLocked:    "false",
			OptimizeMetadataMetricName:       "latency_seconds",
			OptimizeMetadataMetricBaseline:   "1.20",
			OptimizeMetadataMetricResult:     "1.60",
			OptimizeMetadataMetricUnit:       "s",
		},
	})
	if err != nil {
		t.Fatalf("Run optimize capability: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.Optimize == nil || payload.RecommendedSuccessor != workflow.PhaseBuild {
		t.Fatalf("flagged payload = %+v", payload)
	}
	if payload.Optimize.Harness.Locked || len(payload.Optimize.Caveats) == 0 || !strings.Contains(payload.NextAction, "Lock the harness") {
		t.Fatalf("flagged optimize output = %+v next=%q", payload.Optimize, payload.NextAction)
	}
}

func TestRunBuiltInDispatchesOptimizeCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{Capability: NameOptimize, Phase: workflow.PhaseBuild, Metadata: map[string]string{
		OptimizeMetadataObjective:       "Reduce render latency.",
		OptimizeMetadataHarnessName:     "locked fixture comparison",
		OptimizeMetadataHarnessLocked:   "true",
		OptimizeMetadataMetricName:      "render_seconds",
		OptimizeMetadataMetricBaseline:  "1.50",
		OptimizeMetadataMetricResult:    "1.20",
		OptimizeMetadataMetricDirection: "lower",
	}})
	if err != nil {
		t.Fatalf("RunBuiltIn optimize: %v", err)
	}
	if payload.Capability != NameOptimize || payload.Optimize == nil {
		t.Fatalf("RunBuiltIn payload = %+v", payload)
	}
}
