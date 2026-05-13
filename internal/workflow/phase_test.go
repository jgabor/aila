package workflow

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestPhasesHaveStableIdentifiersAndLabels(t *testing.T) {
	t.Parallel()

	want := []struct {
		phase Phase
		id    string
		label string
	}{
		{PhaseIdle, "idle", "IDLE"},
		{PhaseEnvision, "envision", "ENVISION"},
		{PhaseDeliberate, "deliberate", "DELIBERATE"},
		{PhasePlan, "plan", "PLAN"},
		{PhaseBuild, "build", "BUILD"},
		{PhaseAudit, "audit", "AUDIT"},
	}

	got := Phases()
	if len(got) != len(want) {
		t.Fatalf("len(Phases()) = %d, want %d", len(got), len(want))
	}
	for i, expected := range want {
		if got[i] != expected.phase {
			t.Fatalf("Phases()[%d] = %q, want %q", i, got[i], expected.phase)
		}
		if string(expected.phase) != expected.id {
			t.Fatalf("%s identifier = %q, want %q", expected.label, string(expected.phase), expected.id)
		}
		if expected.phase.DisplayLabel() != expected.label {
			t.Fatalf("%s DisplayLabel() = %q, want %q", expected.id, expected.phase.DisplayLabel(), expected.label)
		}
	}
}

func TestPhasesReturnsCopy(t *testing.T) {
	t.Parallel()

	got := Phases()
	got[0] = PhaseAudit
	if Phases()[0] != PhaseIdle {
		t.Fatalf("Phases() returned mutable package storage")
	}
}

func TestPhaseStringAndFormatUseStableIdentifier(t *testing.T) {
	t.Parallel()

	phase := PhaseBuild
	if phase.String() != "build" {
		t.Fatalf("String() = %q, want build", phase.String())
	}
	if fmt.Sprint(phase) != "build" {
		t.Fatalf("fmt.Sprint(phase) = %q, want build", fmt.Sprint(phase))
	}
	if fmt.Sprintf("%q", phase) != "\"build\"" {
		t.Fatalf("fmt %%q = %q, want quoted build", fmt.Sprintf("%q", phase))
	}
}

func TestParsePhaseAcceptsStableIdentifiers(t *testing.T) {
	t.Parallel()

	for _, phase := range Phases() {
		got, err := ParsePhase(phase.String())
		if err != nil {
			t.Fatalf("ParsePhase(%q) returned error: %v", phase, err)
		}
		if got != phase {
			t.Fatalf("ParsePhase(%q) = %q, want %q", phase, got, phase)
		}
	}
}

func TestParsePhaseNormalizesCaseAndWhitespace(t *testing.T) {
	t.Parallel()

	got, err := ParsePhase("  BUILD\n")
	if err != nil {
		t.Fatalf("ParsePhase returned error: %v", err)
	}
	if got != PhaseBuild {
		t.Fatalf("ParsePhase = %q, want %q", got, PhaseBuild)
	}
}

func TestParsePhaseInvalidErrorIsBounded(t *testing.T) {
	t.Parallel()

	invalid := strings.Repeat("x", 4096)
	phase, err := ParsePhase(invalid)
	if err == nil {
		t.Fatal("ParsePhase returned nil error for invalid phase")
	}
	if phase != "" {
		t.Fatalf("ParsePhase invalid phase = %q, want empty", phase)
	}
	message := err.Error()
	if !strings.HasPrefix(message, "invalid workflow phase ") {
		t.Fatalf("error = %q, want invalid workflow phase prefix", message)
	}
	if len(message) > 120 {
		t.Fatalf("error length = %d, want bounded <= 120: %q", len(message), message)
	}
	if strings.Contains(message, "successor") || strings.Contains(message, "transition") {
		t.Fatalf("invalid parse error implies transition semantics: %q", message)
	}
}

func TestProtocolSuccessorsMatchReferenceTable(t *testing.T) {
	t.Parallel()

	want := map[Phase][]Phase{
		PhaseEnvision:   {PhaseDeliberate, PhasePlan, PhaseBuild},
		PhaseDeliberate: {PhasePlan, PhaseBuild, PhaseEnvision},
		PhasePlan:       {PhaseBuild, PhaseDeliberate},
		PhaseBuild:      {PhaseBuild, PhaseAudit, PhasePlan},
		PhaseAudit:      {PhaseBuild, PhasePlan, PhaseDeliberate, PhaseEnvision},
	}

	for from, expected := range want {
		got, err := ProtocolSuccessors(from)
		if err != nil {
			t.Fatalf("ProtocolSuccessors(%s) returned error: %v", from, err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("ProtocolSuccessors(%s) = %v, want %v", from, got, expected)
		}
		for _, to := range expected {
			if err := ValidateProtocolSuccessor(from, to); err != nil {
				t.Fatalf("ValidateProtocolSuccessor(%s, %s) returned error: %v", from, to, err)
			}
		}
	}
}

func TestValidateProtocolSuccessorRejectsEveryDisallowedProtocolPair(t *testing.T) {
	t.Parallel()

	protocolPhases := []Phase{PhaseEnvision, PhaseDeliberate, PhasePlan, PhaseBuild, PhaseAudit}

	for _, from := range protocolPhases {
		allowed, err := ProtocolSuccessors(from)
		if err != nil {
			t.Fatalf("ProtocolSuccessors(%s) returned error: %v", from, err)
		}
		allowedSet := make(map[Phase]bool, len(allowed))
		for _, to := range allowed {
			allowedSet[to] = true
		}

		for _, to := range protocolPhases {
			if allowedSet[to] {
				continue
			}
			err := ValidateProtocolSuccessor(from, to)
			assertSuccessorValidationError(t, err, from, to, SuccessorValidationInvalidEdge)
		}
	}
}

func TestBuildToDeliberateIsInvalid(t *testing.T) {
	t.Parallel()

	err := ValidateProtocolSuccessor(PhaseBuild, PhaseDeliberate)
	assertSuccessorValidationError(t, err, PhaseBuild, PhaseDeliberate, SuccessorValidationInvalidEdge)
}

func TestIdleIsOutsideProtocolSuccessorValidation(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name string
		from Phase
		to   Phase
	}{
		{name: "idle from", from: PhaseIdle, to: PhaseEnvision},
		{name: "idle to", from: PhaseBuild, to: PhaseIdle},
		{name: "idle both", from: PhaseIdle, to: PhaseIdle},
	}

	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateProtocolSuccessor(check.from, check.to)
			assertSuccessorValidationError(t, err, check.from, check.to, SuccessorValidationNonProtocolPhase)
		})
	}
}

func TestProtocolSuccessorErrorsAreTypedAndBounded(t *testing.T) {
	t.Parallel()

	from := Phase(strings.Repeat("x", 4096))
	to := Phase(strings.Repeat("y", 4096))
	err := ValidateProtocolSuccessor(from, to)
	assertSuccessorValidationError(t, err, from, to, SuccessorValidationNonProtocolPhase)
	if len(err.Error()) > 180 {
		t.Fatalf("error length = %d, want bounded <= 180: %q", len(err.Error()), err.Error())
	}
}

func TestProtocolSuccessorValidationDoesNotExposeMutableState(t *testing.T) {
	t.Parallel()

	got, err := ProtocolSuccessors(PhaseBuild)
	if err != nil {
		t.Fatalf("ProtocolSuccessors returned error: %v", err)
	}
	got[0] = PhaseEnvision

	if err := ValidateProtocolSuccessor(PhaseBuild, PhaseAudit); err != nil {
		t.Fatalf("ValidateProtocolSuccessor after caller mutation returned error: %v", err)
	}
	if err := ValidateProtocolSuccessor(PhaseBuild, PhaseEnvision); err == nil {
		t.Fatalf("ValidateProtocolSuccessor accepted caller-mutated successor")
	}

	again, err := ProtocolSuccessors(PhaseBuild)
	if err != nil {
		t.Fatalf("ProtocolSuccessors returned error: %v", err)
	}
	want := []Phase{PhaseBuild, PhaseAudit, PhasePlan}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("ProtocolSuccessors after caller mutation = %v, want %v", again, want)
	}
}

func assertSuccessorValidationError(t *testing.T, err error, from, to Phase, reason SuccessorValidationReason) {
	t.Helper()

	if err == nil {
		t.Fatal("ValidateProtocolSuccessor returned nil error")
	}
	var validationErr SuccessorValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want SuccessorValidationError", err)
	}
	if validationErr.From != from {
		t.Fatalf("error From = %q, want %q", validationErr.From, from)
	}
	if validationErr.To != to {
		t.Fatalf("error To = %q, want %q", validationErr.To, to)
	}
	if validationErr.Reason != reason {
		t.Fatalf("error Reason = %q, want %q", validationErr.Reason, reason)
	}

	message := err.Error()
	if !strings.HasPrefix(message, "invalid workflow successor ") {
		t.Fatalf("error = %q, want invalid workflow successor prefix", message)
	}
	if len(message) > 180 {
		t.Fatalf("error length = %d, want bounded <= 180: %q", len(message), message)
	}
}
