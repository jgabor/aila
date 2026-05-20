package app

import (
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestApplyRuntimeModelToViewExposesSubagentSupervision(t *testing.T) {
	t.Parallel()

	model := runtime.Model{Status: runtime.StatusIdle, Subagents: []runtime.SubagentRun{{
		ID:          "child-active",
		ParentRunID: "parent-run",
		Purpose:     "inspect failing check",
		Status:      runtime.SubagentStatusRunning,
		Summary:     "reading runtime evidence",
		EvidenceLinks: []runtime.SubagentEvidenceLink{{
			ID:      "runtime-test",
			Kind:    "test",
			Command: "go test ./internal/runtime",
		}},
	}}}

	view := applyRuntimeModelToView(tui.IdleEmptyState(), model, t.TempDir())
	if !view.RuntimeActive || view.StatusDetail != "subagent supervision" {
		t.Fatalf("runtime view active=%v detail=%q", view.RuntimeActive, view.StatusDetail)
	}
	if len(view.Subagents) != 1 || view.Subagents[0].ParentRunID != "parent-run" || view.Subagents[0].Purpose != "inspect failing check" || view.Subagents[0].Status != "running" || len(view.Subagents[0].EvidenceLinks) != 1 {
		t.Fatalf("subagent view = %+v", view.Subagents)
	}
	if view.Subagents[0].TransitionClaimed || !view.Subagents[0].DisplayOnly {
		t.Fatalf("subagent ownership flags = %+v", view.Subagents[0])
	}
}

func TestStatusInspectionIncludesSubagentEvidence(t *testing.T) {
	t.Parallel()

	model := runtime.Model{Status: runtime.StatusIdle, Subagents: []runtime.SubagentRun{{
		ID:          "child-failed",
		ParentRunID: "parent-run",
		Purpose:     "verify fallback",
		Status:      runtime.SubagentStatusFailed,
		Summary:     "fixture missing",
		EvidenceLinks: []runtime.SubagentEvidenceLink{{
			ID:   "fixture-proof",
			Kind: "file",
			Path: "internal/tui/testdata/fixtures/multi-agent-active-work/fixture.json",
		}},
	}}}
	view := applyRuntimeModelToView(tui.IdleEmptyState(), model, t.TempDir())

	joined := strings.Join(statusInspectionLines(view, model), "\n")
	for _, want := range []string{
		"subagents: 1",
		"subagent: child-failed parent=parent-run status=failed purpose=verify fallback",
		"subagent summary: child-failed fixture missing",
		"subagent evidence: child-failed fixture-proof kind=file path=internal/tui/testdata/fixtures/multi-agent-active-work/fixture.json",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("status lines missing %q:\n%s", want, joined)
		}
	}
}
