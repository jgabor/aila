package diagnostic

import (
	"regexp"
	"strings"
)

// MaxMessageBytes bounds diagnostic messages before they reach logs or UI surfaces.
const MaxMessageBytes = 240

// Category identifies the subsystem-shaped failure class without selecting behavior.
type Category string

const (
	CategoryStartup            Category = "startup"
	CategoryState              Category = "state"
	CategoryRuntime            Category = "runtime"
	CategoryEffect             Category = "effect"
	CategoryProviderError      Category = "provider_error"
	CategoryPermissionDecision Category = "permission_decision"
	CategoryCancellation       Category = "cancellation"
	CategorySignalShutdown     Category = "signal_shutdown"
)

// Source identifies where the diagnostic was produced.
type Source string

const (
	SourceStartup       Source = "startup"
	SourceStateOpen     Source = "state.open"
	SourceStateSnapshot Source = "state.session_snapshot"
	SourceStateHistory  Source = "state.history"
	SourceRuntime       Source = "runtime"
	SourceEffect        Source = "effect"
	SourceProvider      Source = "provider"
	SourcePermission    Source = "permission"
	SourceSignal        Source = "signal"
)

// Severity is display and routing metadata only.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// AffectedArtifact names the logical artifact or subsystem affected by a diagnostic.
type AffectedArtifact string

const (
	ArtifactNone              AffectedArtifact = "none"
	ArtifactProjectStore      AffectedArtifact = "project_store"
	ArtifactProjectMetadata   AffectedArtifact = "project_metadata"
	ArtifactArtifactIndex     AffectedArtifact = "artifact_index"
	ArtifactProviderRequest   AffectedArtifact = "provider_request"
	ArtifactPermissionRequest AffectedArtifact = "permission_request"
	ArtifactRuntimeEffect     AffectedArtifact = "runtime_effect"
	ArtifactSessionSnapshot   AffectedArtifact = "session_snapshot"
	ArtifactFakeHistory       AffectedArtifact = "fake_history"
	ArtifactPlan              AffectedArtifact = "plan"
	ArtifactVision            AffectedArtifact = "vision"
	ArtifactDecisions         AffectedArtifact = "decisions"
)

// RecoveryAction is a visible recommendation. It does not execute recovery.
type RecoveryAction string

const (
	RecoveryInspect                        RecoveryAction = "inspect"
	RecoveryManualRepair                   RecoveryAction = "manual_repair"
	RecoveryIgnoreForRun                   RecoveryAction = "ignore_for_this_run"
	RecoveryReinitializeConfirmationNeeded RecoveryAction = "reinitialize_with_confirmation_needed"
)

// Spec is untrusted diagnostic input from package boundaries.
type Spec struct {
	Category         Category
	Source           Source
	Severity         Severity
	Message          string
	AffectedArtifact AffectedArtifact
	RecoveryAction   RecoveryAction
	UserInputNeeded  bool
}

// Diagnostic is a passive, bounded record suitable for app, state, runtime, and TUI transport.
type Diagnostic struct {
	Category         Category         `json:"category"`
	Source           Source           `json:"source"`
	Severity         Severity         `json:"severity"`
	BoundedMessage   string           `json:"bounded_message"`
	AffectedArtifact AffectedArtifact `json:"affected_artifact"`
	RecoveryAction   RecoveryAction   `json:"recovery_action"`
	UserInputNeeded  bool             `json:"user_input_needed"`
}

// New returns a passive diagnostic record with a bounded, redacted message.
func New(spec Spec) Diagnostic {
	return Diagnostic{
		Category:         spec.Category,
		Source:           spec.Source,
		Severity:         spec.Severity,
		BoundedMessage:   boundMessage(redactMessage(spec.Message)),
		AffectedArtifact: spec.AffectedArtifact,
		RecoveryAction:   spec.RecoveryAction,
		UserInputNeeded:  spec.UserInputNeeded,
	}
}

// Passive reports whether the diagnostic category is record-only in M15A.
func (d Diagnostic) Passive() bool {
	return d.Category == CategoryProviderError || d.Category == CategoryPermissionDecision
}

func redactMessage(message string) string {
	message = redactSecrets(strings.TrimSpace(message))
	fields := strings.Fields(message)
	for i, field := range fields {
		fields[i] = redactField(field)
	}
	return strings.Join(fields, " ")
}

var (
	credentialURLPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@`)
	authorizationPattern = regexp.MustCompile(`(?i)\bauthorization\b\s*[:=]?\s*(?:bearer|basic)?\s*\S+`)
	credentialPattern    = regexp.MustCompile(`(?i)\b(?:api[_-]?key|apikey|password|secret|token)\b\s*(?:[:=]\s*)?\S+`)
)

func redactSecrets(message string) string {
	message = credentialURLPattern.ReplaceAllString(message, `${1}[secret]@`)
	message = authorizationPattern.ReplaceAllString(message, `[secret]`)
	return credentialPattern.ReplaceAllString(message, `[secret]`)
}

func redactField(field string) string {
	lower := strings.ToLower(field)
	for _, marker := range []string{"api_key=", "api-key=", "apikey=", "authorization=", "password=", "secret=", "token="} {
		if strings.Contains(lower, marker) {
			return "[secret]"
		}
	}

	trimmed := strings.Trim(field, "\"'`()[]{}<>,.;:")
	if strings.HasPrefix(trimmed, "/") || strings.Contains(field, "=/") {
		return "[path]"
	}
	return field
}

func boundMessage(message string) string {
	if len(message) <= MaxMessageBytes {
		return message
	}
	return strings.TrimSpace(message[:MaxMessageBytes-3]) + "..."
}
