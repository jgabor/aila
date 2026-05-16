package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

const (
	// MaxDebugDiagnostics bounds structured diagnostic output count.
	MaxDebugDiagnostics = 8
	// MaxDebugDiagnosticOutputBytes bounds the JSON diagnostic envelope.
	MaxDebugDiagnosticOutputBytes = 8192
)

// DebugDiagnosticsOutput is the documented structured debug diagnostic shape.
type DebugDiagnosticsOutput struct {
	Diagnostics     []diagnostic.Diagnostic `json:"diagnostics"`
	Count           int                     `json:"count"`
	MaxCount        int                     `json:"max_count"`
	MaxMessageBytes int                     `json:"max_message_bytes"`
	MaxOutputBytes  int                     `json:"max_output_bytes"`
}

// DebugDiagnosticsCommandOutput returns bounded startup diagnostics as JSON.
func DebugDiagnosticsCommandOutput(ctx context.Context) (string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve debug diagnostics workspace: %w", err)
	}
	output := NewDebugDiagnosticsOutput(startupDiagnostics(ctx, workspace))
	return output.JSON()
}

// NewDebugDiagnosticsOutput returns the structured debug view for diagnostics.
func NewDebugDiagnosticsOutput(diagnostics []diagnostic.Diagnostic) DebugDiagnosticsOutput {
	bounded := diagnostics
	if len(bounded) > MaxDebugDiagnostics {
		bounded = bounded[:MaxDebugDiagnostics]
	}
	return DebugDiagnosticsOutput{
		Diagnostics:     append([]diagnostic.Diagnostic(nil), bounded...),
		Count:           len(bounded),
		MaxCount:        MaxDebugDiagnostics,
		MaxMessageBytes: diagnostic.MaxMessageBytes,
		MaxOutputBytes:  MaxDebugDiagnosticOutputBytes,
	}
}

// JSON renders the bounded debug diagnostic envelope.
func (o DebugDiagnosticsOutput) JSON() (string, error) {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(o); err != nil {
		return "", fmt.Errorf("marshal debug diagnostics: %w", err)
	}
	if data.Len() > MaxDebugDiagnosticOutputBytes {
		return "", fmt.Errorf("debug diagnostics output exceeded %d bytes", MaxDebugDiagnosticOutputBytes)
	}
	return data.String(), nil
}

func startupDiagnostics(ctx context.Context, workspacePath string) []diagnostic.Diagnostic {
	result, err := state.OpenProjectStoreWithStatus(ctx, workspacePath)
	if err != nil {
		return []diagnostic.Diagnostic{storeOpenUnavailableDiagnostic("project store unavailable: " + boundedStoreError(err))}
	}
	return result.Status.Diagnostics
}

func diagnosticViews(diagnostics []diagnostic.Diagnostic) []tui.DiagnosticView {
	if len(diagnostics) == 0 {
		return nil
	}
	bounded := diagnostics
	if len(bounded) > MaxDebugDiagnostics {
		bounded = bounded[:MaxDebugDiagnostics]
	}
	views := make([]tui.DiagnosticView, 0, len(bounded))
	for _, diagnostic := range bounded {
		views = append(views, tui.DiagnosticView{
			Severity:         string(diagnostic.Severity),
			Source:           string(diagnostic.Source),
			RecoveryAction:   string(diagnostic.RecoveryAction),
			AffectedArtifact: string(diagnostic.AffectedArtifact),
			UserInputNeeded:  diagnostic.UserInputNeeded,
			BoundedMessage:   diagnostic.BoundedMessage,
		})
	}
	return views
}

func diagnosticSummary(diagnostics []diagnostic.Diagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}
	diagnostic := diagnostics[0]
	parts := []string{
		string(diagnostic.Severity),
		string(diagnostic.Source),
		string(diagnostic.AffectedArtifact),
		string(diagnostic.RecoveryAction),
		"user_input_needed=" + strings.ToLower(fmt.Sprint(diagnostic.UserInputNeeded)),
	}
	return strings.Join(parts, " ")
}
