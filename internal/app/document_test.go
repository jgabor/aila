package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/policy"
)

func TestDocumentCommandWritesDocsThroughMutationHistoryAndStateStore(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.Autonomy = string(permission.AutonomyWrite)
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyWrite))
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})
	controller.workspacePath = workspace

	documented := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteDocument, Kind: policy.CommandInputSlash}, controller.view)

	if documented.Document == nil {
		t.Fatal("document view is nil")
	}
	if documented.Document.Signal != "complete" || documented.Document.Target.Path != "docs/aila-documentation-output.md" || documented.Document.Mutation.Status != "completed" || !documented.Document.Mutation.DecisionAllowed || documented.Document.Mutation.ApprovalRequired {
		t.Fatalf("document view = %+v", documented.Document)
	}
	if documented.Document.RecommendedSuccessor != "audit" || !documented.Document.SuccessorValid || documented.Document.TransitionClaimed || !documented.Document.DisplayOnly {
		t.Fatalf("document successor/display = %+v", documented.Document)
	}
	if documented.Mutation == nil || documented.Mutation.Status != "completed" || documented.Mutation.Path != "docs/aila-documentation-output.md" {
		t.Fatalf("mutation view = %+v", documented.Mutation)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "docs", "aila-documentation-output.md"))
	if err != nil {
		t.Fatalf("read document output: %v", err)
	}
	for _, want := range []string{"# Aila Documentation Alignment", "Capability: document", "mutation safety"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("document output missing %q in:\n%s", want, content)
		}
	}
	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "documentation.md"))
	if err != nil {
		t.Fatalf("read documentation artifact: %v", err)
	}
	for _, want := range []string{"# Documentation Alignment", "Target: docs/aila-documentation-output.md", "Documented the /document command mutation safety path."} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("documentation artifact missing %q in:\n%s", want, artifact)
		}
	}
	if runner.model.LastCapability.Document == nil || runner.model.LastCapability.RecommendedSuccessor != "audit" {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) == 0 || snapshots[len(snapshots)-1].Snapshot.Runtime.Result == "" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
	var sawMutation bool
	for _, event := range historyEvents {
		if event.Event.Kind == history.EventKindMutation && event.Event.Mutation != nil && event.Event.Mutation.RequestID == "document-alignment" {
			sawMutation = true
			if event.Event.Mutation.ToolName != "write" || event.Event.Mutation.Status != "completed" || !reflect.DeepEqual(event.Event.Mutation.ChangedPaths, []string{"docs/aila-documentation-output.md"}) || event.Event.Mutation.CommandSource != "document" {
				t.Fatalf("document mutation history = %+v", event.Event.Mutation)
			}
		}
	}
	if !sawMutation {
		t.Fatalf("history events missing document mutation: %#v", historyEvents)
	}
}

func TestDocumentCommandFlagsDeniedWriteWithoutDocsArtifact(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.Autonomy = string(permission.AutonomyRead)
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), view, runner, func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		return SnapshotPersistenceResult{}
	}, func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult {
		return HistoryPersistenceResult{}
	})
	controller.workspacePath = workspace

	documented := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteDocument, Kind: policy.CommandInputSlash}, controller.view)

	if documented.Document == nil || documented.Document.Signal != "flagged" || documented.Document.Mutation.Status != "denied" || documented.Document.Mutation.DecisionAllowed || !documented.Document.Mutation.ApprovalRequired || len(documented.Document.Caveats) == 0 {
		t.Fatalf("denied document view = %+v", documented.Document)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-documentation-output.md")); !os.IsNotExist(err) {
		t.Fatalf("denied document created output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".aila", "artifacts", "documentation.md")); !os.IsNotExist(err) {
		t.Fatalf("denied document created artifact: %v", err)
	}
	if documented.Mutation == nil || documented.Mutation.Status != "denied" || runner.model.LastCapability.Document == nil || runner.model.LastCapability.RecommendedSuccessor != "build" {
		t.Fatalf("denied runtime state mutation=%+v capability=%+v", documented.Mutation, runner.model.LastCapability)
	}
}
