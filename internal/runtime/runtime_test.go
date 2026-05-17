package runtime

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/utility"
	"github.com/jgabor/aila/internal/workflow"
)

func TestUpdateHandlesPromptDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle}
	firstModel, firstEffects := Update(model, PromptSubmitted{Text: "explain status"})
	secondModel, secondEffects := Update(model, PromptSubmitted{Text: "explain status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for prompt model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for prompt effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}

	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.NextOperation != 1 {
		t.Fatalf("NextOperation = %d, want 1", firstModel.NextOperation)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-1",
		Kind:    OperationPrompt,
		Subject: "explain status",
		Source:  "user",
	})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "prompt", Text: "explain status"}}) {
		t.Fatalf("Transcript = %#v", got)
	}

	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakePromptEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakePromptEffect", firstEffects[0])
	}
	if effect.Prompt != "explain status" {
		t.Fatalf("Prompt = %q", effect.Prompt)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-1",
		Kind:    OperationPrompt,
		Subject: "explain status",
		Source:  "user",
	})
}

func TestUpdateHandlesCommandDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle, NextOperation: 7}
	firstModel, firstEffects := Update(model, CommandSelected{Name: "status"})
	secondModel, secondEffects := Update(model, CommandSelected{Name: "status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for command model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for command effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.LastCommand != "status" {
		t.Fatalf("LastCommand = %q, want status", firstModel.LastCommand)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-8",
		Kind:    OperationCommand,
		Subject: "status",
		Source:  "user",
	})
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakeCommandEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakeCommandEffect", firstEffects[0])
	}
	if effect.Command != "status" {
		t.Fatalf("Command = %q", effect.Command)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-8",
		Kind:    OperationCommand,
		Subject: "status",
		Source:  "user",
	})
}

func TestUpdateRoutesBriefCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "brief-status",
		Capability: capability.NameBrief,
		Metadata: map[string]string{
			capability.BriefMetadataRuntimeStatus:       "idle",
			capability.BriefMetadataProjectStoreStatus:  "initialized",
			capability.BriefMetadataHistoryState:        "loaded",
			capability.BriefMetadataContextStatus:       "current",
			capability.BriefMetadataHealthStatus:        "available",
			capability.BriefMetadataSuggestedNextAction: "Continue the current task.",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameBrief {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "brief", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameBrief || completed.Payload.RecommendedSuccessor != "" || completed.Payload.NextAction != "Continue the current task." {
		t.Fatalf("brief payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameBrief {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
	if model.LastCapability.RecommendedSuccessor != "" {
		t.Fatalf("brief completion recommended successor %q", model.LastCapability.RecommendedSuccessor)
	}
}

func TestUpdateRoutesVisionCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "vision-status",
		Capability: capability.NameVision,
		Input:      "Shape Aila's long-term project direction.",
		Phase:      workflow.PhaseEnvision,
		Metadata: map[string]string{
			capability.VisionMetadataNorthStar:       "Aila stays focused on terminal coding-agent work.",
			capability.VisionMetadataRecommendedNext: workflow.PhasePlan.String(),
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameVision {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "vision", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameVision || completed.Payload.RecommendedSuccessor != workflow.PhasePlan || completed.Payload.Vision == nil {
		t.Fatalf("vision payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameVision || model.LastCapability.Vision == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesDiscussCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "discuss-status",
		Capability: capability.NameDiscuss,
		Input:      "Decide whether Aila should plan next.",
		Phase:      workflow.PhaseDeliberate,
		Metadata: map[string]string{
			capability.DiscussMetadataQuestion:        "Should Aila plan before building?",
			capability.DiscussMetadataSelected:        "Plan the scoped next step",
			capability.DiscussMetadataRecommendedNext: workflow.PhasePlan.String(),
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameDiscuss {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "discuss", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameDiscuss || completed.Payload.RecommendedSuccessor != workflow.PhasePlan || completed.Payload.Discuss == nil {
		t.Fatalf("discuss payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameDiscuss || model.LastCapability.Discuss == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesResearchCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "research-status",
		Capability: capability.NameResearch,
		Input:      "Research cross-cutting context patterns.",
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.ResearchMetadataTopic:    "cross-cutting context patterns",
			capability.ResearchMetadataPatterns: "Fold evidence into context|Keep workflow transitions FSM-owned",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameResearch {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "research", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameResearch || completed.Payload.RecommendedSuccessor != "" || completed.Payload.Research == nil {
		t.Fatalf("research payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameResearch || model.LastCapability.Research == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesProfileCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "profile-status",
		Capability: capability.NameProfile,
		Input:      "Profile decision patterns.",
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.ProfileMetadataSubject:           "Aila decision profile",
			capability.ProfileMetadataDecisionSignals:   "Prefer bounded roadmap slices",
			capability.ProfileMetadataUpdateSuggestions: "Keep validation evidence close",
			capability.ProfileMetadataEvidence:          "Recent runs used direct validation",
			capability.ProfileMetadataDurable:           "true",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameProfile {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "profile", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameProfile || completed.Payload.RecommendedSuccessor != "" || completed.Payload.Profile == nil {
		t.Fatalf("profile payload = %+v", completed.Payload)
	}
	if !hasCapabilityBoundary(completed.Payload.BoundaryRequests, capability.BoundaryStateWrite, "profile") {
		t.Fatalf("profile boundary requests = %+v, want state write profile", completed.Payload.BoundaryRequests)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameProfile || model.LastCapability.Profile == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
	if model.LastCapability.RecommendedSuccessor != "" {
		t.Fatalf("profile completion recommended successor %q", model.LastCapability.RecommendedSuccessor)
	}
}

func hasCapabilityBoundary(requests []capability.BoundaryRequest, kind capability.BoundaryKind, target string) bool {
	for _, request := range requests {
		if request.Kind == kind && request.Target == target {
			return true
		}
	}
	return false
}

func TestUpdateRoutesPlanCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "plan-status",
		Capability: capability.NamePlan,
		Input:      "prepare the current scoped work",
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.PlanMetadataProjectState: "project store initialized",
			capability.PlanMetadataSessionState: "runtime idle with roadmap context",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NamePlan {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "plan", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NamePlan || completed.Payload.RecommendedSuccessor != workflow.PhasePlan || completed.Payload.Plan == nil {
		t.Fatalf("plan payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NamePlan || model.LastCapability.Plan == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesBuildCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "build-status",
		Capability: capability.NameBuild,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.BuildMetadataPlanItemID:       "implement",
			capability.BuildMetadataPlanItemText:     "Implement one bounded step",
			capability.BuildMetadataToolName:         "write",
			capability.BuildMetadataToolStatus:       "completed",
			capability.BuildMetadataTargetPath:       "docs/aila-build-output.md",
			capability.BuildMetadataExpectedEffect:   "create bounded build output",
			capability.BuildMetadataDecisionSource:   "autonomy_policy",
			capability.BuildMetadataDecisionAutonomy: "write",
			capability.BuildMetadataDecisionAllowed:  "true",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameBuild {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "build", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameBuild || completed.Payload.RecommendedSuccessor != workflow.PhaseAudit || completed.Payload.Build == nil {
		t.Fatalf("build payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameBuild || model.LastCapability.Build == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesOptimizeCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "optimize-metric",
		Capability: capability.NameOptimize,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.OptimizeMetadataObjective:       "Reduce render evidence latency.",
			capability.OptimizeMetadataHarnessName:     "locked fixture comparison",
			capability.OptimizeMetadataHarnessLocked:   "true",
			capability.OptimizeMetadataMetricName:      "render_seconds",
			capability.OptimizeMetadataMetricBaseline:  "1.50",
			capability.OptimizeMetadataMetricResult:    "1.20",
			capability.OptimizeMetadataMetricDirection: "lower",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameOptimize {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "optimize", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameOptimize || completed.Payload.RecommendedSuccessor != workflow.PhaseAudit || completed.Payload.Optimize == nil {
		t.Fatalf("optimize payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameOptimize || model.LastCapability.Optimize == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesDocumentCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "document-docs",
		Capability: capability.NameDocument,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.DocumentMetadataTargetPath:       "docs/aila-documentation-output.md",
			capability.DocumentMetadataSourceBehavior:   "/document routes docs through mutation safety.",
			capability.DocumentMetadataPlanSummary:      "Record the document safety path.",
			capability.DocumentMetadataToolStatus:       "completed",
			capability.DocumentMetadataDecisionAllowed:  "true",
			capability.DocumentMetadataDecisionAutonomy: "write",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameDocument {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "document", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameDocument || completed.Payload.RecommendedSuccessor != workflow.PhaseAudit || completed.Payload.Document == nil {
		t.Fatalf("document payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameDocument || model.LastCapability.Document == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesDesignCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "design-system",
		Capability: capability.NameDesign,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			capability.DesignMetadataGoalID:         "terminal-design",
			capability.DesignMetadataGoalSummary:    "Keep terminal UI visual decisions durable.",
			capability.DesignMetadataSurface:        "terminal-ui",
			capability.DesignMetadataDecisions:      "hierarchy::information architecture::Keep phase context above evidence.::Orientation comes before detail.",
			capability.DesignMetadataReviewPrompts:  "wide-layout::Does the wide layout preserve hierarchy?::docs/mockup-desktop.png",
			capability.DesignMetadataCaveats:        "screenshots are review aids, not correctness contracts",
			capability.DesignMetadataNextAction:     "Audit the design artifact before continuing.",
			capability.DesignMetadataDurable:        "true",
			capability.DesignMetadataArtifactStatus: "planned",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameDesign {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "design", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameDesign || completed.Payload.RecommendedSuccessor != workflow.PhaseAudit || completed.Payload.Design == nil {
		t.Fatalf("design payload = %+v", completed.Payload)
	}
	if len(completed.Payload.Design.Decisions) != 1 || len(completed.Payload.Design.ReviewPrompts) != 1 || completed.Payload.Design.DesignArtifact == "" {
		t.Fatalf("design output = %+v", completed.Payload.Design)
	}
	if len(completed.Payload.ArtifactRefs) != 2 || len(completed.Payload.BoundaryRequests) != 3 {
		t.Fatalf("design refs/boundaries = refs:%+v boundaries:%+v", completed.Payload.ArtifactRefs, completed.Payload.BoundaryRequests)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameDesign || model.LastCapability.Design == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestUpdateRoutesAuditCapabilityThroughEffectBoundary(t *testing.T) {
	t.Parallel()

	request := capability.Request{
		ID:         "audit-review",
		Capability: capability.NameAudit,
		Phase:      workflow.PhaseAudit,
		SourceRefs: []capability.SourceRef{{ID: "review-diff", Kind: "diff", Path: "internal/app/inspection.go", Excerpt: "changed file: internal/app/inspection.go"}},
		Metadata: map[string]string{
			capability.AuditMetadataFindingSeverity: "warning",
			capability.AuditMetadataFindingTitle:    "Review current changes",
			capability.AuditMetadataFindingMessage:  "Changed files need review before continuing.",
			capability.AuditMetadataEvidenceState:   "diff_available",
		},
	}
	model, effects := Update(Model{Status: StatusIdle}, CapabilityProposed{Request: request})
	if model.Status != StatusActive || model.ActiveCapability.Capability != capability.NameAudit {
		t.Fatalf("capability model = status:%q active:%+v", model.Status, model.ActiveCapability)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(CapabilityEffect)
	if !ok {
		t.Fatalf("effect type = %T, want CapabilityEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-1", Kind: OperationCapability, Subject: "audit", Source: "runtime.capability"})

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	completed, ok := messages[0].(CapabilityCompleted)
	if !ok {
		t.Fatalf("message type = %T, want CapabilityCompleted", messages[0])
	}
	if completed.Payload.Capability != capability.NameAudit || completed.Payload.RecommendedSuccessor != workflow.PhaseBuild || completed.Payload.Audit == nil {
		t.Fatalf("audit payload = %+v", completed.Payload)
	}

	model, effects = Update(model, completed)
	if len(effects) != 0 || model.Status != StatusIdle || model.ActiveCapability.Capability != "" || model.LastCapability.Capability != capability.NameAudit || model.LastCapability.Audit == nil {
		t.Fatalf("completed capability model = status:%q active:%+v last:%+v effects:%d", model.Status, model.ActiveCapability, model.LastCapability, len(effects))
	}
}

func TestBriefCapabilityProposalQueuesWhileRuntimeActive(t *testing.T) {
	t.Parallel()

	model, effects := Update(Model{Status: StatusActive}, CapabilityProposed{Request: capability.Request{Capability: capability.NameBrief}})
	if len(effects) != 0 || len(model.Queued) != 1 || model.Queued[0].Kind != "capability" || model.Queued[0].Text != "brief" {
		t.Fatalf("queued capability model = queued:%+v effects:%d", model.Queued, len(effects))
	}
}

func TestUpdateHandlesCompactContextProposalDeterministically(t *testing.T) {
	t.Parallel()

	request := CompactContextRequest{
		Blocks:     []CompactContextBlock{{ID: "block-1", Kind: "prompt", Title: "Prompt", Text: "review compact", SourceRefIDs: []string{"prompt-1"}}},
		SourceRefs: []CompactSourceRef{{ID: "prompt-1", Kind: "prompt", Excerpt: "review compact"}},
		Claims:     []CompactContextClaim{{Text: "manual compact requested", SourceRefIDs: []string{"prompt-1"}}},
		Budget:     CompactContextBudget{BlockCount: 1, SourceRefCount: 1, ClaimCount: 1, UsedBytes: 14},
		Source:     CompactSourceMetadata{Caller: "test.compact", RequestID: "compact-1", Description: "manual /compact command", Mode: CompactModeManual},
	}
	model := Model{Status: StatusIdle, NextOperation: 2}
	firstModel, firstEffects := Update(model, CompactContextProposed{Request: request})
	secondModel, secondEffects := Update(model, CompactContextProposed{Request: request})

	if !reflect.DeepEqual(firstModel, secondModel) || !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("compact update not deterministic:\nfirst:  %#v %#v\nsecond: %#v %#v", firstModel, firstEffects, secondModel, secondEffects)
	}
	if firstModel.Status != StatusActive || firstModel.LastCommand != "compact" || !reflect.DeepEqual(firstModel.ActiveCompact, request) {
		t.Fatalf("compact model = status:%q last:%q active:%+v", firstModel.Status, firstModel.LastCommand, firstModel.ActiveCompact)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{ID: "op-3", Kind: OperationCompact, Subject: "manual context compaction", Source: "user"})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "command", Text: "compact"}}) {
		t.Fatalf("compact transcript = %#v", got)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(CompactContextEffect)
	if !ok || !reflect.DeepEqual(effect.Request, request) {
		t.Fatalf("effect = %#v, want compact effect with request", firstEffects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-3", Kind: OperationCompact, Subject: "manual context compaction", Source: "user"})
}

func TestUpdateAppliesCompactContextResult(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-4", Kind: OperationCompact, Subject: "manual context compaction", Source: "user"}
	result := CompactContextResult{
		Status:     "completed",
		Summary:    "manual compaction preserved 1 source refs",
		SourceRefs: []CompactSourceRef{{ID: "prompt-1", Kind: "prompt", Excerpt: "review compact"}},
		Budget:     CompactContextBudget{BlockCount: 1, SourceRefCount: 1, UsedBytes: 64},
	}
	model, effects := Update(Model{Status: StatusActive, ActiveOperation: operation}, CompactContextCompleted{Operation: operation, Result: result})
	if len(effects) != 0 || model.Status != StatusIdle || model.Result != result.Summary || !reflect.DeepEqual(model.LastCompact, result) {
		t.Fatalf("compact completed model/effects = %+v %#v", model, effects)
	}
	if got := model.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "result", Text: result.Summary}}) {
		t.Fatalf("transcript = %#v", got)
	}
}

func TestUpdateStartsBackgroundCompactOnlyWhilePrimaryRuntimeCanYield(t *testing.T) {
	t.Parallel()

	request := CompactContextRequest{
		Blocks:     []CompactContextBlock{{ID: "block-1", Kind: "prompt", Text: "background compact", SourceRefIDs: []string{"prompt-1"}}},
		SourceRefs: []CompactSourceRef{{ID: "prompt-1", Kind: "prompt", Excerpt: "background compact"}},
		Claims:     []CompactContextClaim{{Text: "background compact requested", SourceRefIDs: []string{"prompt-1"}}},
		Budget:     CompactContextBudget{BlockCount: 1, SourceRefCount: 1, ClaimCount: 1, UsedBytes: 18},
	}
	base := Model{Status: StatusIdle, NextOperation: 8, Result: "previous result", Transcript: []TranscriptEntry{{Kind: "result", Text: "previous result"}}}
	updated, effects := Update(base, BackgroundCompactContextProposed{Request: request})

	if updated.Status != base.Status || updated.Result != base.Result || !reflect.DeepEqual(updated.Transcript, base.Transcript) || updated.ActiveOperation.Kind != "" {
		t.Fatalf("background compact changed primary state = %+v", updated)
	}
	if updated.ActiveCompact.Source.Mode != CompactModeBackground || updated.LastCompact.Status != "running" || updated.LastCompact.Source.Mode != CompactModeBackground {
		t.Fatalf("background compact state = active %+v last %+v", updated.ActiveCompact, updated.LastCompact)
	}
	if updated.NextOperation != 9 {
		t.Fatalf("NextOperation = %d, want 9", updated.NextOperation)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want one compact effect", len(effects))
	}
	effect, ok := effects[0].(CompactContextEffect)
	if !ok {
		t.Fatalf("effect = %#v, want compact effect", effects[0])
	}
	if effect.Request.Source.Mode != CompactModeBackground || effect.Request.Source.Caller != "app.compact.background" {
		t.Fatalf("background effect request source = %+v", effect.Request.Source)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-9", Kind: OperationCompact, Subject: "background context compaction", Source: "runtime.utility"})
}

func TestUpdateBlocksBackgroundCompactWhenPrimaryRuntimeCannotYield(t *testing.T) {
	t.Parallel()

	request := CompactContextRequest{Blocks: []CompactContextBlock{{ID: "block-1", Text: "background compact"}}}
	cases := []struct {
		name  string
		model Model
		want  utility.DenialReason
	}{
		{name: "primary active", model: Model{Status: StatusActive}, want: utility.DenialPrimaryBusy},
		{name: "active operation", model: Model{Status: StatusIdle, ActiveOperation: OperationMetadata{Kind: OperationBash}}, want: utility.DenialActiveOperation},
		{name: "approval pending", model: Model{Status: StatusIdle, PendingApproval: ApprovalProposal{ID: "approval-1"}}, want: utility.DenialApprovalPending},
		{name: "queued input", model: Model{Status: StatusIdle, Queued: []QueuedEntry{{Kind: "prompt", Text: "queued"}}}, want: utility.DenialQueuedUserInput},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			updated, effects := Update(tc.model, BackgroundCompactContextProposed{Request: request})
			if len(effects) != 0 {
				t.Fatalf("background compact effects = %#v, want none", effects)
			}
			if updated.LastCompact.Status != "blocked" || updated.LastCompact.Source.Mode != CompactModeBackground || len(updated.LastCompact.Caveats) != 1 || !strings.Contains(updated.LastCompact.Caveats[0], string(tc.want)) {
				t.Fatalf("blocked background compact = %+v, want reason %s", updated.LastCompact, tc.want)
			}
			if updated.Status != tc.model.Status || !reflect.DeepEqual(updated.Transcript, tc.model.Transcript) || updated.ActiveCompact.Source.Mode != "" {
				t.Fatalf("blocked background compact changed primary state = %+v", updated)
			}
		})
	}
}

func TestUpdateAppliesBackgroundCompactResultWithoutPrimaryRuntimeMutation(t *testing.T) {
	t.Parallel()

	primaryOperation := OperationMetadata{ID: "op-primary", Kind: OperationPrompt, Subject: "foreground", Source: "user"}
	backgroundOperation := OperationMetadata{ID: "op-background", Kind: OperationCompact, Subject: "background context compaction", Source: "runtime.utility"}
	request := CompactContextRequest{Source: CompactSourceMetadata{Mode: CompactModeBackground}}
	result := CompactContextResult{
		Status:     "completed",
		Summary:    "background compaction preserved 1 source refs",
		SourceRefs: []CompactSourceRef{{ID: "prompt-1", Kind: "prompt", Excerpt: "background compact"}},
		Budget:     CompactContextBudget{BlockCount: 1, SourceRefCount: 1, UsedBytes: 64},
		Source:     CompactSourceMetadata{Mode: CompactModeBackground},
	}
	base := Model{
		Status:          StatusActive,
		Result:          "foreground still running",
		ActiveOperation: primaryOperation,
		ActiveCompact:   request,
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "foreground"}},
	}
	updated, effects := Update(base, CompactContextCompleted{Operation: backgroundOperation, Result: result})

	if len(effects) != 0 {
		t.Fatalf("background compact completion effects = %#v, want none", effects)
	}
	if updated.Status != base.Status || updated.Result != base.Result || !reflect.DeepEqual(updated.Transcript, base.Transcript) || updated.ActiveOperation != primaryOperation {
		t.Fatalf("background compact completion changed primary state = %+v", updated)
	}
	if updated.ActiveCompact.Source.Mode != "" || !reflect.DeepEqual(updated.LastCompact, result) {
		t.Fatalf("background compact completion state = active %+v last %+v", updated.ActiveCompact, updated.LastCompact)
	}
}

func TestUpdateStartsUtilityJobOnlyWhileRuntimeIdle(t *testing.T) {
	t.Parallel()

	request := summaryRefreshUtilityRequest()
	model := Model{Status: StatusIdle, NextOperation: 5, Result: "previous result", Transcript: []TranscriptEntry{{Kind: "result", Text: "previous result"}}}
	firstModel, firstEffects := Update(model, UtilityJobProposed{Request: request})
	secondModel, secondEffects := Update(model, UtilityJobProposed{Request: request})

	if !reflect.DeepEqual(firstModel, secondModel) || !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("utility update not deterministic:\nfirst:  %#v %#v\nsecond: %#v %#v", firstModel, firstEffects, secondModel, secondEffects)
	}
	if firstModel.Status != StatusIdle || firstModel.Result != model.Result || !reflect.DeepEqual(firstModel.Transcript, model.Transcript) {
		t.Fatalf("utility changed primary runtime state = %+v", firstModel)
	}
	if !reflect.DeepEqual(firstModel.ActiveUtility, request) || firstModel.LastUtility.Status != utility.StatusRunning {
		t.Fatalf("utility state = active %+v last %+v", firstModel.ActiveUtility, firstModel.LastUtility)
	}
	if firstModel.ActiveOperation.Kind != "" || firstModel.NextOperation != 6 {
		t.Fatalf("primary operation changed = active %+v next %d", firstModel.ActiveOperation, firstModel.NextOperation)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want one utility effect", len(firstEffects))
	}
	effect, ok := firstEffects[0].(UtilityJobEffect)
	if !ok || !reflect.DeepEqual(effect.Request, request) {
		t.Fatalf("utility effect = %#v", firstEffects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-6", Kind: OperationUtility, Subject: "summary_refresh status-summary-refresh", Source: "runtime.utility"})
}

func TestUpdateBlocksUtilityJobWhenPrimaryRuntimeCannotYield(t *testing.T) {
	t.Parallel()

	request := summaryRefreshUtilityRequest()
	cases := []struct {
		name  string
		model Model
		want  utility.DenialReason
	}{
		{name: "primary active", model: Model{Status: StatusActive}, want: utility.DenialPrimaryBusy},
		{name: "active operation", model: Model{Status: StatusIdle, ActiveOperation: OperationMetadata{Kind: OperationBash}}, want: utility.DenialActiveOperation},
		{name: "approval pending", model: Model{Status: StatusIdle, PendingApproval: ApprovalProposal{ID: "approval-1"}}, want: utility.DenialApprovalPending},
		{name: "queued input", model: Model{Status: StatusIdle, Queued: []QueuedEntry{{Kind: "prompt", Text: "queued"}}}, want: utility.DenialQueuedUserInput},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			updated, effects := Update(tc.model, UtilityJobProposed{Request: request})
			if len(effects) != 0 {
				t.Fatalf("utility effects = %#v, want none", effects)
			}
			if updated.LastUtility.Status != utility.StatusBlocked || updated.LastUtility.Denial.Reason != tc.want {
				t.Fatalf("blocked utility = %+v, want reason %s", updated.LastUtility, tc.want)
			}
			if updated.Status != tc.model.Status || !reflect.DeepEqual(updated.Transcript, tc.model.Transcript) || updated.ActiveUtility.ID != "" {
				t.Fatalf("blocked utility changed primary state = %+v", updated)
			}
		})
	}
}

func summaryRefreshUtilityRequest() utility.JobRequest {
	return utility.NormalizeJobRequest(utility.JobRequest{
		ID:     "status-summary-refresh",
		Kind:   utility.JobSummaryRefresh,
		Model:  "test/utility",
		Source: utility.Source{Caller: "test.utility"},
		SummaryRefresh: utility.SummaryRefreshInput{
			OriginalSummary: "Runtime status is visible.",
			RequiredDetails: []string{"primary runtime remains idle", "source refs stay visible"},
			SourceRefIDs:    []string{"test-ref-runtime", "test-ref-roadmap"},
			ConfidenceHint:  "high",
		},
	})
}

func TestUpdateAppliesUtilityResultWithoutPrimaryRuntimeMutation(t *testing.T) {
	t.Parallel()

	request := summaryRefreshUtilityRequest()
	result := utility.RunJob(request)
	operation := OperationMetadata{ID: "op-utility", Kind: OperationUtility, Subject: "summary_refresh status-summary-refresh", Source: "runtime.utility"}
	base := Model{Status: StatusIdle, Result: "previous result", ActiveUtility: request, LastUtility: utility.RunningResult(request), Transcript: []TranscriptEntry{{Kind: "result", Text: "previous result"}}}
	updated, effects := Update(base, UtilityJobCompleted{Operation: operation, Result: result})
	if len(effects) != 0 {
		t.Fatalf("utility completion effects = %#v, want none", effects)
	}
	if updated.Status != base.Status || updated.Result != base.Result || !reflect.DeepEqual(updated.Transcript, base.Transcript) || updated.ActiveOperation.Kind != "" {
		t.Fatalf("utility completion changed primary state = %+v", updated)
	}
	if updated.ActiveUtility.ID != "" || !reflect.DeepEqual(updated.LastUtility, result) {
		t.Fatalf("utility completion state = active %+v last %+v", updated.ActiveUtility, updated.LastUtility)
	}
}

func TestUpdateHandlesReadToolProposalDeterministically(t *testing.T) {
	t.Parallel()

	request := ReadToolRequest{Path: "docs/notes.md", StartLine: 3, LineLimit: 2, MaxPreviewBytes: 256, Source: ReadSourceMetadata{Caller: "test", RequestID: "read-1"}}
	model := Model{Status: StatusIdle, NextOperation: 4}
	firstModel, firstEffects := Update(model, ReadToolProposed{Request: request})
	secondModel, secondEffects := Update(model, ReadToolProposed{Request: request})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for read proposal model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for read proposal effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{
		ID:      "op-5",
		Kind:    OperationRead,
		Subject: "docs/notes.md",
		Source:  "user",
	})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "tool", Text: "read docs/notes.md"}}) {
		t.Fatalf("Transcript = %#v", got)
	}
	if !reflect.DeepEqual(firstModel.ActiveRead, request) {
		t.Fatalf("ActiveRead = %#v, want %#v", firstModel.ActiveRead, request)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(ReadToolEffect)
	if !ok {
		t.Fatalf("effect type = %T, want ReadToolEffect", firstEffects[0])
	}
	if !reflect.DeepEqual(effect.Request, request) {
		t.Fatalf("read request = %#v, want %#v", effect.Request, request)
	}
	assertOperationMetadata(t, effect.Metadata(), firstModel.ActiveOperation)
}

func TestUpdateHandlesSearchToolProposalDeterministically(t *testing.T) {
	t.Parallel()

	request := SearchToolRequest{ToolName: SearchToolGrep, Query: "TODO", IncludePattern: "**/*.go", MaxResults: 4, MaxPreviewBytes: 128, Source: SearchSourceMetadata{Caller: "test", RequestID: "grep-1"}}
	model := Model{Status: StatusIdle, NextOperation: 4}
	firstModel, firstEffects := Update(model, SearchToolProposed{Request: request})
	secondModel, secondEffects := Update(model, SearchToolProposed{Request: request})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for search proposal model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for search proposal effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{ID: "op-5", Kind: OperationGrep, Subject: "TODO in **/*.go", Source: "user"})
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "tool", Text: "grep TODO in **/*.go"}}) {
		t.Fatalf("Transcript = %#v", got)
	}
	if !reflect.DeepEqual(firstModel.ActiveSearch, request) {
		t.Fatalf("ActiveSearch = %#v, want %#v", firstModel.ActiveSearch, request)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(SearchToolEffect)
	if !ok {
		t.Fatalf("effect type = %T, want SearchToolEffect", firstEffects[0])
	}
	if !reflect.DeepEqual(effect.Request, request) {
		t.Fatalf("search request = %#v, want %#v", effect.Request, request)
	}
	assertOperationMetadata(t, effect.Metadata(), firstModel.ActiveOperation)
}

func TestUpdateHandlesFakeResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "hello", Source: "user"}
	model := Model{
		Status:        StatusActive,
		NextOperation: 3,
		Transcript:    []TranscriptEntry{{Kind: "prompt", Text: "hello"}},
	}

	completed, effects := Update(model, FakeEffectCompleted{Operation: operation, Result: "fake answer"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", completed.Status, StatusIdle)
	}
	if completed.Result != "fake answer" {
		t.Fatalf("Result = %q, want fake answer", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "fake answer"}) {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := FailureMetadata{Code: "fake_failed", Message: "fake failure", Retryable: true}
	failed, effects := Update(model, FakeEffectFailed{Operation: operation, Failure: failure})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", failed.Status, StatusIdle)
	}
	if failed.Result != "fake failure" {
		t.Fatalf("Result = %q, want fake failure", failed.Result)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got != (TranscriptEntry{Kind: "failure", Text: "fake failure"}) {
		t.Fatalf("last transcript = %#v", got)
	}
}

func TestUpdateHandlesReadToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationRead, Subject: "notes.txt", Source: "user"}
	model := Model{
		Status:        StatusActive,
		NextOperation: 3,
		Transcript:    []TranscriptEntry{{Kind: "tool", Text: "read notes.txt"}},
	}
	result := ReadToolResult{
		ToolName:              "read",
		RequestedPath:         "notes.txt",
		WorkspaceRelativePath: "notes.txt",
		EffectiveRange:        ReadLineRange{StartLine: 2, EndLine: 3, Limit: 2},
		PreviewText:           "2: beta\n3: gamma\n",
		Error:                 ReadToolError{Kind: ReadToolErrorNone},
	}

	completed, effects := Update(model, ReadToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", completed.Status, StatusIdle)
	}
	if !reflect.DeepEqual(completed.LastRead, result) {
		t.Fatalf("LastRead = %#v, want %#v", completed.LastRead, result)
	}
	if completed.ActiveRead != (ReadToolRequest{}) {
		t.Fatalf("ActiveRead = %#v, want cleared after read completion", completed.ActiveRead)
	}
	if !strings.Contains(completed.Result, "read notes.txt:2-3") || !strings.Contains(completed.Result, "2: beta") {
		t.Fatalf("Result = %q, want bounded read summary with line refs", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got.Kind != "result" || got.Text != completed.Result {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := result
	failure.Error = ReadToolError{Kind: ReadToolErrorMissingFile, Message: "file does not exist"}
	failure.PreviewText = ""
	failed, effects := Update(model, ReadToolCompleted{Operation: operation, Result: failure})
	if len(effects) != 0 {
		t.Fatalf("failed len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("failed Status = %q, want %q", failed.Status, StatusIdle)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got.Kind != "failure" || !strings.Contains(got.Text, "missing_file") {
		t.Fatalf("failure transcript = %#v", got)
	}
}

func TestUpdateHandlesSearchToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationGrep, Subject: "TODO", Source: "user"}
	model := Model{Status: StatusActive, NextOperation: 3, Transcript: []TranscriptEntry{{Kind: "tool", Text: "grep TODO"}}}
	result := SearchToolResult{
		ToolName: "grep",
		Query:    "TODO",
		Matches:  []SearchToolMatch{{Path: "notes.txt", LineNumber: 2, PreviewText: "TODO here"}},
		Error:    SearchToolError{Kind: SearchToolErrorNone},
	}

	completed, effects := Update(model, SearchToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle || completed.LastSearch.ToolName != "grep" || len(completed.LastSearch.Matches) != 1 {
		t.Fatalf("completed search model = %#v", completed)
	}
	if !strings.Contains(completed.Result, "grep TODO: 1 matches") || !strings.Contains(completed.Result, "notes.txt:2: TODO here") {
		t.Fatalf("Result = %q, want bounded search summary with line refs", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got.Kind != "result" || got.Text != completed.Result {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := result
	failure.Error = SearchToolError{Kind: SearchToolErrorInvalidQuery, Message: "regex query is invalid"}
	failure.Matches = nil
	failed, effects := Update(model, SearchToolCompleted{Operation: operation, Result: failure})
	if len(effects) != 0 || failed.Status != StatusIdle {
		t.Fatalf("failed model/effects = %#v %#v", failed, effects)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got.Kind != "failure" || !strings.Contains(got.Text, "invalid_query") {
		t.Fatalf("failure transcript = %#v", got)
	}
}

func TestUpdateQueuesPromptWhileFakeWorkIsActive(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, PromptSubmitted{Text: "queued follow-up"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if updated.NextOperation != model.NextOperation {
		t.Fatalf("NextOperation = %d, want %d", updated.NextOperation, model.NextOperation)
	}
	if got, want := updated.Transcript, model.Transcript; !reflect.DeepEqual(got, want) {
		t.Fatalf("Transcript = %#v, want active transcript unchanged %#v", got, want)
	}
	if got, want := updated.ActiveOperation, operation; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveOperation = %#v, want %#v", got, want)
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
}

func TestUpdateRecordsInterruptingForActiveFakeWork(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Queued:          []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "user pressed interrupt"})
	if updated.Status != StatusCanceling {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusCanceling)
	}
	if !reflect.DeepEqual(updated.Queued, model.Queued) {
		t.Fatalf("Queued = %#v, want %#v", updated.Queued, model.Queued)
	}
	if got := updated.Transcript[len(updated.Transcript)-1]; got != (TranscriptEntry{Kind: "interrupting", Text: "user pressed interrupt"}) {
		t.Fatalf("last transcript = %#v", got)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(FakeInterruptEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakeInterruptEffect", effects[0])
	}
	wantCancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	wantOperation := operation
	wantOperation.Cancel = wantCancel
	assertOperationMetadata(t, updated.ActiveOperation, wantOperation)
	assertOperationMetadata(t, effect.Metadata(), wantOperation)
	if effect.Cancel != wantCancel {
		t.Fatalf("Cancel = %#v, want %#v", effect.Cancel, wantCancel)
	}
}

func TestUpdateDoesNotFakeCancelActiveReadWork(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationRead, Subject: "notes.txt", Source: "user"}
	model := Model{
		Status:          StatusActive,
		NextOperation:   1,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "tool", Text: "read notes.txt"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "ctrl-c"})

	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want no fake interrupt effect for read work", len(effects))
	}
	if !reflect.DeepEqual(updated, model) {
		t.Fatalf("updated model = %#v, want active read unchanged %#v", updated, model)
	}
}

func TestUpdateQueuesReadProposalWhileWorkIsActive(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationRead, Subject: "active.txt", Source: "user"}
	model := Model{
		Status:          StatusActive,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "tool", Text: "read active.txt"}},
	}

	updated, effects := Update(model, ReadToolProposed{Request: ReadToolRequest{Path: "queued.txt"}})

	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want queued read no-op", len(effects))
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "read", Text: "queued.txt"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(updated.Transcript, model.Transcript) || updated.ActiveOperation != operation {
		t.Fatalf("active read mutated unexpectedly: %#v", updated)
	}
}

func TestUpdateQueuesSearchProposalWhileWorkIsActive(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationFind, Subject: "active", Source: "user"}
	model := Model{Status: StatusActive, ActiveOperation: operation, Transcript: []TranscriptEntry{{Kind: "tool", Text: "find active"}}}

	updated, effects := Update(model, SearchToolProposed{Request: SearchToolRequest{ToolName: SearchToolFind, Pattern: "queued/*.go"}})

	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want queued search no-op", len(effects))
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "find", Text: "queued/*.go"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(updated.Transcript, model.Transcript) || updated.ActiveOperation != operation {
		t.Fatalf("active search mutated unexpectedly: %#v", updated)
	}
}

func TestUpdateRecordsCanceledOutcomeFromFakeInterruptResolution(t *testing.T) {
	t.Parallel()

	cancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user", Cancel: cancel}
	model := Model{
		Status:          StatusCanceling,
		ActiveOperation: operation,
		Queued:          []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript: []TranscriptEntry{
			{Kind: "prompt", Text: "active work"},
			{Kind: "interrupting", Text: "user pressed interrupt"},
		},
	}

	updated, effects := Update(model, FakeInterruptResolved{Operation: operation, Cancel: cancel})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusCanceled {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusCanceled)
	}
	if updated.Result != "fake work canceled" {
		t.Fatalf("Result = %q, want fake work canceled", updated.Result)
	}
	if !reflect.DeepEqual(updated.Queued, model.Queued) {
		t.Fatalf("Queued = %#v, want %#v", updated.Queued, model.Queued)
	}
	if got := updated.Transcript[len(updated.Transcript)-1]; got != (TranscriptEntry{Kind: "canceled", Text: "fake work canceled"}) {
		t.Fatalf("last transcript = %#v", got)
	}
	assertOperationMetadata(t, updated.ActiveOperation, operation)
}

func TestUpdateIgnoresInterruptWhenNoFakeWorkIsActive(t *testing.T) {
	t.Parallel()

	model := Model{
		Status:     StatusIdle,
		Result:     "previous result",
		Queued:     []QueuedEntry{{Kind: "prompt", Text: "queued follow-up"}},
		Transcript: []TranscriptEntry{{Kind: "result", Text: "previous result"}},
	}

	updated, effects := Update(model, InterruptRequested{Reason: "user pressed interrupt"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if !reflect.DeepEqual(updated, model) {
		t.Fatalf("updated model = %#v, want unchanged %#v", updated, model)
	}
}

func TestUpdateQueuesOrdinaryPromptInsteadOfTreatingItAsInterrupt(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status:          StatusActive,
		ActiveOperation: operation,
		Transcript:      []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	updated, effects := Update(model, PromptSubmitted{Text: "please stop after this"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if updated.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusActive)
	}
	if got, want := updated.Queued, []QueuedEntry{{Kind: "prompt", Text: "please stop after this"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Queued = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(updated.Transcript, model.Transcript) {
		t.Fatalf("Transcript = %#v, want unchanged %#v", updated.Transcript, model.Transcript)
	}
	assertOperationMetadata(t, updated.ActiveOperation, operation)
}

func TestUpdatePreservesQueuedPromptSubmissionOrder(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusActive}
	model, effects := Update(model, PromptSubmitted{Text: "first queued"})
	if len(effects) != 0 {
		t.Fatalf("first len(effects) = %d, want 0", len(effects))
	}
	model, effects = Update(model, PromptSubmitted{Text: "second queued"})
	if len(effects) != 0 {
		t.Fatalf("second len(effects) = %d, want 0", len(effects))
	}
	model, effects = Update(model, PromptSubmitted{Text: "third queued"})
	if len(effects) != 0 {
		t.Fatalf("third len(effects) = %d, want 0", len(effects))
	}

	want := []QueuedEntry{
		{Kind: "prompt", Text: "first queued"},
		{Kind: "prompt", Text: "second queued"},
		{Kind: "prompt", Text: "third queued"},
	}
	if !reflect.DeepEqual(model.Queued, want) {
		t.Fatalf("Queued = %#v, want %#v", model.Queued, want)
	}
}

func TestUpdateKeepsQueuedPromptsVisibleAfterFakeWorkCompletes(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "active work", Source: "user"}
	model := Model{
		Status: StatusActive,
		Queued: []QueuedEntry{
			{Kind: "prompt", Text: "first queued"},
			{Kind: "prompt", Text: "second queued"},
		},
		Transcript: []TranscriptEntry{{Kind: "prompt", Text: "active work"}},
	}

	completed, effects := Update(model, FakeEffectCompleted{Operation: operation, Result: "done"})
	if len(effects) != 0 {
		t.Fatalf("completed len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("completed Status = %q, want %q", completed.Status, StatusIdle)
	}
	if !reflect.DeepEqual(completed.Queued, model.Queued) {
		t.Fatalf("completed Queued = %#v, want %#v", completed.Queued, model.Queued)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "done"}) {
		t.Fatalf("completed last transcript = %#v", got)
	}

	failure := FailureMetadata{Code: "fake_failed", Message: "failed", Retryable: true}
	failed, effects := Update(model, FakeEffectFailed{Operation: operation, Failure: failure})
	if len(effects) != 0 {
		t.Fatalf("failed len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("failed Status = %q, want %q", failed.Status, StatusIdle)
	}
	if !reflect.DeepEqual(failed.Queued, model.Queued) {
		t.Fatalf("failed Queued = %#v, want %#v", failed.Queued, model.Queued)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got != (TranscriptEntry{Kind: "failure", Text: "failed"}) {
		t.Fatalf("failed last transcript = %#v", got)
	}
}

func TestDispatchHandlesPromptEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-1", Kind: OperationPrompt, Subject: "explain status", Source: "user"}
	messages := Dispatch([]Effect{FakePromptEffect{Operation: operation, Prompt: "explain status"}})

	if got, want := messages, []Message{FakeEffectCompleted{Operation: operation, Result: "Fake Aila response: explain status"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchHandlesCommandEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-2", Kind: OperationCommand, Subject: "status", Source: "user"}
	messages := Dispatch([]Effect{FakeCommandEffect{Operation: operation, Command: "status"}})

	if got, want := messages, []Message{FakeEffectCompleted{Operation: operation, Result: "fake command result: status"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchHandlesCompactContextEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-compact", Kind: OperationCompact, Subject: "manual context compaction", Source: "user"}
	request := CompactContextRequest{
		Blocks:     []CompactContextBlock{{ID: "block-1", Kind: "prompt", Text: "review compact", SourceRefIDs: []string{"prompt-1"}}},
		SourceRefs: []CompactSourceRef{{ID: "prompt-1", Kind: "prompt", Excerpt: "review compact"}},
		Budget:     CompactContextBudget{BlockCount: 1, SourceRefCount: 1, UsedBytes: 14},
	}
	messages := Dispatch([]Effect{CompactContextEffect{Operation: operation, Request: request}})
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one compact completion", messages)
	}
	completed, ok := messages[0].(CompactContextCompleted)
	if !ok || completed.Operation != operation || completed.Result.Status != "completed" || len(completed.Result.SourceRefs) != 1 {
		t.Fatalf("compact dispatch = %#v", messages[0])
	}
}

func TestDispatchHandlesUtilityJobEffect(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-utility", Kind: OperationUtility, Subject: "summary_refresh status-summary-refresh", Source: "runtime.utility"}
	request := summaryRefreshUtilityRequest()
	messages := Dispatch([]Effect{UtilityJobEffect{Operation: operation, Request: request}})
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one utility completion", messages)
	}
	completed, ok := messages[0].(UtilityJobCompleted)
	if !ok || completed.Operation != operation || completed.Result.Status != utility.StatusCompleted || completed.Result.SummaryRefresh.Status != utility.SummaryRefreshRefreshed || len(completed.Result.SummaryRefresh.SourceRefIDs) != 2 || len(completed.Result.EvidenceRefs) != 4 {
		t.Fatalf("utility dispatch = %#v", messages[0])
	}
	if completed.Result.Safety.FileMutation || completed.Result.Safety.GitMutation || completed.Result.Safety.ProjectArtifactMutation || completed.Result.Safety.PermissionApproval || completed.Result.Safety.WorkflowPhaseTransition || completed.Result.Safety.FinalJudgment || completed.Result.Safety.ContextRefresh || completed.Result.Safety.ContextCompaction || completed.Result.Safety.ContextRewrite {
		t.Fatalf("utility dispatch crossed safety boundary: %+v", completed.Result.Safety)
	}
}

func TestDispatchHandlesInterruptEffect(t *testing.T) {
	t.Parallel()

	cancel := CancelMetadata{Requested: true, Reason: "user pressed interrupt"}
	operation := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "active work", Source: "user", Cancel: cancel}
	messages := Dispatch([]Effect{FakeInterruptEffect{Operation: operation, Cancel: cancel}})

	if got, want := messages, []Message{FakeInterruptResolved{Operation: operation, Cancel: cancel}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", got, want)
	}
}

func TestDispatchReturnsMixedResultsInInputOrder(t *testing.T) {
	t.Parallel()

	prompt := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "hello", Source: "user"}
	command := OperationMetadata{ID: "op-4", Kind: OperationCommand, Subject: "status", Source: "user"}
	messages := Dispatch([]Effect{
		FakePromptEffect{Operation: prompt, Prompt: "hello"},
		FakeCommandEffect{Operation: command, Command: "status"},
		FakePromptEffect{Operation: prompt, Prompt: "again"},
	})

	want := []Message{
		FakeEffectCompleted{Operation: prompt, Result: "Fake Aila response: hello"},
		FakeEffectCompleted{Operation: command, Result: "fake command result: status"},
		FakeEffectCompleted{Operation: prompt, Result: "Fake Aila response: again"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", messages, want)
	}
}

func TestDispatchIgnoresUnsupportedEffects(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-5", Kind: OperationPrompt, Subject: "ignored", Source: "user"}
	messages := Dispatch([]Effect{
		unsupportedEffect{operation: operation},
		FakePromptEffect{Operation: operation, Prompt: "kept"},
	})

	want := []Message{FakeEffectCompleted{Operation: operation, Result: "Fake Aila response: kept"}}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("Dispatch() = %#v, want %#v", messages, want)
	}
}

func TestDispatchIsDeterministic(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-6", Kind: OperationCommand, Subject: "status", Source: "user"}
	effects := []Effect{FakeCommandEffect{Operation: operation, Command: "status"}}
	first := Dispatch(effects)
	second := Dispatch(effects)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Dispatch is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestDispatchHandlesNoEffects(t *testing.T) {
	t.Parallel()

	if messages := Dispatch(nil); len(messages) != 0 {
		t.Fatalf("len(messages) = %d, want 0", len(messages))
	}
}

func TestDispatchContextRecordsCancellationDiagnosticMessage(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operation := OperationMetadata{ID: "op-7", Kind: OperationPrompt, Subject: "canceled", Source: "user"}

	messages := DispatchContext(ctx, []Effect{FakePromptEffect{Operation: operation, Prompt: "canceled"}})

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	message, ok := messages[0].(RuntimeDiagnostic)
	if !ok {
		t.Fatalf("message = %T, want RuntimeDiagnostic", messages[0])
	}
	if message.Diagnostic.Category != diagnostic.CategoryCancellation || message.Diagnostic.Source != diagnostic.SourceEffect {
		t.Fatalf("diagnostic identity = %#v", message.Diagnostic)
	}
	if !strings.Contains(message.Diagnostic.BoundedMessage, "context canceled") {
		t.Fatalf("diagnostic message = %q, want context cancellation", message.Diagnostic.BoundedMessage)
	}

	model, effects := Update(Model{Status: StatusActive}, message)
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusActive {
		t.Fatalf("status = %q, want active unchanged", model.Status)
	}
	if len(model.Diagnostics) != 1 || model.Diagnostics[0].Category != diagnostic.CategoryCancellation {
		t.Fatalf("model diagnostics = %#v", model.Diagnostics)
	}
}

func TestDispatchRecoversEffectPanicAsDiagnosticMessage(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-8", Kind: OperationPrompt, Subject: "panic", Source: "user"}
	messages := Dispatch([]Effect{panicEffect{operation: operation}})

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	message, ok := messages[0].(RuntimeDiagnostic)
	if !ok {
		t.Fatalf("message = %T, want RuntimeDiagnostic", messages[0])
	}
	if message.Diagnostic.Category != diagnostic.CategoryEffect || message.Diagnostic.Source != diagnostic.SourceEffect {
		t.Fatalf("diagnostic identity = %#v", message.Diagnostic)
	}
	if !strings.Contains(message.Diagnostic.BoundedMessage, "supervised effect panic recovered") {
		t.Fatalf("diagnostic message = %q, want recovered panic", message.Diagnostic.BoundedMessage)
	}

	model, effects := Update(Model{Status: StatusActive}, message)
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusActive {
		t.Fatalf("status = %q, want active unchanged", model.Status)
	}
	if len(model.Diagnostics) != 1 || model.Diagnostics[0].RecoveryAction != diagnostic.RecoveryInspect {
		t.Fatalf("model diagnostics = %#v", model.Diagnostics)
	}
}

func TestUpdateDoesNotMutateInputModel(t *testing.T) {
	t.Parallel()

	model := Model{
		Status:        StatusIdle,
		NextOperation: 2,
		Transcript:    []TranscriptEntry{{Kind: "result", Text: "previous"}},
	}
	original := Model{
		Status:        model.Status,
		NextOperation: model.NextOperation,
		Transcript:    append([]TranscriptEntry(nil), model.Transcript...),
	}

	updated, _ := Update(model, PromptSubmitted{Text: "next"})
	updated.Transcript[0].Text = "mutated copy"

	if !reflect.DeepEqual(model, original) {
		t.Fatalf("input model mutated:\ngot:  %#v\nwant: %#v", model, original)
	}
}

func TestFailureAndCancelMetadataAreInert(t *testing.T) {
	t.Parallel()

	metadata := OperationMetadata{
		ID:      "op-9",
		Kind:    OperationPrompt,
		Subject: "danger?",
		Source:  "user",
		Failure: FailureMetadata{Code: "bounded", Message: "bounded failure", Retryable: false},
		Cancel:  CancelMetadata{Requested: true, Reason: "user requested stop"},
	}

	model, effects := Update(Model{Status: StatusIdle}, FakeEffectFailed{
		Operation: metadata,
		Failure:   metadata.Failure,
	})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", model.Status, StatusIdle)
	}
	if model.Result != metadata.Failure.Message {
		t.Fatalf("Result = %q, want %q", model.Result, metadata.Failure.Message)
	}
}

func TestUpdateSpawnsSubagentAsSupervisedEffect(t *testing.T) {
	t.Parallel()

	parent := OperationMetadata{ID: "op-parent", Kind: OperationCapability, Subject: "build", Source: "runtime.capability"}
	request := SubagentRequest{
		Purpose: "inspect runtime tests",
		Input:   "collect evidence for supervised child work",
		Tools:   []string{"read", "grep"},
		Budget:  SubagentBudget{MaxTurns: 3, MaxTokens: 1200, TimeoutMillis: 5000},
		EvidenceLinks: []SubagentEvidenceLink{{
			ID:      "source-doc",
			Kind:    "doc",
			Path:    "ARCHITECTURE.md",
			Excerpt: "Subagents are supervised concurrent work",
		}},
		Source: SubagentSourceMetadata{Caller: "test", RequestID: "spawn-1"},
	}

	model, effects := Update(Model{Status: StatusActive, ActiveOperation: parent, NextOperation: 4}, SubagentSpawnProposed{Request: request})
	if model.Status != StatusActive || model.ActiveOperation != parent {
		t.Fatalf("primary operation changed = status:%q active:%+v", model.Status, model.ActiveOperation)
	}
	if model.NextOperation != 5 {
		t.Fatalf("NextOperation = %d, want 5", model.NextOperation)
	}
	run, ok := findSubagentRun(model.Subagents, "op-5")
	if !ok {
		t.Fatalf("subagents = %+v, want op-5", model.Subagents)
	}
	if run.ParentRunID != parent.ID || run.Purpose != request.Purpose || run.Status != SubagentStatusRunning || run.Budget != request.Budget || !reflect.DeepEqual(run.Tools, request.Tools) || len(run.EvidenceLinks) != 1 {
		t.Fatalf("spawned subagent = %+v", run)
	}
	if len(effects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(effects))
	}
	effect, ok := effects[0].(SpawnSubagentEffect)
	if !ok {
		t.Fatalf("effect type = %T, want SpawnSubagentEffect", effects[0])
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{ID: "op-5", Kind: OperationSubagent, Subject: request.Purpose, Source: "runtime.subagent"})
	if effect.Request.ID != "op-5" || effect.Request.ParentRunID != parent.ID || effect.Request.Source.Caller != "test" {
		t.Fatalf("effect request = %+v", effect.Request)
	}

	messages := Dispatch(effects)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	progress, ok := messages[0].(SubagentProgressed)
	if !ok {
		t.Fatalf("message type = %T, want SubagentProgressed", messages[0])
	}
	model, effects = Update(model, progress)
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	run, ok = findSubagentRun(model.Subagents, "op-5")
	if !ok || run.Status != SubagentStatusRunning || !strings.Contains(run.Summary, "subagent running") {
		t.Fatalf("progressed subagent = %+v found=%v", run, ok)
	}
}

func TestUpdateKeepsSubagentLifecycleObservable(t *testing.T) {
	t.Parallel()

	base := Model{Subagents: []SubagentRun{{
		ID:          "child-active",
		ParentRunID: "parent-run",
		Purpose:     "collect evidence",
		Status:      SubagentStatusRunning,
		Summary:     "spawn requested",
	}}}
	progressed, effects := Update(base, SubagentProgressed{
		ID:          "child-active",
		ParentRunID: "parent-run",
		Purpose:     "collect evidence",
		Status:      SubagentStatusRunning,
		Summary:     "read source docs",
		EvidenceLinks: []SubagentEvidenceLink{{
			ID:   "runtime-src",
			Kind: "file",
			Path: "internal/runtime/runtime.go",
		}},
	})
	if len(effects) != 0 {
		t.Fatalf("progress effects = %d, want 0", len(effects))
	}
	run, ok := findSubagentRun(progressed.Subagents, "child-active")
	if !ok || run.Status != SubagentStatusRunning || run.Summary != "read source docs" || len(run.EvidenceLinks) != 1 {
		t.Fatalf("progressed run = %+v found=%v", run, ok)
	}

	completed, effects := Update(progressed, SubagentCompleted{ParentRunID: "parent-run", Result: SubagentResult{
		ID:      "child-active",
		Purpose: "collect evidence",
		Summary: "evidence ready",
		EvidenceLinks: []SubagentEvidenceLink{{
			ID:      "completion-log",
			Kind:    "log",
			Command: "go test ./internal/runtime",
		}},
	}})
	if len(effects) != 0 {
		t.Fatalf("completion effects = %d, want 0", len(effects))
	}
	run, ok = findSubagentRun(completed.Subagents, "child-active")
	if !ok || run.Status != SubagentStatusCompleted || run.Summary != "evidence ready" || len(run.EvidenceLinks) != 1 {
		t.Fatalf("completed run = %+v found=%v", run, ok)
	}

	failed, effects := Update(completed, SubagentFailed{
		ID:          "child-failed",
		ParentRunID: "parent-run",
		Purpose:     "try alternate proof",
		Failure:     FailureMetadata{Code: "fixture_missing", Message: "fixture not found", Retryable: true},
	})
	if len(effects) != 0 {
		t.Fatalf("failure effects = %d, want 0", len(effects))
	}
	run, ok = findSubagentRun(failed.Subagents, "child-failed")
	if !ok || run.Status != SubagentStatusFailed || run.Failure.Code != "fixture_missing" || run.Summary != "fixture not found" {
		t.Fatalf("failed run = %+v found=%v", run, ok)
	}

	canceled, effects := Update(failed, SubagentCanceled{
		ID:          "child-canceled",
		ParentRunID: "parent-run",
		Purpose:     "bounded review",
		Cancel:      CancelMetadata{Requested: true, Reason: "parent stopped"},
	})
	if len(effects) != 0 {
		t.Fatalf("cancellation effects = %d, want 0", len(effects))
	}
	run, ok = findSubagentRun(canceled.Subagents, "child-canceled")
	if !ok || run.Status != SubagentStatusCanceled || !run.Cancel.Requested || run.Summary != "parent stopped" {
		t.Fatalf("canceled run = %+v found=%v", run, ok)
	}
}

func TestActiveSubagentCannotBecomeUnobservable(t *testing.T) {
	t.Parallel()

	activeChild := SubagentRun{
		ID:          "child-active",
		ParentRunID: "parent-run",
		Purpose:     "watch build",
		Status:      SubagentStatusRunning,
		Summary:     "running",
		EvidenceLinks: []SubagentEvidenceLink{{
			ID:   "active-evidence",
			Kind: "runtime",
		}},
	}
	primary := OperationMetadata{ID: "op-parent", Kind: OperationPrompt, Subject: "parent", Source: "user"}
	base := Model{Status: StatusActive, ActiveOperation: primary, Subagents: []SubagentRun{activeChild}}

	messages := []Message{
		PromptSubmitted{Text: "queued follow-up"},
		RuntimeDiagnostic{Diagnostic: diagnostic.New(diagnostic.Spec{Category: diagnostic.CategoryRuntime, Source: diagnostic.SourceRuntime, Severity: diagnostic.SeverityWarning, Message: "bounded diagnostic"})},
		FakeEffectCompleted{Operation: primary, Result: "parent done"},
	}
	for _, message := range messages {
		updated, _ := Update(base, message)
		run, ok := findSubagentRun(updated.Subagents, activeChild.ID)
		if !ok || !run.Status.Active() || run.ParentRunID != activeChild.ParentRunID || run.Purpose != activeChild.Purpose || len(run.EvidenceLinks) != 1 {
			t.Fatalf("%T hid active subagent: %+v found=%v", message, run, ok)
		}
	}
}

func findSubagentRun(runs []SubagentRun, id string) (SubagentRun, bool) {
	for _, run := range runs {
		if run.ID == id {
			return run, true
		}
	}
	return SubagentRun{}, false
}

func TestRuntimeProductionFilesHaveNoForbiddenImportsOrTokens(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]bool{
		"io":            true,
		"net/http":      true,
		"os":            true,
		"os/exec":       true,
		"path/filepath": true,
		"sync":          true,
	}
	forbiddenTokens := []string{
		"go ",
		"http.",
		"os.",
		"exec.",
		"Open(",
		"ReadFile(",
		"WriteFile(",
		"Mkdir",
		"Remove(",
		"Chdir(",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(string(content), token) {
				t.Fatalf("%s contains forbidden token %q", file, token)
			}
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if forbiddenImports[path] {
				t.Fatalf("%s imports forbidden package %q", file, path)
			}
		}
	}
}

func assertOperationMetadata(t *testing.T, got OperationMetadata, want OperationMetadata) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
}

type unsupportedEffect struct {
	operation OperationMetadata
}

func (unsupportedEffect) runtimeEffect() {}

func (effect unsupportedEffect) Metadata() OperationMetadata {
	return effect.operation
}

type panicEffect struct {
	operation OperationMetadata
}

func (panicEffect) runtimeEffect() {}

func (effect panicEffect) Metadata() OperationMetadata {
	return effect.operation
}

func (panicEffect) dispatchPanic() {
	panic("fake effect worker panic")
}

func TestUpdateHandlesBashToolProposalDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle}
	request := BashToolRequest{Argv: []string{"git", "status", "--short"}, WorkingDir: ".", MaxOutputBytes: 256, TimeoutMillis: 1000, Source: BashSourceMetadata{Caller: "test", RequestID: "bash-1"}}

	firstModel, firstEffects := Update(model, BashToolProposed{Request: request})
	secondModel, secondEffects := Update(model, BashToolProposed{Request: request})

	if firstModel.Status != StatusActive || firstModel.ActiveOperation.Kind != OperationBash || firstModel.ActiveBash.Argv[0] != "git" || len(firstEffects) != 1 {
		t.Fatalf("first bash proposal model=%+v effects=%v", firstModel, firstEffects)
	}
	if firstModel.NextOperation != secondModel.NextOperation || firstModel.ActiveOperation.ID != secondModel.ActiveOperation.ID || len(secondEffects) != 1 {
		t.Fatalf("bash proposal not deterministic: first=%+v second=%+v", firstModel, secondModel)
	}
	effect, ok := firstEffects[0].(BashToolEffect)
	if !ok {
		t.Fatalf("effect type = %T, want BashToolEffect", firstEffects[0])
	}
	if effect.Request.Argv[0] != "git" || effect.Operation.Subject != "git status --short" {
		t.Fatalf("bash effect = %+v", effect)
	}
}

func TestUpdateHandlesBashToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-bash", Kind: OperationBash, Subject: "pwd", Source: "test"}
	model := Model{Status: StatusActive, ActiveOperation: operation, ActiveBash: BashToolRequest{Argv: []string{"pwd"}}}
	result := BashToolResult{ToolName: "bash", RequestedArgv: []string{"pwd"}, WorkspaceRelativeWorkDir: ".", CommandFamily: "pwd", ExpectedEffect: "print workspace working directory", ExitCode: 0, Status: "completed", Stdout: BashToolOutput{Text: "/workspace\n", Bytes: 11}, Error: BashToolError{Kind: BashToolErrorNone}}

	completed, effects := Update(model, BashToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 || completed.Status != StatusIdle || completed.LastBash.CommandFamily != "pwd" || completed.ActiveBash.Argv != nil || completed.ActiveOperation.ID != "" {
		t.Fatalf("completed bash model=%+v effects=%v", completed, effects)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got.Kind != "result" || !strings.Contains(got.Text, "completed exit 0") {
		t.Fatalf("completed bash transcript = %+v", got)
	}

	failure := result
	failure.Error = BashToolError{Kind: BashToolErrorUnsafeCommand, Message: "command is not allowed"}
	failure.Status = "failed"
	failed, effects := Update(model, BashToolCompleted{Operation: operation, Result: failure})
	if len(effects) != 0 || failed.Status != StatusIdle || failed.LastBash.Error.Kind != BashToolErrorUnsafeCommand {
		t.Fatalf("failed bash model=%+v effects=%v", failed, effects)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got.Kind != "failure" || !strings.Contains(got.Text, "unsafe_command") {
		t.Fatalf("failed bash transcript = %+v", got)
	}
}

func TestUpdateQueuesBashToolProposalWhileActive(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusActive, ActiveOperation: OperationMetadata{Kind: OperationPrompt}}
	updated, effects := Update(model, BashToolProposed{Request: BashToolRequest{Argv: []string{"pwd"}}})
	if len(effects) != 0 || len(updated.Queued) != 1 || updated.Queued[0].Kind != "bash" || updated.Queued[0].Text != "pwd" {
		t.Fatalf("queued bash model=%+v effects=%v", updated, effects)
	}
}

func TestUpdateHandlesFetchToolProposalDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle}
	request := FetchToolRequest{URL: " https://example.com/docs ", MaxPreviewBytes: 256, TimeoutMillis: 1000, Source: FetchSourceMetadata{Caller: "test", RequestID: "fetch-1"}}

	firstModel, firstEffects := Update(model, FetchToolProposed{Request: request})
	secondModel, secondEffects := Update(model, FetchToolProposed{Request: request})

	if firstModel.Status != StatusActive || firstModel.ActiveOperation.Kind != OperationFetch || firstModel.ActiveFetch.URL != "https://example.com/docs" || len(firstEffects) != 1 {
		t.Fatalf("first fetch proposal model=%+v effects=%v", firstModel, firstEffects)
	}
	if firstModel.NextOperation != secondModel.NextOperation || firstModel.ActiveOperation.ID != secondModel.ActiveOperation.ID || len(secondEffects) != 1 {
		t.Fatalf("fetch proposal not deterministic: first=%+v second=%+v", firstModel, secondModel)
	}
	effect, ok := firstEffects[0].(FetchToolEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FetchToolEffect", firstEffects[0])
	}
	if effect.Request.URL != "https://example.com/docs" || effect.Operation.Subject != "https://example.com/docs" {
		t.Fatalf("fetch effect = %+v", effect)
	}
}

func TestUpdateHandlesFetchToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-fetch", Kind: OperationFetch, Subject: "https://example.com/docs", Source: "test"}
	model := Model{Status: StatusActive, ActiveOperation: operation, ActiveFetch: FetchToolRequest{URL: "https://example.com/docs"}}
	result := FetchToolResult{ToolName: "fetch", RequestedURL: "https://example.com/docs", EffectiveURL: "https://example.com/docs", Method: "GET", ExpectedEffect: "read remote content through bounded fetch", Status: "completed", HTTPStatusCode: 200, HTTPStatus: "200 OK", ContentType: "text/plain", PreviewText: "hello", Error: FetchToolError{Kind: FetchToolErrorNone}}

	completed, effects := Update(model, FetchToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 || completed.Status != StatusIdle || completed.LastFetch.EffectiveURL != "https://example.com/docs" || completed.ActiveFetch.URL != "" || completed.ActiveOperation.ID != "" {
		t.Fatalf("completed fetch model=%+v effects=%v", completed, effects)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got.Kind != "result" || !strings.Contains(got.Text, "completed 200") {
		t.Fatalf("completed fetch transcript = %+v", got)
	}

	failure := result
	failure.Error = FetchToolError{Kind: FetchToolErrorHTTPStatus, Message: "remote returned 404 Not Found"}
	failure.Status = "http_error"
	failure.HTTPStatusCode = 404
	failed, effects := Update(model, FetchToolCompleted{Operation: operation, Result: failure})
	if len(effects) != 0 || failed.Status != StatusIdle || failed.LastFetch.Error.Kind != FetchToolErrorHTTPStatus {
		t.Fatalf("failed fetch model=%+v effects=%v", failed, effects)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got.Kind != "failure" || !strings.Contains(got.Text, "http_status") {
		t.Fatalf("failed fetch transcript = %+v", got)
	}
}

func TestUpdateQueuesFetchToolProposalWhileActive(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusActive, ActiveOperation: OperationMetadata{Kind: OperationPrompt}}
	updated, effects := Update(model, FetchToolProposed{Request: FetchToolRequest{URL: "https://example.com/docs"}})
	if len(effects) != 0 || len(updated.Queued) != 1 || updated.Queued[0].Kind != "fetch" || updated.Queued[0].Text != "https://example.com/docs" {
		t.Fatalf("queued fetch model=%+v effects=%v", updated, effects)
	}
}

func TestUpdateRetainsToolDecisionMetadata(t *testing.T) {
	t.Parallel()

	readDecision := ToolDecision{
		Present:          true,
		Autonomy:         "off",
		Source:           "autonomy_policy",
		Allowed:          false,
		Automatic:        false,
		ApprovalRequired: true,
		Reason:           "autonomy off requires approval",
		OperationKind:    "read",
		Tool:             "read",
		Target:           "notes.txt",
		ExpectedEffect:   "bounded workspace file preview",
		Reversible:       true,
	}
	readOperation := OperationMetadata{ID: "op-read", Kind: OperationRead, Subject: "notes.txt", Source: "test"}
	readModel := Model{Status: StatusActive, ActiveOperation: readOperation, ActiveRead: ReadToolRequest{Path: "notes.txt"}}
	readResult := ReadToolResult{
		ToolName:      "read",
		RequestedPath: "notes.txt",
		Error:         ReadToolError{Kind: ReadToolErrorPermission, Message: readDecision.Reason},
		Decision:      readDecision,
	}

	readCompleted, effects := Update(readModel, ReadToolCompleted{Operation: readOperation, Result: readResult})
	if len(effects) != 0 || readCompleted.Status != StatusIdle || !reflect.DeepEqual(readCompleted.LastRead.Decision, readDecision) {
		t.Fatalf("completed read model=%+v effects=%v", readCompleted, effects)
	}
	if readCompleted.ActiveOperation.ID != "" || readCompleted.ActiveRead.Path != "" {
		t.Fatalf("active read state not cleared: %+v", readCompleted)
	}

	searchDecision := readDecision
	searchDecision.Tool = "grep"
	searchDecision.Target = "needle in **/*.go"
	searchOperation := OperationMetadata{ID: "op-search", Kind: OperationGrep, Subject: "needle", Source: "test"}
	searchCompleted, effects := Update(Model{Status: StatusActive, ActiveOperation: searchOperation}, SearchToolCompleted{Operation: searchOperation, Result: SearchToolResult{ToolName: "grep", Query: "needle", Error: SearchToolError{Kind: SearchToolErrorPermission, Message: searchDecision.Reason}, Decision: searchDecision}})
	if len(effects) != 0 || searchCompleted.Status != StatusIdle || !reflect.DeepEqual(searchCompleted.LastSearch.Decision, searchDecision) {
		t.Fatalf("completed search model=%+v effects=%v", searchCompleted, effects)
	}

	bashDecision := readDecision
	bashDecision.Tool = "bash"
	bashDecision.Target = ""
	bashDecision.Command = []string{"pwd"}
	bashOperation := OperationMetadata{ID: "op-bash", Kind: OperationBash, Subject: "pwd", Source: "test"}
	bashCompleted, effects := Update(Model{Status: StatusActive, ActiveOperation: bashOperation, ActiveBash: BashToolRequest{Argv: []string{"pwd"}}}, BashToolCompleted{Operation: bashOperation, Result: BashToolResult{ToolName: "bash", RequestedArgv: []string{"pwd"}, Status: "denied", Error: BashToolError{Kind: BashToolErrorPermission, Message: bashDecision.Reason}, Decision: bashDecision}})
	if len(effects) != 0 || bashCompleted.Status != StatusIdle || !reflect.DeepEqual(bashCompleted.LastBash.Decision, bashDecision) {
		t.Fatalf("completed bash model=%+v effects=%v", bashCompleted, effects)
	}

	fetchDecision := readDecision
	fetchDecision.Tool = "fetch"
	fetchDecision.Target = "https://example.com/docs"
	fetchOperation := OperationMetadata{ID: "op-fetch", Kind: OperationFetch, Subject: "https://example.com/docs", Source: "test"}
	fetchCompleted, effects := Update(Model{Status: StatusActive, ActiveOperation: fetchOperation, ActiveFetch: FetchToolRequest{URL: "https://example.com/docs"}}, FetchToolCompleted{Operation: fetchOperation, Result: FetchToolResult{ToolName: "fetch", RequestedURL: "https://example.com/docs", Status: "denied", Error: FetchToolError{Kind: FetchToolErrorPermission, Message: fetchDecision.Reason}, Decision: fetchDecision}})
	if len(effects) != 0 || fetchCompleted.Status != StatusIdle || !reflect.DeepEqual(fetchCompleted.LastFetch.Decision, fetchDecision) {
		t.Fatalf("completed fetch model=%+v effects=%v", fetchCompleted, effects)
	}
}

func TestUpdateHandlesAgentStreamMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-agent", Kind: OperationPrompt, Subject: "explain", Source: "agent-test"}
	model := Model{Status: StatusActive, ActiveOperation: operation, Transcript: []TranscriptEntry{{Kind: "prompt", Text: "explain"}}}

	updated, effects := Update(model, AgentAssistantDelta{Operation: operation, Provider: "fake", Model: "fake-model", Sequence: 1, Text: "Hello "})
	if len(effects) != 0 || updated.Status != StatusActive || updated.AssistantDraft != "Hello " || updated.Result != "Hello " || updated.AgentProvider != "fake" || updated.AgentModel != "fake-model" {
		t.Fatalf("delta model=%+v effects=%v", updated, effects)
	}
	updated, effects = Update(updated, AgentAssistantDelta{Operation: operation, Provider: "fake", Model: "fake-model", Sequence: 2, Text: "world"})
	if len(effects) != 0 || updated.AssistantDraft != "Hello world" || updated.Transcript[len(updated.Transcript)-1] != (TranscriptEntry{Kind: "assistant_delta", Text: "world"}) {
		t.Fatalf("second delta model=%+v effects=%v", updated, effects)
	}

	request := AgentToolRequest{ID: "call-1", Name: "read", Arguments: []AgentToolArgument{{Name: "path", Value: "README.md"}}, Provider: "fake", Model: "fake-model", Sequence: 3}
	updated, effects = Update(updated, AgentToolRequested{Operation: operation, Request: request})
	if len(effects) != 0 || updated.Status != StatusActive || !reflect.DeepEqual(updated.LastAgentToolRequest, request) || updated.LastRead.ToolName != "" || updated.ActiveRead.Path != "" {
		t.Fatalf("tool request model=%+v effects=%v", updated, effects)
	}

	completed, effects := Update(updated, AgentTurnCompleted{Operation: operation, Provider: "fake", Model: "fake-model", FinishReason: "stop"})
	if len(effects) != 0 || completed.Status != StatusIdle || completed.Result != "Hello world" || completed.AgentFinishReason != "stop" || completed.ActiveOperation.ID != "" {
		t.Fatalf("completed model=%+v effects=%v", completed, effects)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "Hello world"}) {
		t.Fatalf("completion transcript = %+v", got)
	}
}

func TestUpdateHandlesAgentTurnFailure(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-agent", Kind: OperationPrompt, Subject: "explain", Source: "agent-test"}
	failure := FailureMetadata{Code: "rate_limited", Message: "provider is rate limited", Retryable: true}
	updated, effects := Update(Model{Status: StatusActive, ActiveOperation: operation}, AgentTurnFailed{Operation: operation, Provider: "fake", Model: "fake-model", Failure: failure})
	if len(effects) != 0 || updated.Status != StatusIdle || updated.Result != failure.Message || updated.ActiveOperation.ID != "" || updated.AgentProvider != "fake" || updated.AgentModel != "fake-model" {
		t.Fatalf("failed model=%+v effects=%v", updated, effects)
	}
	if got := updated.Transcript[len(updated.Transcript)-1]; got.Kind != "failure" || got.Text != failure.Message {
		t.Fatalf("failure transcript = %+v", got)
	}
}

func TestUpdateHandlesApprovalProposalWithoutEffects(t *testing.T) {
	t.Parallel()

	proposal := ApprovalProposal{
		ID:             "approval-1",
		OperationKind:  "file_mutation",
		Target:         "internal/demo.txt",
		RiskSummary:    "would change a workspace file",
		Preview:        []string{"write preview"},
		DefaultAction:  ApprovalActionDeny,
		Path:           "internal/demo.txt",
		Command:        []string{"write", "internal/demo.txt"},
		WorkingDir:     ".",
		ExpectedEffect: "preview only",
		DiffPreview:    []string{"-old", "+new"},
		Reversible:     true,
		RunID:          "run-approval",
		Capability:     "test",
	}
	model := Model{Status: StatusIdle, NextOperation: 2}
	firstModel, firstEffects := Update(model, ApprovalProposed{Proposal: proposal})
	secondModel, secondEffects := Update(model, ApprovalProposed{Proposal: proposal})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("approval proposal model not deterministic:\nfirst: %#v\nsecond:%#v", firstModel, secondModel)
	}
	if len(firstEffects) != 0 || len(secondEffects) != 0 {
		t.Fatalf("approval proposal effects = %v %v, want none", firstEffects, secondEffects)
	}
	if firstModel.Status != StatusApprovalPending || !reflect.DeepEqual(firstModel.PendingApproval, proposal) {
		t.Fatalf("approval state = %+v, want pending proposal %+v", firstModel, proposal)
	}
	assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{ID: "op-3", Kind: OperationApproval, Subject: "internal/demo.txt", Source: "user"})

	proposal.Preview[0] = "mutated"
	proposal.Command[0] = "mutated"
	proposal.DiffPreview[0] = "mutated"
	if firstModel.PendingApproval.Preview[0] != "write preview" || firstModel.PendingApproval.Command[0] != "write" || firstModel.PendingApproval.DiffPreview[0] != "-old" {
		t.Fatalf("pending approval reused caller slices: %+v", firstModel.PendingApproval)
	}
}

func TestUpdateHandlesApprovalDecisionsAsMessagesOnly(t *testing.T) {
	t.Parallel()

	proposal := ApprovalProposal{ID: "approval-1", OperationKind: "file_mutation", Target: "internal/demo.txt", DefaultAction: ApprovalActionDeny}
	pending, effects := Update(Model{Status: StatusIdle}, ApprovalProposed{Proposal: proposal})
	if len(effects) != 0 {
		t.Fatalf("proposal effects = %v", effects)
	}

	for _, action := range []ApprovalAction{ApprovalActionApprove, ApprovalActionDeny, ApprovalActionDefer} {
		action := action
		t.Run(string(action), func(t *testing.T) {
			updated, effects := Update(pending, ApprovalDecisionSelected{ProposalID: proposal.ID, Action: action})
			if len(effects) != 0 {
				t.Fatalf("approval decision effects = %v, want none", effects)
			}
			if updated.Status != StatusIdle || updated.PendingApproval.ID != "" || updated.LastApprovalDecision.Action != action || updated.LastApprovalDecision.Stale {
				t.Fatalf("approval decision state = %+v", updated)
			}
			if !strings.Contains(updated.Result, "approval "+string(action)) || !strings.Contains(updated.Result, "internal/demo.txt") {
				t.Fatalf("approval result = %q", updated.Result)
			}
		})
	}
}

func TestUpdateIgnoresStaleApprovalDecisionWithoutMutationEffects(t *testing.T) {
	t.Parallel()

	pending, _ := Update(Model{Status: StatusIdle}, ApprovalProposed{Proposal: ApprovalProposal{ID: "approval-1", Target: "internal/demo.txt"}})
	updated, effects := Update(pending, ApprovalDecisionSelected{ProposalID: "approval-other", Action: ApprovalActionApprove})
	if len(effects) != 0 {
		t.Fatalf("stale approval effects = %v, want none", effects)
	}
	if updated.Status != StatusApprovalPending || updated.PendingApproval.ID != "approval-1" || !updated.LastApprovalDecision.Stale {
		t.Fatalf("stale approval state = %+v", updated)
	}
}

func TestUpdateHandlesMutationToolProposalsDeterministically(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		message  Message
		wantKind OperationKind
		wantTool MutationToolName
		wantText string
		effectOK func(Effect) bool
	}{
		{
			name:     "edit",
			message:  EditToolProposed{Request: MutationToolRequest{Path: "notes.txt", TargetVersion: "sha256:old", OldText: "old", NewText: "new", ExpectedEffect: "replace text", Source: MutationSourceMetadata{Caller: "test"}}},
			wantKind: OperationEdit,
			wantTool: MutationToolEdit,
			wantText: "edit notes.txt",
			effectOK: func(effect Effect) bool { _, ok := effect.(EditToolEffect); return ok },
		},
		{
			name:     "write",
			message:  WriteToolProposed{Request: MutationToolRequest{Path: "notes.txt", TargetVersion: "missing", Content: "new", ExpectedEffect: "create file", Source: MutationSourceMetadata{Caller: "test"}}},
			wantKind: OperationWrite,
			wantTool: MutationToolWrite,
			wantText: "write notes.txt",
			effectOK: func(effect Effect) bool { _, ok := effect.(WriteToolEffect); return ok },
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := Model{Status: StatusIdle, NextOperation: 8}
			firstModel, firstEffects := Update(model, tc.message)
			secondModel, secondEffects := Update(model, tc.message)
			if !reflect.DeepEqual(firstModel, secondModel) || !reflect.DeepEqual(firstEffects, secondEffects) {
				t.Fatalf("mutation proposal not deterministic:\nfirst=%+v effects=%+v\nsecond=%+v effects=%+v", firstModel, firstEffects, secondModel, secondEffects)
			}
			assertOperationMetadata(t, firstModel.ActiveOperation, OperationMetadata{ID: "op-9", Kind: tc.wantKind, Subject: "notes.txt", Source: "user"})
			if firstModel.ActiveMutation.ToolName != tc.wantTool || firstModel.ActiveMutation.Path != "notes.txt" {
				t.Fatalf("ActiveMutation = %+v", firstModel.ActiveMutation)
			}
			if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "tool", Text: tc.wantText}}) {
				t.Fatalf("Transcript = %#v", got)
			}
			if len(firstEffects) != 1 || !tc.effectOK(firstEffects[0]) {
				t.Fatalf("effects = %#v", firstEffects)
			}
			assertOperationMetadata(t, firstEffects[0].Metadata(), firstModel.ActiveOperation)
		})
	}
}

func TestUpdateHandlesMutationToolResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-2", Kind: OperationWrite, Subject: "notes.txt", Source: "user"}
	model := Model{Status: StatusActive, NextOperation: 2, ActiveOperation: operation, ActiveMutation: MutationToolRequest{ToolName: MutationToolWrite, Path: "notes.txt"}}
	result := MutationToolResult{ToolName: "write", RequestedPath: "notes.txt", WorkspaceRelativePath: "notes.txt", Status: "completed", PreviousVersion: "missing", NewVersion: "sha256:new", BytesWritten: 5}
	completed, effects := Update(model, MutationToolCompleted{Operation: operation, Result: result})
	if len(effects) != 0 || completed.Status != StatusIdle || completed.Result != "write notes.txt: completed 5 bytes" {
		t.Fatalf("completed mutation = %+v effects=%v", completed, effects)
	}
	if completed.ActiveMutation != (MutationToolRequest{}) || !reflect.DeepEqual(completed.LastMutation, result) || completed.ActiveOperation != (OperationMetadata{}) {
		t.Fatalf("mutation state = active %+v last %+v op %+v", completed.ActiveMutation, completed.LastMutation, completed.ActiveOperation)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "write notes.txt: completed 5 bytes"}) {
		t.Fatalf("transcript = %+v", got)
	}

	failure := result
	failure.Error = MutationToolError{Kind: MutationToolErrorTargetVersionMismatch, Message: "target version mismatch"}
	failed, _ := Update(model, MutationToolCompleted{Operation: operation, Result: failure})
	if failed.Transcript[len(failed.Transcript)-1].Kind != "failure" || !strings.Contains(failed.Result, "target_version_mismatch") {
		t.Fatalf("failed mutation = %+v", failed)
	}
}
