package workflow

import (
	"strconv"
	"strings"
)

// Phase identifies Aila's workflow parking state and Agentera protocol phases.
type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseEnvision   Phase = "envision"
	PhaseDeliberate Phase = "deliberate"
	PhasePlan       Phase = "plan"
	PhaseBuild      Phase = "build"
	PhaseAudit      Phase = "audit"
)

var orderedPhases = []Phase{
	PhaseIdle,
	PhaseEnvision,
	PhaseDeliberate,
	PhasePlan,
	PhaseBuild,
	PhaseAudit,
}

var phaseDisplayLabels = map[Phase]string{
	PhaseIdle:       "IDLE",
	PhaseEnvision:   "ENVISION",
	PhaseDeliberate: "DELIBERATE",
	PhasePlan:       "PLAN",
	PhaseBuild:      "BUILD",
	PhaseAudit:      "AUDIT",
}

var protocolSuccessors = map[Phase][]Phase{
	PhaseEnvision:   {PhaseDeliberate, PhasePlan, PhaseBuild},
	PhaseDeliberate: {PhasePlan, PhaseBuild, PhaseEnvision},
	PhasePlan:       {PhaseBuild, PhaseDeliberate},
	PhaseBuild:      {PhaseBuild, PhaseAudit, PhasePlan},
	PhaseAudit:      {PhaseBuild, PhasePlan, PhaseDeliberate, PhaseEnvision},
}

// RuntimeStatus identifies non-phase runtime signals that can decorate workflow display.
type RuntimeStatus string

const (
	RuntimeStatusWaiting RuntimeStatus = "waiting"
	RuntimeStatusStuck   RuntimeStatus = "stuck"
	RuntimeStatusFlagged RuntimeStatus = "flagged"
)

var orderedRuntimeStatuses = []RuntimeStatus{
	RuntimeStatusWaiting,
	RuntimeStatusStuck,
	RuntimeStatusFlagged,
}

var runtimeStatusDisplayLabels = map[RuntimeStatus]string{
	RuntimeStatusWaiting: "waiting",
	RuntimeStatusStuck:   "stuck",
	RuntimeStatusFlagged: "flagged",
}

// SuccessorValidationReason identifies why a protocol successor check failed.
type SuccessorValidationReason string

const (
	SuccessorValidationNonProtocolPhase SuccessorValidationReason = "non_protocol_phase"
	SuccessorValidationInvalidEdge      SuccessorValidationReason = "invalid_edge"
)

// SuccessorValidationError reports a rejected protocol successor check.
type SuccessorValidationError struct {
	From   Phase
	To     Phase
	Reason SuccessorValidationReason
}

func (e SuccessorValidationError) Error() string {
	return "invalid workflow successor from " + quoteBounded(e.From.String(), 48) + " to " + quoteBounded(e.To.String(), 48) + ": " + string(e.Reason)
}

// Phases returns the complete stable workflow phase vocabulary in protocol order.
func Phases() []Phase {
	return append([]Phase(nil), orderedPhases...)
}

// ProtocolSuccessors returns the valid Agentera protocol successors for a phase.
func ProtocolSuccessors(from Phase) ([]Phase, error) {
	successors, ok := protocolSuccessors[from]
	if !ok {
		return nil, SuccessorValidationError{From: from, Reason: SuccessorValidationNonProtocolPhase}
	}
	return append([]Phase(nil), successors...), nil
}

// ValidateProtocolSuccessor checks whether to is an allowed Agentera protocol successor of from.
func ValidateProtocolSuccessor(from, to Phase) error {
	successors, ok := protocolSuccessors[from]
	if !ok {
		return SuccessorValidationError{From: from, To: to, Reason: SuccessorValidationNonProtocolPhase}
	}
	if _, ok := protocolSuccessors[to]; !ok {
		return SuccessorValidationError{From: from, To: to, Reason: SuccessorValidationNonProtocolPhase}
	}
	for _, successor := range successors {
		if successor == to {
			return nil
		}
	}
	return SuccessorValidationError{From: from, To: to, Reason: SuccessorValidationInvalidEdge}
}

// String returns the stable phase identifier.
func (p Phase) String() string {
	return string(p)
}

// DisplayLabel returns the stable user-facing phase label.
func (p Phase) DisplayLabel() string {
	if label, ok := phaseDisplayLabels[p]; ok {
		return label
	}
	return string(p)
}

// ParsePhase resolves stable phase identifier text into a workflow phase.
func ParsePhase(text string) (Phase, error) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, phase := range orderedPhases {
		if normalized == string(phase) {
			return phase, nil
		}
	}
	return "", phaseParseError{text: text}
}

// RuntimeStatuses returns the complete stable non-phase runtime status vocabulary.
func RuntimeStatuses() []RuntimeStatus {
	return append([]RuntimeStatus(nil), orderedRuntimeStatuses...)
}

// String returns the stable runtime status identifier.
func (s RuntimeStatus) String() string {
	return string(s)
}

// DisplayLabel returns the stable user-facing runtime status label.
func (s RuntimeStatus) DisplayLabel() string {
	if label, ok := runtimeStatusDisplayLabels[s]; ok {
		return label
	}
	return string(s)
}

// ParseRuntimeStatus resolves stable status identifier text into a runtime status.
func ParseRuntimeStatus(text string) (RuntimeStatus, error) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, status := range orderedRuntimeStatuses {
		if normalized == string(status) {
			return status, nil
		}
	}
	return "", runtimeStatusParseError{text: text}
}

type phaseParseError struct {
	text string
}

func (e phaseParseError) Error() string {
	return "invalid workflow phase " + quoteBounded(e.text, 64)
}

type runtimeStatusParseError struct {
	text string
}

func (e runtimeStatusParseError) Error() string {
	return "invalid workflow runtime status " + quoteBounded(e.text, 64)
}

func quoteBounded(text string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	return strconv.Quote(text)
}
