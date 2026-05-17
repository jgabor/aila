package capability

import (
	"context"
	"errors"
	"fmt"

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
	{Name: NameResearch, OwningPhase: workflow.PhaseDeliberate, Description: "adapt concepts, patterns, and solutions"},
	{Name: NamePlan, OwningPhase: workflow.PhasePlan, Description: "scoped planning with behavioral acceptance criteria"},
	{Name: NameBuild, OwningPhase: workflow.PhaseBuild, Description: "execute a single task or plan step and hold"},
	{Name: NameOptimize, OwningPhase: workflow.PhaseBuild, Description: "design and run metric-driven optimization work"},
	{Name: NameDocument, OwningPhase: workflow.PhaseBuild, Description: "align documentation with the project"},
	{Name: NameDesign, OwningPhase: workflow.PhaseEnvision, Description: "create a durable design system"},
	{Name: NameAudit, OwningPhase: workflow.PhaseAudit, Description: "architecture, test, dependency, and project health audit"},
	{Name: NameProfile, OwningPhase: workflow.PhaseDeliberate, Description: "profile decision patterns from previous conversations"},
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
