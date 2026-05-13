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

// Phases returns the complete stable workflow phase vocabulary in protocol order.
func Phases() []Phase {
	return append([]Phase(nil), orderedPhases...)
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

type phaseParseError struct {
	text string
}

func (e phaseParseError) Error() string {
	return "invalid workflow phase " + quoteBounded(e.text, 64)
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
