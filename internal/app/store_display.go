package app

import (
	"errors"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

// StoreDisplayStatus is app-owned, path-safe presentation data for project store startup.
type StoreDisplayStatus struct {
	Status      string
	Source      string
	Detail      string
	Diagnostics []diagnostic.Diagnostic
}

// NewStoreDisplayState injects project store labels without exposing state internals to the TUI.
func NewStoreDisplayState(base tui.ViewState, store StoreDisplayStatus) tui.ViewState {
	base.ProjectStoreStatus = store.Status
	base.ProjectStoreSource = store.Source
	base.ProjectStoreDetail = store.Detail
	base.Diagnostics = diagnosticViews(store.Diagnostics)
	return base
}

func boundedStoreError(err error) string {
	switch {
	case errors.Is(err, state.ErrEmptyWorkspace):
		return "empty workspace"
	case errors.Is(err, state.ErrUnsafeStorePath):
		return "unsafe store path"
	default:
		message := err.Error()
		if strings.HasPrefix(message, "open ") {
			return "open failed"
		}
		if strings.HasPrefix(message, "create store directory ") {
			return "create store directory"
		}
		if index := strings.Index(message, ":"); index > 0 {
			message = message[:index]
		}
		if strings.ContainsAny(message, `/\`) {
			return "open failed"
		}
		if len(message) > 80 {
			message = message[:80]
		}
		if strings.TrimSpace(message) == "" {
			return "open failed"
		}
		return message
	}
}

func storeOpenUnavailableDiagnostic(detail string) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryStartup,
		Source:           diagnostic.SourceStateOpen,
		Severity:         diagnostic.SeverityError,
		Message:          detail,
		AffectedArtifact: diagnostic.ArtifactProjectStore,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
}
