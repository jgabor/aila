package capability

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

// Name identifies one fixed built-in Aila capability.
type Name string

const (
	NameBrief       Name = "brief"
	NameVision      Name = "vision"
	NameDiscuss     Name = "discuss"
	NameResearch    Name = "research"
	NamePlan        Name = "plan"
	NameBuild       Name = "build"
	NameOptimize    Name = "optimize"
	NameDocument    Name = "document"
	NameDesign      Name = "design"
	NameAudit       Name = "audit"
	NameProfile     Name = "profile"
	NameOrchestrate Name = "orchestrate"
)

var orderedDefinitions = []Definition{
	{Name: NameBrief, OwningPhase: workflow.PhaseIdle, CrossCutting: true, Description: "project status briefing and next action"},
	{Name: NameVision, OwningPhase: workflow.PhaseEnvision, Description: "project vision and long-term goals"},
	{Name: NameDiscuss, OwningPhase: workflow.PhaseDeliberate, Description: "structured deliberation before consequential choices"},
	{Name: NameResearch, OwningPhase: workflow.PhaseIdle, CrossCutting: true, Description: "adapt concepts, patterns, and solutions"},
	{Name: NamePlan, OwningPhase: workflow.PhasePlan, Description: "scoped planning with behavioral acceptance criteria"},
	{Name: NameBuild, OwningPhase: workflow.PhaseBuild, Description: "execute a single task or plan step and hold"},
	{Name: NameOptimize, OwningPhase: workflow.PhaseBuild, Description: "design and run metric-driven optimization work"},
	{Name: NameDocument, OwningPhase: workflow.PhaseBuild, Description: "align documentation with the project"},
	{Name: NameDesign, OwningPhase: workflow.PhaseBuild, Description: "create a durable visual identity and UI system"},
	{Name: NameAudit, OwningPhase: workflow.PhaseAudit, Description: "architecture, test, dependency, and project health audit"},
	{Name: NameProfile, OwningPhase: workflow.PhaseIdle, CrossCutting: true, Description: "profile decision patterns from previous conversations"},
	{Name: NameOrchestrate, OwningPhase: workflow.PhaseBuild, Description: "autonomous plan execution with evaluation and retry checks"},
}

// Definition describes one fixed built-in capability without implementing it.
type Definition struct {
	Name         Name
	OwningPhase  workflow.Phase
	CrossCutting bool
	Description  string
}

// Registry is a closed view over the fixed built-in capability vocabulary.
type Registry struct {
	definitions []Definition
}

// BuiltInRegistry returns a registry containing Aila's fixed capabilities.
func BuiltInRegistry() Registry {
	return Registry{definitions: Definitions()}
}

// Definitions returns the fixed built-in capability definitions in display order.
func Definitions() []Definition {
	return append([]Definition(nil), orderedDefinitions...)
}

// Lookup returns one fixed capability definition by name.
func (r Registry) Lookup(name Name) (Definition, bool) {
	for _, definition := range r.definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

// All returns a copy of every fixed capability definition in this registry.
func (r Registry) All() []Definition {
	return append([]Definition(nil), r.definitions...)
}

// Capability is the contract concrete capabilities must satisfy later.
type Capability interface {
	Name() Name
	OwningPhase() workflow.Phase
	Run(context.Context, Request) (ExitPayload, error)
}

// Request is the capability input shape owned by the runtime adapter.
type Request struct {
	ID         string
	Capability Name
	Input      string
	Phase      workflow.Phase
	SourceRefs []SourceRef
	Metadata   map[string]string
}

// ContextField identifies caller-supplied context a model-backed capability
// runner may require before it can safely prepare a turn.
type ContextField string

const (
	ContextFieldCapability ContextField = "capability"
	ContextFieldPhase      ContextField = "phase"
	ContextFieldInput      ContextField = "input"
	ContextFieldSourceRefs ContextField = "source_refs"
	ContextFieldMetadata   ContextField = "metadata"
)

// MissingContextReason is a typed reason for pausing before model-backed
// execution. The runner seam returns these as waiting or stuck payloads instead
// of guessing missing context in the TUI or provider adapter.
type MissingContextReason string

const (
	MissingContextUnknownCapability MissingContextReason = "unknown_capability"
	MissingContextPhase             MissingContextReason = "missing_phase"
	MissingContextInput             MissingContextReason = "missing_input"
	MissingContextSourceRefs        MissingContextReason = "missing_source_refs"
	MissingContextMetadata          MissingContextReason = "missing_metadata"
)

// ExecutionPath describes how a built-in capability may proceed at the runner
// seam without performing provider, filesystem, shell, git, artifact, or network
// IO in deterministic update code.
type ExecutionPath string

const (
	ExecutionPathModelBacked ExecutionPath = "model_backed"
	ExecutionPathWaiting     ExecutionPath = "waiting"
	ExecutionPathStuck       ExecutionPath = "stuck"
)

// ExecutionContract specifies the model-backed runner contract for one fixed
// capability. It is a typed contract, not a plugin declaration.
type ExecutionContract struct {
	Capability     Name
	Path           ExecutionPath
	RequiredFields []ContextField
	OutputField    string
}

// BoundedRequestContext is the exact request context handed to a runner seam.
// It preserves runtime-provided phase, input, source refs, and metadata so the
// TUI cannot reinterpret capability context as presentation state.
type BoundedRequestContext struct {
	RequestID  string
	Capability Name
	Phase      workflow.Phase
	Input      string
	SourceRefs []SourceRef
	Metadata   map[string]string
}

// PreparedExecution is inert data produced by the runner seam. A model-backed
// path includes a model-call boundary request; blocked paths include a typed
// waiting or stuck payload using the existing deterministic exit shape.
type PreparedExecution struct {
	Contract             ExecutionContract
	Context              BoundedRequestContext
	Path                 ExecutionPath
	MissingContextReason MissingContextReason
	ModelCall            *BoundaryRequest
	Waiting              *ExitPayload
	Stuck                *ExitPayload
}

var executionContracts = []ExecutionContract{
	{Capability: NameBrief, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase}, OutputField: "brief"},
	{Capability: NameVision, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "vision"},
	{Capability: NameDiscuss, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "discuss"},
	{Capability: NameResearch, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "research"},
	{Capability: NamePlan, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "plan"},
	{Capability: NameBuild, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "build"},
	{Capability: NameOptimize, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "optimize"},
	{Capability: NameDocument, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "document"},
	{Capability: NameDesign, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "design"},
	{Capability: NameAudit, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "audit"},
	{Capability: NameProfile, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "profile"},
	{Capability: NameOrchestrate, Path: ExecutionPathModelBacked, RequiredFields: []ContextField{ContextFieldCapability, ContextFieldPhase, ContextFieldInput}, OutputField: "orchestrate"},
}

// BuiltInExecutionContracts returns the model-backed runner contracts for the
// fixed built-in registry in display order.
func BuiltInExecutionContracts() []ExecutionContract {
	contracts := append([]ExecutionContract(nil), executionContracts...)
	for index := range contracts {
		contracts[index].RequiredFields = append([]ContextField(nil), contracts[index].RequiredFields...)
	}
	return contracts
}

// LookupExecutionContract returns the runner contract for one fixed capability.
func LookupExecutionContract(name Name) (ExecutionContract, bool) {
	for _, contract := range executionContracts {
		if contract.Capability == name {
			contract.RequiredFields = append([]ContextField(nil), contract.RequiredFields...)
			return contract, true
		}
	}
	return ExecutionContract{}, false
}

// PrepareModelExecution builds the inert, bounded request context for the
// model-backed runner seam. It performs no IO and never calls a provider.
func PrepareModelExecution(request Request) PreparedExecution {
	if request.Capability == "" {
		request.Capability = NameBrief
	}
	if request.Phase == "" && request.Capability == NameBrief {
		request.Phase = workflow.PhaseIdle
	}
	context := BoundedRequestContext{
		RequestID:  request.ID,
		Capability: request.Capability,
		Phase:      request.Phase,
		Input:      request.Input,
		SourceRefs: append([]SourceRef(nil), request.SourceRefs...),
		Metadata:   cloneMap(request.Metadata),
	}
	contract, ok := LookupExecutionContract(request.Capability)
	if !ok {
		payload := ExitPayload{
			Capability: request.Capability,
			Signal:     ExitStuck,
			Blocker:    fmt.Sprintf("unsupported built-in capability %q", request.Capability),
			Attempted:  false,
		}
		return PreparedExecution{Context: context, Path: ExecutionPathStuck, MissingContextReason: MissingContextUnknownCapability, Stuck: &payload}
	}
	if reason, ok := missingRequiredContext(contract, context); ok {
		payload := ExitPayload{
			Capability:  request.Capability,
			Signal:      ExitWaiting,
			NeededInput: string(reason),
			Summary:     "Capability execution is waiting for required model context.",
			Attempted:   false,
		}
		return PreparedExecution{Contract: contract, Context: context, Path: ExecutionPathWaiting, MissingContextReason: reason, Waiting: &payload}
	}
	modelCall := request.RequestModelCall("run fixed built-in capability through the provider-backed agent runner")
	return PreparedExecution{Contract: contract, Context: context, Path: ExecutionPathModelBacked, ModelCall: &modelCall}
}

func missingRequiredContext(contract ExecutionContract, context BoundedRequestContext) (MissingContextReason, bool) {
	for _, field := range contract.RequiredFields {
		switch field {
		case ContextFieldCapability:
			if context.Capability == "" {
				return MissingContextUnknownCapability, true
			}
		case ContextFieldPhase:
			if context.Phase == "" {
				return MissingContextPhase, true
			}
		case ContextFieldInput:
			if strings.TrimSpace(context.Input) == "" {
				return MissingContextInput, true
			}
		case ContextFieldSourceRefs:
			if len(context.SourceRefs) == 0 {
				return MissingContextSourceRefs, true
			}
		case ContextFieldMetadata:
			if len(context.Metadata) == 0 {
				return MissingContextMetadata, true
			}
		}
	}
	return "", false
}

// SourceRef points at evidence supplied to a capability request or exit.
type SourceRef struct {
	ID        string
	Kind      string
	Path      string
	LineStart int
	LineEnd   int
	Command   string
	Excerpt   string
}

// BoundaryKind identifies an effect or store boundary a capability may request.
type BoundaryKind string

const (
	BoundaryModelCall       BoundaryKind = "model_call"
	BoundaryToolExecution   BoundaryKind = "tool_execution"
	BoundaryPermissionCheck BoundaryKind = "permission_check"
	BoundaryArtifactAccess  BoundaryKind = "artifact_access"
	BoundaryContextAccess   BoundaryKind = "context_access"
	BoundaryStateAccess     BoundaryKind = "state_access"
	BoundaryStateWrite      BoundaryKind = "state_write"
)

// BoundaryRequest is an inert descriptor. It is not executable by capabilities.
type BoundaryRequest struct {
	Kind       BoundaryKind
	RequestID  string
	Capability Name
	Reason     string
	Operation  string
	Target     string
	Metadata   map[string]string
}

// RequestModelCall describes a model call the runtime may choose to execute.
func (r Request) RequestModelCall(reason string) BoundaryRequest {
	return r.boundary(BoundaryModelCall, reason, "model.call", "")
}

// RequestToolExecution describes a tool effect the runtime may choose to execute.
func (r Request) RequestToolExecution(operation, target, reason string) BoundaryRequest {
	return r.boundary(BoundaryToolExecution, reason, operation, target)
}

// RequestPermissionCheck describes a permission evaluation the runtime may choose to execute.
func (r Request) RequestPermissionCheck(operation, target, reason string) BoundaryRequest {
	return r.boundary(BoundaryPermissionCheck, reason, operation, target)
}

// RequestArtifactAccess describes artifact resolver or store access requested by a capability.
func (r Request) RequestArtifactAccess(target, reason string) BoundaryRequest {
	return r.boundary(BoundaryArtifactAccess, reason, "artifact.access", target)
}

// RequestContextAccess describes context builder access requested by a capability.
func (r Request) RequestContextAccess(target, reason string) BoundaryRequest {
	return r.boundary(BoundaryContextAccess, reason, "context.access", target)
}

// RequestStateAccess describes a runtime/store state read requested by a capability.
func (r Request) RequestStateAccess(target, reason string) BoundaryRequest {
	return r.boundary(BoundaryStateAccess, reason, "state.access", target)
}

// RequestStateWrite describes a state-store write requested by a capability.
func (r Request) RequestStateWrite(target, reason string) BoundaryRequest {
	return r.boundary(BoundaryStateWrite, reason, "state.write", target)
}

func (r Request) boundary(kind BoundaryKind, reason, operation, target string) BoundaryRequest {
	return BoundaryRequest{
		Kind:       kind,
		RequestID:  r.ID,
		Capability: r.Capability,
		Reason:     reason,
		Operation:  operation,
		Target:     target,
		Metadata:   cloneMap(r.Metadata),
	}
}

// ExitSignal is the single protocol signal emitted by a capability invocation.
type ExitSignal string

const (
	ExitComplete ExitSignal = "complete"
	ExitFlagged  ExitSignal = "flagged"
	ExitWaiting  ExitSignal = "waiting"
	ExitStuck    ExitSignal = "stuck"
)

// ArtifactRef identifies an artifact touched or requested through a boundary.
type ArtifactRef struct {
	ID   string
	Kind string
	Path string
}

// ExitPayload is the one protocol result a capability invocation may emit.
type ExitPayload struct {
	Capability           Name
	Signal               ExitSignal
	Summary              string
	Concerns             []string
	NeededInput          string
	Blocker              string
	Attempted            bool
	NextAction           string
	RecommendedSuccessor workflow.Phase
	ArtifactRefs         []ArtifactRef
	SourceRefs           []SourceRef
	BoundaryRequests     []BoundaryRequest
	Plan                 *PlanOutput
	Build                *BuildOutput
	Audit                *AuditOutput
	Vision               *VisionOutput
	Discuss              *DiscussOutput
	Research             *ResearchOutput
	Profile              *ProfileOutput
	Optimize             *OptimizeOutput
	Document             *DocumentOutput
	Design               *DesignOutput
	Orchestrate          *OrchestrateOutput
}

// Invocation guards the one-exit-payload rule for a capability run.
type Invocation struct {
	request Request
	emitted bool
	payload ExitPayload
}

// NewInvocation creates a single-use exit guard for one capability request.
func NewInvocation(request Request) Invocation {
	return Invocation{request: request}
}

// Emit accepts the first valid exit payload and rejects every later one.
func (i *Invocation) Emit(payload ExitPayload) (ExitPayload, error) {
	if i.emitted {
		return ExitPayload{}, errors.New("capability invocation already emitted an exit payload")
	}
	if payload.Capability == "" {
		payload.Capability = i.request.Capability
	}
	if err := ValidateExitPayload(payload); err != nil {
		return ExitPayload{}, err
	}
	i.emitted = true
	i.payload = cloneExitPayload(payload)
	return cloneExitPayload(payload), nil
}

// Emitted reports whether this invocation already accepted its single exit.
func (i Invocation) Emitted() bool {
	return i.emitted
}

// Payload returns the accepted payload and whether one exists.
func (i Invocation) Payload() (ExitPayload, bool) {
	if !i.emitted {
		return ExitPayload{}, false
	}
	return cloneExitPayload(i.payload), true
}

// ValidateExitPayload checks protocol shape without changing workflow phase.
func ValidateExitPayload(payload ExitPayload) error {
	switch payload.Signal {
	case ExitComplete, ExitFlagged:
	case ExitWaiting:
		if payload.NeededInput == "" {
			return errors.New("waiting capability exit requires needed input")
		}
		if payload.RecommendedSuccessor != "" {
			return errors.New("waiting capability exit must not recommend a successor")
		}
	case ExitStuck:
		if payload.Blocker == "" {
			return errors.New("stuck capability exit requires a blocker")
		}
	default:
		return fmt.Errorf("invalid capability exit signal %q", payload.Signal)
	}
	return nil
}

func cloneExitPayload(payload ExitPayload) ExitPayload {
	payload.Concerns = append([]string(nil), payload.Concerns...)
	payload.ArtifactRefs = append([]ArtifactRef(nil), payload.ArtifactRefs...)
	payload.SourceRefs = append([]SourceRef(nil), payload.SourceRefs...)
	payload.BoundaryRequests = append([]BoundaryRequest(nil), payload.BoundaryRequests...)
	if payload.Plan != nil {
		plan := *payload.Plan
		plan.Items = append([]PlanItem(nil), payload.Plan.Items...)
		for index := range plan.Items {
			plan.Items[index].Acceptance = append([]string(nil), plan.Items[index].Acceptance...)
			plan.Items[index].SourceRefIDs = append([]string(nil), plan.Items[index].SourceRefIDs...)
		}
		plan.Blockers = append([]string(nil), payload.Plan.Blockers...)
		plan.SourceRefs = append([]SourceRef(nil), payload.Plan.SourceRefs...)
		payload.Plan = &plan
	}
	if payload.Build != nil {
		build := *payload.Build
		build.ChangedPaths = append([]string(nil), payload.Build.ChangedPaths...)
		build.Blockers = append([]string(nil), payload.Build.Blockers...)
		build.Caveats = append([]string(nil), payload.Build.Caveats...)
		build.SourceRefs = append([]SourceRef(nil), payload.Build.SourceRefs...)
		payload.Build = &build
	}
	if payload.Audit != nil {
		audit := *payload.Audit
		audit.Findings = append([]AuditFinding(nil), payload.Audit.Findings...)
		for index := range audit.Findings {
			audit.Findings[index].SourceRefIDs = append([]string(nil), payload.Audit.Findings[index].SourceRefIDs...)
			audit.Findings[index].NextActions = append([]string(nil), payload.Audit.Findings[index].NextActions...)
		}
		audit.NextActions = append([]string(nil), payload.Audit.NextActions...)
		audit.Caveats = append([]string(nil), payload.Audit.Caveats...)
		audit.SourceRefs = append([]SourceRef(nil), payload.Audit.SourceRefs...)
		payload.Audit = &audit
	}
	if payload.Vision != nil {
		vision := *payload.Vision
		vision.Principles = append([]string(nil), payload.Vision.Principles...)
		vision.LongTermGoals = append([]string(nil), payload.Vision.LongTermGoals...)
		vision.Blockers = append([]string(nil), payload.Vision.Blockers...)
		vision.SourceRefs = append([]SourceRef(nil), payload.Vision.SourceRefs...)
		payload.Vision = &vision
	}
	if payload.Discuss != nil {
		discussion := *payload.Discuss
		discussion.Options = append([]DiscussOption(nil), payload.Discuss.Options...)
		discussion.Blockers = append([]string(nil), payload.Discuss.Blockers...)
		discussion.SourceRefs = append([]SourceRef(nil), payload.Discuss.SourceRefs...)
		payload.Discuss = &discussion
	}
	if payload.Research != nil {
		research := *payload.Research
		research.Patterns = append([]ResearchPattern(nil), payload.Research.Patterns...)
		for index := range research.Patterns {
			research.Patterns[index].EvidenceRefIDs = append([]string(nil), payload.Research.Patterns[index].EvidenceRefIDs...)
		}
		research.Evidence = append([]ResearchEvidence(nil), payload.Research.Evidence...)
		research.Caveats = append([]string(nil), payload.Research.Caveats...)
		research.SourceRefs = append([]SourceRef(nil), payload.Research.SourceRefs...)
		payload.Research = &research
	}

	if payload.Optimize != nil {
		optimize := *payload.Optimize
		optimize.Evidence = append([]OptimizeEvidence(nil), payload.Optimize.Evidence...)
		optimize.Caveats = append([]string(nil), payload.Optimize.Caveats...)
		optimize.SourceRefs = append([]SourceRef(nil), payload.Optimize.SourceRefs...)
		payload.Optimize = &optimize
	}

	if payload.Document != nil {
		document := *payload.Document
		document.Plan.Steps = append([]string(nil), payload.Document.Plan.Steps...)
		document.ChangedDocs = append([]DocumentChange(nil), payload.Document.ChangedDocs...)
		document.DiffLines = append([]string(nil), payload.Document.DiffLines...)
		document.Caveats = append([]string(nil), payload.Document.Caveats...)
		document.SourceRefs = append([]SourceRef(nil), payload.Document.SourceRefs...)
		payload.Document = &document
	}

	if payload.Design != nil {
		design := *payload.Design
		design.Decisions = append([]DesignDecision(nil), payload.Design.Decisions...)
		design.ReviewPrompts = append([]DesignReviewPrompt(nil), payload.Design.ReviewPrompts...)
		design.Caveats = append([]string(nil), payload.Design.Caveats...)
		design.SourceRefs = append([]SourceRef(nil), payload.Design.SourceRefs...)
		payload.Design = &design
	}

	if payload.Orchestrate != nil {
		orchestrate := *payload.Orchestrate
		orchestrate.Cycles = append([]OrchestrateCycle(nil), payload.Orchestrate.Cycles...)
		for index := range orchestrate.Cycles {
			orchestrate.Cycles[index].ChildWorkIDs = append([]string(nil), payload.Orchestrate.Cycles[index].ChildWorkIDs...)
			orchestrate.Cycles[index].EvidenceRefIDs = append([]string(nil), payload.Orchestrate.Cycles[index].EvidenceRefIDs...)
		}
		orchestrate.ChildWork = append([]OrchestrateChildWork(nil), payload.Orchestrate.ChildWork...)
		for index := range orchestrate.ChildWork {
			orchestrate.ChildWork[index].EvidenceRefIDs = append([]string(nil), payload.Orchestrate.ChildWork[index].EvidenceRefIDs...)
		}
		orchestrate.Decisions = append([]OrchestrateDecision(nil), payload.Orchestrate.Decisions...)
		orchestrate.Evidence = append([]OrchestrateEvidence(nil), payload.Orchestrate.Evidence...)
		orchestrate.Blockers = append([]string(nil), payload.Orchestrate.Blockers...)
		orchestrate.Caveats = append([]string(nil), payload.Orchestrate.Caveats...)
		orchestrate.SourceRefs = append([]SourceRef(nil), payload.Orchestrate.SourceRefs...)
		payload.Orchestrate = &orchestrate
	}

	if payload.Profile != nil {
		profile := *payload.Profile
		profile.DecisionSignals = append([]ProfileDecisionSignal(nil), payload.Profile.DecisionSignals...)
		for index := range profile.DecisionSignals {
			profile.DecisionSignals[index].EvidenceRefIDs = append([]string(nil), payload.Profile.DecisionSignals[index].EvidenceRefIDs...)
		}
		profile.UpdateSuggestions = append([]ProfileUpdateSuggestion(nil), payload.Profile.UpdateSuggestions...)
		for index := range profile.UpdateSuggestions {
			profile.UpdateSuggestions[index].EvidenceRefIDs = append([]string(nil), payload.Profile.UpdateSuggestions[index].EvidenceRefIDs...)
		}
		profile.Evidence = append([]ProfileEvidence(nil), payload.Profile.Evidence...)
		profile.Caveats = append([]string(nil), payload.Profile.Caveats...)
		profile.SourceRefs = append([]SourceRef(nil), payload.Profile.SourceRefs...)
		payload.Profile = &profile
	}
	for index := range payload.BoundaryRequests {
		payload.BoundaryRequests[index].Metadata = cloneMap(payload.BoundaryRequests[index].Metadata)
	}
	return payload
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
