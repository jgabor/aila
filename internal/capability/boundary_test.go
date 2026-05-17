package capability

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestCapabilityRegistryIsFixedAndPhaseOwned(t *testing.T) {
	t.Parallel()

	want := []struct {
		name         Name
		owningPhase  workflow.Phase
		crossCutting bool
	}{
		{NameBrief, workflow.PhaseIdle, true},
		{NameVision, workflow.PhaseEnvision, false},
		{NameDiscuss, workflow.PhaseDeliberate, false},
		{NameResearch, workflow.PhaseDeliberate, false},
		{NamePlan, workflow.PhasePlan, false},
		{NameBuild, workflow.PhaseBuild, false},
		{NameOptimize, workflow.PhaseBuild, false},
		{NameDocument, workflow.PhaseBuild, false},
		{NameDesign, workflow.PhaseEnvision, false},
		{NameAudit, workflow.PhaseAudit, false},
		{NameProfile, workflow.PhaseDeliberate, false},
		{NameOrchestrate, workflow.PhaseBuild, false},
	}

	definitions := Definitions()
	if len(definitions) != len(want) {
		t.Fatalf("Definitions() length = %d, want %d", len(definitions), len(want))
	}
	registry := BuiltInRegistry()
	for index, expected := range want {
		definition := definitions[index]
		if definition.Name != expected.name || definition.OwningPhase != expected.owningPhase || definition.CrossCutting != expected.crossCutting {
			t.Fatalf("definition %d = %+v, want name=%s phase=%s crossCutting=%v", index, definition, expected.name, expected.owningPhase, expected.crossCutting)
		}
		if definition.Description == "" {
			t.Fatalf("definition %s has empty description", definition.Name)
		}
		lookup, ok := registry.Lookup(expected.name)
		if !ok || lookup != definition {
			t.Fatalf("Lookup(%s) = %+v ok=%v, want %+v", expected.name, lookup, ok, definition)
		}
	}

	definitions[0] = Definition{Name: NameAudit}
	if got, ok := registry.Lookup(NameBrief); !ok || got.Name != NameBrief {
		t.Fatalf("registry exposed mutable definition storage: %+v ok=%v", got, ok)
	}
	all := registry.All()
	all[0] = Definition{Name: NameAudit}
	if again, ok := registry.Lookup(NameBrief); !ok || again.Name != NameBrief {
		t.Fatalf("registry All exposed mutable definition storage: %+v ok=%v", again, ok)
	}
}

func TestCapabilityRequestsDescribeEffectBoundaries(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "capability-request-1",
		Capability: NamePlan,
		Input:      "make a plan",
		Phase:      workflow.PhaseDeliberate,
		Metadata:   map[string]string{"run_id": "run-1"},
	}
	got := []BoundaryRequest{
		request.RequestModelCall("draft plan through runtime model loop"),
		request.RequestToolExecution("tool.read", "README.md", "read source through runtime effect"),
		request.RequestPermissionCheck("tool.write", "ROADMAP.md", "permission owns write decision"),
		request.RequestArtifactAccess("plan artifact", "state store resolves plan"),
		request.RequestContextAccess("foreground context", "context builder supplies source refs"),
		request.RequestStateWrite("capability run record", "state store records result"),
	}
	wantKinds := []BoundaryKind{
		BoundaryModelCall,
		BoundaryToolExecution,
		BoundaryPermissionCheck,
		BoundaryArtifactAccess,
		BoundaryContextAccess,
		BoundaryStateWrite,
	}
	for index, boundary := range got {
		if boundary.Kind != wantKinds[index] {
			t.Fatalf("boundary %d kind = %q, want %q", index, boundary.Kind, wantKinds[index])
		}
		if boundary.RequestID != request.ID || boundary.Capability != request.Capability || boundary.Reason == "" || boundary.Operation == "" {
			t.Fatalf("boundary %d missing descriptor fields: %+v", index, boundary)
		}
		if boundary.Metadata["run_id"] != "run-1" {
			t.Fatalf("boundary %d metadata = %+v, want copied run_id", index, boundary.Metadata)
		}
	}

	got[0].Metadata["run_id"] = "mutated"
	if request.Metadata["run_id"] != "run-1" {
		t.Fatalf("boundary metadata mutation reached request metadata: %+v", request.Metadata)
	}
}

func TestCapabilityInvocationAcceptsExactlyOneExitPayload(t *testing.T) {
	t.Parallel()

	request := Request{ID: "request-1", Capability: NameBuild, Phase: workflow.PhaseBuild}
	invocation := NewInvocation(request)
	payload := ExitPayload{
		Signal:               ExitComplete,
		Summary:              "build step complete",
		RecommendedSuccessor: workflow.PhaseAudit,
		BoundaryRequests:     []BoundaryRequest{request.RequestArtifactAccess("build log", "runtime resolves artifact")},
	}

	first, err := invocation.Emit(payload)
	if err != nil {
		t.Fatalf("first Emit returned error: %v", err)
	}
	if first.Capability != NameBuild || !invocation.Emitted() {
		t.Fatalf("first Emit = %+v emitted=%v, want build emitted", first, invocation.Emitted())
	}
	if _, err := invocation.Emit(ExitPayload{Signal: ExitFlagged, Summary: "second"}); err == nil {
		t.Fatal("second Emit returned nil error")
	}
	stored, ok := invocation.Payload()
	if !ok || !reflect.DeepEqual(stored, first) {
		t.Fatalf("Payload() = %+v ok=%v, want first %+v", stored, ok, first)
	}
	stored.BoundaryRequests[0].Reason = "mutated"
	again, _ := invocation.Payload()
	if again.BoundaryRequests[0].Reason == "mutated" {
		t.Fatalf("Payload exposed mutable boundary requests: %+v", again)
	}
}

func TestCapabilityExitPayloadValidatesWaitingAndStuckSignals(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name    string
		payload ExitPayload
		wantErr string
	}{
		{name: "waiting needs input", payload: ExitPayload{Signal: ExitWaiting}, wantErr: "needed input"},
		{name: "waiting rejects successor", payload: ExitPayload{Signal: ExitWaiting, NeededInput: "choose next", RecommendedSuccessor: workflow.PhasePlan}, wantErr: "must not recommend"},
		{name: "stuck needs blocker", payload: ExitPayload{Signal: ExitStuck}, wantErr: "blocker"},
		{name: "unknown signal", payload: ExitPayload{Signal: ExitSignal("paused")}, wantErr: "invalid capability exit signal"},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateExitPayload(check.payload)
			if err == nil || !strings.Contains(err.Error(), check.wantErr) {
				t.Fatalf("ValidateExitPayload error = %v, want containing %q", err, check.wantErr)
			}
		})
	}
	for _, payload := range []ExitPayload{
		{Signal: ExitWaiting, NeededInput: "choose a capability"},
		{Signal: ExitStuck, Blocker: "missing source refs"},
		{Signal: ExitFlagged, Summary: "concern recorded"},
	} {
		if err := ValidateExitPayload(payload); err != nil {
			t.Fatalf("ValidateExitPayload(%+v) returned error: %v", payload, err)
		}
	}
}

func TestCapabilityBoundaryAvoidsExecutionPackages(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob capability files: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, "\"")
			for _, forbidden := range []string{
				"os",
				"os/exec",
				"io/fs",
				"net/http",
				"github.com/jgabor/aila/internal/agent",
				"github.com/jgabor/aila/internal/app",
				"github.com/jgabor/aila/internal/permission",
				"github.com/jgabor/aila/internal/runtime",
				"github.com/jgabor/aila/internal/state",
				"github.com/jgabor/aila/internal/tools",
			} {
				if path == forbidden {
					t.Fatalf("%s imports forbidden execution or ownership package %q", file, forbidden)
				}
			}
		}
	}
}
