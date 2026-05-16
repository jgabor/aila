package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticContractRepresentsPathSafeRecoveryRecord(t *testing.T) {
	t.Parallel()

	diagnostic := New(Spec{
		Category:         CategoryState,
		Source:           SourceStateOpen,
		Severity:         SeverityError,
		Message:          "open /home/jgabor/work/.aila/project.toml failed token=abc123",
		AffectedArtifact: ArtifactProjectMetadata,
		RecoveryAction:   RecoveryManualRepair,
		UserInputNeeded:  true,
	})

	if diagnostic.Category != CategoryState || diagnostic.Source != SourceStateOpen || diagnostic.Severity != SeverityError {
		t.Fatalf("diagnostic identity = %#v", diagnostic)
	}
	if diagnostic.AffectedArtifact != ArtifactProjectMetadata || diagnostic.RecoveryAction != RecoveryManualRepair || !diagnostic.UserInputNeeded {
		t.Fatalf("diagnostic recovery fields = %#v", diagnostic)
	}
	for _, leaked := range []string{"/home/", ".aila/project.toml", "abc123", "token="} {
		if strings.Contains(diagnostic.BoundedMessage, leaked) {
			t.Fatalf("diagnostic message leaked %q in %q", leaked, diagnostic.BoundedMessage)
		}
	}
	if len(diagnostic.BoundedMessage) > MaxMessageBytes {
		t.Fatalf("diagnostic message length = %d, want <= %d", len(diagnostic.BoundedMessage), MaxMessageBytes)
	}
}

func TestDiagnosticMessageRedactsCommonCredentialForms(t *testing.T) {
	t.Parallel()

	message := strings.Join([]string{
		"Authorization: Bearer sk-live-secret",
		"password: hunter2",
		"token abc123",
		"apikey=plain-key",
		"apiKey=camel-key",
		"api_key=snake-key",
		"https://user:pass@example.com/path",
	}, " ")
	diagnostic := New(Spec{
		Category:         CategoryStartup,
		Source:           SourceStartup,
		Severity:         SeverityError,
		Message:          message,
		AffectedArtifact: ArtifactProviderRequest,
		RecoveryAction:   RecoveryInspect,
	})

	for _, leaked := range []string{
		"Authorization", "Bearer", "sk-live-secret",
		"password", "hunter2",
		"token", "abc123",
		"apikey", "apiKey", "api_key", "plain-key", "camel-key", "snake-key",
		"user:pass", "pass@example.com",
	} {
		if strings.Contains(diagnostic.BoundedMessage, leaked) {
			t.Fatalf("diagnostic message leaked %q in %q", leaked, diagnostic.BoundedMessage)
		}
	}
}

func TestDiagnosticMessageIsBounded(t *testing.T) {
	t.Parallel()

	diagnostic := New(Spec{
		Category:         CategoryRuntime,
		Source:           SourceRuntime,
		Severity:         SeverityWarning,
		Message:          strings.Repeat("x", MaxMessageBytes+100),
		AffectedArtifact: ArtifactRuntimeEffect,
		RecoveryAction:   RecoveryInspect,
	})

	if len(diagnostic.BoundedMessage) > MaxMessageBytes {
		t.Fatalf("diagnostic message length = %d, want <= %d", len(diagnostic.BoundedMessage), MaxMessageBytes)
	}
	if !strings.HasSuffix(diagnostic.BoundedMessage, "...") {
		t.Fatalf("diagnostic message = %q, want truncation marker", diagnostic.BoundedMessage)
	}
}

func TestProviderAndPermissionDiagnosticsArePassiveRecords(t *testing.T) {
	t.Parallel()

	provider := New(Spec{
		Category:         CategoryProviderError,
		Source:           SourceProvider,
		Severity:         SeverityError,
		Message:          "provider returned a bounded error",
		AffectedArtifact: ArtifactProviderRequest,
		RecoveryAction:   RecoveryIgnoreForRun,
	})
	permission := New(Spec{
		Category:         CategoryPermissionDecision,
		Source:           SourcePermission,
		Severity:         SeverityWarning,
		Message:          "permission decision requires inspection",
		AffectedArtifact: ArtifactPermissionRequest,
		RecoveryAction:   RecoveryInspect,
		UserInputNeeded:  true,
	})

	if !provider.Passive() || !permission.Passive() {
		t.Fatalf("provider and permission diagnostics must remain passive records")
	}
	if provider.RecoveryAction != RecoveryIgnoreForRun || permission.RecoveryAction != RecoveryInspect {
		t.Fatalf("recovery actions changed: provider=%q permission=%q", provider.RecoveryAction, permission.RecoveryAction)
	}
}

func TestDiagnosticStructuredFieldsUseStableNames(t *testing.T) {
	t.Parallel()

	diagnostic := New(Spec{
		Category:         CategoryCancellation,
		Source:           SourceRuntime,
		Severity:         SeverityInfo,
		Message:          "run canceled by request",
		AffectedArtifact: ArtifactRuntimeEffect,
		RecoveryAction:   RecoveryInspect,
	})

	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("marshal diagnostic: %v", err)
	}
	for _, field := range []string{
		`"category"`,
		`"source"`,
		`"severity"`,
		`"bounded_message"`,
		`"affected_artifact"`,
		`"recovery_action"`,
		`"user_input_needed"`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("encoded diagnostic %s missing field %s", encoded, field)
		}
	}
}

func TestRecoveryActionsAreRecommendationsOnly(t *testing.T) {
	t.Parallel()

	actions := []RecoveryAction{
		RecoveryInspect,
		RecoveryManualRepair,
		RecoveryIgnoreForRun,
		RecoveryReinitializeConfirmationNeeded,
	}
	for _, action := range actions {
		diagnostic := New(Spec{
			Category:         CategoryState,
			Source:           SourceStateOpen,
			Severity:         SeverityWarning,
			Message:          "recovery action recommendation",
			AffectedArtifact: ArtifactProjectStore,
			RecoveryAction:   action,
		})
		if diagnostic.RecoveryAction != action {
			t.Fatalf("recovery action = %q, want %q", diagnostic.RecoveryAction, action)
		}
	}
}
