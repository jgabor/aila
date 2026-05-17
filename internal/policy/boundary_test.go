package policy

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/workflow"
)

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestCommandRoutesAreClosedPolicyRecommendations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  CommandRoute
	}{
		{name: "new", input: "/new", want: CommandRouteNew},
		{name: "clear", input: "/clear", want: CommandRouteClear},
		{name: "continue", input: "/continue", want: CommandRouteContinue},
		{name: "editor", input: "/editor", want: CommandRouteEditor},
		{name: "model", input: "/model", want: CommandRouteModel},
		{name: "model utility", input: "/model --utility", want: CommandRouteModel},
		{name: "auto", input: "/auto", want: CommandRouteAuto},
		{name: "status", input: "/status", want: CommandRouteStatus},
		{name: "review", input: "/review", want: CommandRouteReview},
		{name: "help", input: "/help", want: CommandRouteHelp},
		{name: "history", input: "/history", want: CommandRouteHistory},
		{name: "compact", input: "/compact", want: CommandRouteCompact},
		{name: "diff", input: "/diff", want: CommandRouteDiff},
		{name: "undo", input: "/undo", want: CommandRouteUndo},
		{name: "redo", input: "/redo", want: CommandRouteRedo},
		{name: "quit", input: "/quit", want: CommandRouteQuit},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := RecommendSlashCommand(tc.input)
			if !ok {
				t.Fatalf("RecommendSlashCommand(%q) did not match", tc.input)
			}
			if got.Route != tc.want || got.Kind != CommandInputSlash {
				t.Fatalf("RecommendSlashCommand(%q) = %+v, want route %q slash", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompactCommandRoutesSlashAndShortcutToSameClosedRoute(t *testing.T) {
	t.Parallel()

	slash, ok := RecommendSlashCommand("/compact")
	if !ok || slash.Route != CommandRouteCompact || slash.Kind != CommandInputSlash {
		t.Fatalf("/compact recommendation = %+v, want compact slash", slash)
	}
	shortcut, ok := RecommendShortcut("ctrl+x", "k")
	if !ok || shortcut.Route != CommandRouteCompact || shortcut.Kind != CommandInputShortcut {
		t.Fatalf("ctrl+x k recommendation = %+v, want compact shortcut", shortcut)
	}
	if slash.Route != shortcut.Route {
		t.Fatalf("compact route mismatch: slash=%q shortcut=%q", slash.Route, shortcut.Route)
	}
}

func TestModelAndAutonomyRoutesCarryClosedTargets(t *testing.T) {
	t.Parallel()

	model, ok := RecommendSlashCommand("/model")
	if !ok || model.Route != CommandRouteModel || model.Target != CommandTargetPrimaryModel || model.Kind != CommandInputSlash {
		t.Fatalf("/model recommendation = %+v, want primary model slash route", model)
	}
	utility, ok := RecommendSlashCommand("/model --utility")
	if !ok || utility.Route != CommandRouteModel || utility.Target != CommandTargetUtilityModel || utility.Kind != CommandInputSlash {
		t.Fatalf("/model --utility recommendation = %+v, want utility model slash route", utility)
	}
	auto, ok := RecommendSlashCommand("/auto")
	if !ok || auto.Route != CommandRouteAuto || auto.Target != CommandTargetAutonomy || auto.Kind != CommandInputSlash {
		t.Fatalf("/auto recommendation = %+v, want autonomy slash route", auto)
	}
	modelShortcut, ok := RecommendShortcut("ctrl+x", "m")
	if !ok || modelShortcut.Route != CommandRouteModel || modelShortcut.Target != CommandTargetPrimaryModel || modelShortcut.Kind != CommandInputShortcut {
		t.Fatalf("ctrl+x m recommendation = %+v, want primary model shortcut route", modelShortcut)
	}
	autoShortcut, ok := RecommendShortcut("ctrl+x", "a")
	if !ok || autoShortcut.Route != CommandRouteAuto || autoShortcut.Target != CommandTargetAutonomy || autoShortcut.Kind != CommandInputShortcut {
		t.Fatalf("ctrl+x a recommendation = %+v, want autonomy shortcut route", autoShortcut)
	}
}

func TestShellPrefixRecommendationsAreClosed(t *testing.T) {
	t.Parallel()

	executable, ok := RecommendShellPrefix("!git status --short")
	if !ok || executable.Kind != ShellPrefixExecutable || executable.ExactInput != "!git status --short" || executable.CommandText != "git status --short" {
		t.Fatalf("executable shell prefix = %+v, ok=%v", executable, ok)
	}

	summarized, ok := RecommendShellPrefix("!!git status --short")
	if !ok || summarized.Kind != ShellPrefixSummarized || summarized.ExactInput != "!!git status --short" || summarized.CommandText != "git status --short" {
		t.Fatalf("summarized shell prefix = %+v, ok=%v", summarized, ok)
	}

	for _, input := range []string{"hello", "/status", "!", "!!"} {
		if got, ok := RecommendShellPrefix(input); ok {
			t.Fatalf("RecommendShellPrefix(%q) = %+v, want no shell prefix", input, got)
		}
	}
}

func TestSlashAndShortcutRoutesShareRoute(t *testing.T) {
	t.Parallel()

	newSlash, ok := RecommendSlashCommand("/new")
	if !ok {
		t.Fatal("/new did not match")
	}
	newShortcut, ok := RecommendShortcut("ctrl+x", "n")
	if !ok {
		t.Fatal("ctrl+x n did not match")
	}
	if newSlash.Route != newShortcut.Route || newSlash.Route != CommandRouteNew {
		t.Fatalf("new route mismatch: slash=%+v shortcut=%+v", newSlash, newShortcut)
	}
	continueSlash, ok := RecommendSlashCommand("/continue")
	if !ok {
		t.Fatal("/continue did not match")
	}
	continueShortcut, ok := RecommendShortcut("ctrl+x", "c")
	if !ok {
		t.Fatal("ctrl+x c did not match")
	}
	if continueSlash.Route != continueShortcut.Route || continueSlash.Route != CommandRouteContinue {
		t.Fatalf("continue route mismatch: slash=%+v shortcut=%+v", continueSlash, continueShortcut)
	}
	clearSlash, ok := RecommendSlashCommand("/clear")
	if !ok {
		t.Fatal("/clear did not match")
	}
	if clearSlash.Route != CommandRouteClear || clearSlash.Kind != CommandInputSlash {
		t.Fatalf("clear route = %+v, want slash-only clear route", clearSlash)
	}

	editorSlash, ok := RecommendSlashCommand("/editor")
	if !ok {
		t.Fatal("/editor did not match")
	}
	editorShortcut, ok := RecommendShortcut("ctrl+x", "e")
	if !ok {
		t.Fatal("ctrl+x e did not match")
	}
	if editorSlash.Route != editorShortcut.Route || editorSlash.Route != CommandRouteEditor {
		t.Fatalf("editor route mismatch: slash=%+v shortcut=%+v", editorSlash, editorShortcut)
	}

	modelSlash, ok := RecommendSlashCommand("/model")
	if !ok {
		t.Fatal("/model did not match")
	}
	modelShortcut, ok := RecommendShortcut("ctrl+x", "m")
	if !ok {
		t.Fatal("ctrl+x m did not match")
	}
	if modelSlash.Route != modelShortcut.Route || modelSlash.Target != modelShortcut.Target || modelSlash.Route != CommandRouteModel {
		t.Fatalf("model route mismatch: slash=%+v shortcut=%+v", modelSlash, modelShortcut)
	}

	autoSlash, ok := RecommendSlashCommand("/auto")
	if !ok {
		t.Fatal("/auto did not match")
	}
	autoShortcut, ok := RecommendShortcut("ctrl+x", "a")
	if !ok {
		t.Fatal("ctrl+x a did not match")
	}
	if autoSlash.Route != autoShortcut.Route || autoSlash.Target != autoShortcut.Target || autoSlash.Route != CommandRouteAuto {
		t.Fatalf("auto route mismatch: slash=%+v shortcut=%+v", autoSlash, autoShortcut)
	}

	statusSlash, ok := RecommendSlashCommand("/status")
	if !ok {
		t.Fatal("/status did not match")
	}
	statusShortcut, ok := RecommendShortcut("ctrl+x", "s")
	if !ok {
		t.Fatal("ctrl+x s did not match")
	}
	if statusSlash.Route != statusShortcut.Route || statusSlash.Route != CommandRouteStatus {
		t.Fatalf("status route mismatch: slash=%+v shortcut=%+v", statusSlash, statusShortcut)
	}
	reviewSlash, ok := RecommendSlashCommand("/review")
	if !ok {
		t.Fatal("/review did not match")
	}
	reviewShortcut, ok := RecommendShortcut("ctrl+x", "i")
	if !ok {
		t.Fatal("ctrl+x i did not match")
	}
	if reviewSlash.Route != reviewShortcut.Route || reviewSlash.Route != CommandRouteReview {
		t.Fatalf("review route mismatch: slash=%+v shortcut=%+v", reviewSlash, reviewShortcut)
	}
	historySlash, ok := RecommendSlashCommand("/history")
	if !ok {
		t.Fatal("/history did not match")
	}
	historyShortcut, ok := RecommendShortcut("ctrl+x", "h")
	if !ok {
		t.Fatal("ctrl+x h did not match")
	}
	if historySlash.Route != historyShortcut.Route || historySlash.Route != CommandRouteHistory {
		t.Fatalf("history route mismatch: slash=%+v shortcut=%+v", historySlash, historyShortcut)
	}

	diffSlash, ok := RecommendSlashCommand("/diff")
	if !ok {
		t.Fatal("/diff did not match")
	}
	diffShortcut, ok := RecommendShortcut("ctrl+x", "d")
	if !ok {
		t.Fatal("ctrl+x d did not match")
	}
	if diffSlash.Route != diffShortcut.Route || diffSlash.Route != CommandRouteDiff {
		t.Fatalf("diff route mismatch: slash=%+v shortcut=%+v", diffSlash, diffShortcut)
	}

	undoSlash, ok := RecommendSlashCommand("/undo")
	if !ok {
		t.Fatal("/undo did not match")
	}
	undoShortcut, ok := RecommendShortcut("ctrl+x", "u")
	if !ok {
		t.Fatal("ctrl+x u did not match")
	}
	if undoSlash.Route != undoShortcut.Route || undoSlash.Route != CommandRouteUndo {
		t.Fatalf("undo route mismatch: slash=%+v shortcut=%+v", undoSlash, undoShortcut)
	}

	redoSlash, ok := RecommendSlashCommand("/redo")
	if !ok {
		t.Fatal("/redo did not match")
	}
	redoShortcut, ok := RecommendShortcut("ctrl+x", "r")
	if !ok {
		t.Fatal("ctrl+x r did not match")
	}
	if redoSlash.Route != redoShortcut.Route || redoSlash.Route != CommandRouteRedo {
		t.Fatalf("redo route mismatch: slash=%+v shortcut=%+v", redoSlash, redoShortcut)
	}

	quitSlash, ok := RecommendSlashCommand("/quit")
	if !ok {
		t.Fatal("/quit did not match")
	}
	quitShortcut, ok := RecommendShortcut("ctrl+x", "q")
	if !ok {
		t.Fatal("ctrl+x q did not match")
	}
	if quitSlash.Route != quitShortcut.Route || quitSlash.Route != CommandRouteQuit {
		t.Fatalf("quit route mismatch: slash=%+v shortcut=%+v", quitSlash, quitShortcut)
	}
}

func TestCommandBoundaryRejectsDeferredFamilies(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"/new now",
		"/clear now",
		"/continue latest",
		"/status now",
		"/editor now",
		"/model deepseek",
		"/model --primary",
		"/model --utility now",
		"/auto read",
		"/help commands",
		"/review now",
		"/quit --force",
		"/undo now",
		"/redo --last",
		"/q",
		"/exit",
		"!git status",
		"git status",
		"run tests",
	} {
		if got, ok := RecommendSlashCommand(input); ok {
			t.Fatalf("RecommendSlashCommand(%q) = %+v, want no route", input, got)
		}
	}

	for _, shortcut := range []struct {
		prefix string
		key    string
	}{
		{prefix: "ctrl+x", key: "new"},
		{prefix: "ctrl+x", key: "clear"},
		{prefix: "ctrl+x", key: "continue"},
		{prefix: "ctrl+x", key: "editor"},
		{prefix: "ctrl+x", key: "status"},
		{prefix: "ctrl+x", key: "model"},
		{prefix: "ctrl+x", key: "auto"},
		{prefix: "ctrl+x", key: "review"},
		{prefix: "ctrl+x", key: "undo"},
		{prefix: "ctrl+c", key: "q"},
		{prefix: "", key: "q"},
	} {
		if got, ok := RecommendShortcut(shortcut.prefix, shortcut.key); ok {
			t.Fatalf("RecommendShortcut(%q, %q) = %+v, want no route", shortcut.prefix, shortcut.key, got)
		}
	}
}

func TestCommandBoundaryStaysPureAndClosed(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "command.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse command.go: %v", err)
	}
	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
	for _, forbidden := range []string{
		"os",
		"os/exec",
		"io/fs",
		"net/http",
		"github.com/jgabor/aila/internal/app",
		"github.com/jgabor/aila/internal/agent",
		"github.com/jgabor/aila/internal/capability",
		"github.com/jgabor/aila/internal/permission",
		"github.com/jgabor/aila/internal/runtime",
		"github.com/jgabor/aila/internal/state",
		"github.com/jgabor/aila/internal/tools",
		"github.com/jgabor/aila/internal/workflow",
	} {
		if imports[forbidden] {
			t.Fatalf("command boundary imports forbidden IO or ownership package %q", forbidden)
		}
	}

	source, err := os.ReadFile("command.go")
	if err != nil {
		t.Fatalf("read command.go: %v", err)
	}
	for _, forbidden := range []string{
		"Registry",
		"Register",
		"Args",
		"Shell",
		"Alias",
		"CLI",
		"Capability",
		"Workflow",
		"Plugin",
		"MCP",
		"exec.Command",
		"os.Read",
		"os.Write",
		"git ",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("command boundary contains deferred or IO marker %q", forbidden)
		}
	}
}

func TestCapabilitySlashRoutesRecommendCandidatesWithoutTransition(t *testing.T) {
	t.Parallel()

	checks := []struct {
		input string
		want  capability.Name
	}{
		{input: "/brief", want: capability.NameBrief},
		{input: "/vision", want: capability.NameVision},
		{input: "/discuss", want: capability.NameDiscuss},
		{input: "/research", want: capability.NameResearch},
		{input: "/plan", want: capability.NamePlan},
		{input: "/build", want: capability.NameBuild},
		{input: "/optimize", want: capability.NameOptimize},
		{input: "/document", want: capability.NameDocument},
		{input: "/design", want: capability.NameDesign},
		{input: "/audit", want: capability.NameAudit},
		{input: "/profile", want: capability.NameProfile},
		{input: "/orchestrate", want: capability.NameOrchestrate},
	}
	for _, check := range checks {
		check := check
		t.Run(check.input, func(t *testing.T) {
			t.Parallel()

			current := workflow.PhaseDeliberate
			got, ok := RecommendCapability(check.input, current)
			if !ok {
				t.Fatalf("RecommendCapability(%q) did not match", check.input)
			}
			if got.Source != CapabilityRouteExplicitSlash || got.Candidate != check.want || got.Confidence != 100 {
				t.Fatalf("RecommendCapability(%q) = %+v, want explicit %s confidence 100", check.input, got, check.want)
			}
			if got.CurrentPhase != current || got.TransitionClaimed || got.RecommendedSuccessor != "" {
				t.Fatalf("explicit route changed or claimed phase: %+v current=%s", got, current)
			}
			if len(got.SourceRefs) != 1 || got.SourceRefs[0].Kind != "prompt" || got.SourceRefs[0].Excerpt != check.input {
				t.Fatalf("explicit route source refs = %+v", got.SourceRefs)
			}
		})
	}

	for _, input := range []string{"/plan now", "/unknown", "/build --fast"} {
		if got, ok := RecommendCapability(input, workflow.PhasePlan); ok {
			t.Fatalf("RecommendCapability(%q) = %+v, want no route for deferred command syntax", input, got)
		}
	}
}

func TestCapabilityNaturalLanguageAndWaitingRecommendationsStayPure(t *testing.T) {
	t.Parallel()

	current := workflow.PhaseEnvision
	planned, ok := RecommendCapability("please plan the next slice with acceptance criteria", current)
	if !ok || planned.Source != CapabilityRouteNaturalLanguage || planned.Candidate != capability.NamePlan || planned.Confidence < 80 {
		t.Fatalf("planning recommendation = %+v ok=%v", planned, ok)
	}
	if planned.CurrentPhase != current || planned.TransitionClaimed || planned.RuntimeStatus != "" || planned.NeededInput != "" {
		t.Fatalf("planning recommendation mutated state or asked for input: %+v", planned)
	}

	waiting, ok := RecommendCapability("help", current)
	if !ok || waiting.Source != CapabilityRouteWaiting || waiting.RuntimeStatus != workflow.RuntimeStatusWaiting {
		t.Fatalf("waiting recommendation = %+v ok=%v", waiting, ok)
	}
	if waiting.Candidate != capability.NameBrief || waiting.Confidence >= 50 || waiting.NeededInput == "" {
		t.Fatalf("waiting route = %+v, want low-confidence brief candidate with needed input", waiting)
	}
	if waiting.CurrentPhase != current || waiting.TransitionClaimed || waiting.RecommendedSuccessor != "" {
		t.Fatalf("waiting recommendation changed or claimed phase: %+v", waiting)
	}
}

func TestCapabilitySuccessorRecommendationsRequireFSMValidation(t *testing.T) {
	t.Parallel()

	request := capability.Request{ID: "capability-route", Capability: capability.NameBuild, Phase: workflow.PhaseBuild}
	valid := RecommendCapabilitySuccessor(workflow.PhaseBuild, capability.ExitPayload{
		Capability:           capability.NameBuild,
		Signal:               capability.ExitComplete,
		RecommendedSuccessor: workflow.PhaseAudit,
		SourceRefs:           []capability.SourceRef{{ID: "build-exit", Kind: "capability_exit"}},
		BoundaryRequests:     []capability.BoundaryRequest{request.RequestArtifactAccess("build artifact", "state resolver boundary")},
	})
	if valid.Source != CapabilityRouteSuccessorCheck || !valid.SuccessorValid || valid.SuccessorRejected || valid.TransitionClaimed {
		t.Fatalf("valid successor recommendation = %+v", valid)
	}
	if valid.CurrentPhase != workflow.PhaseBuild || valid.RecommendedSuccessor != workflow.PhaseAudit || valid.SuccessorReason == "" {
		t.Fatalf("valid successor details = %+v", valid)
	}
	if len(valid.BoundaryRequests) != 1 || valid.BoundaryRequests[0].Kind != capability.BoundaryArtifactAccess {
		t.Fatalf("valid successor boundary descriptors = %+v", valid.BoundaryRequests)
	}

	invalid := RecommendCapabilitySuccessor(workflow.PhaseBuild, capability.ExitPayload{
		Capability:           capability.NameBuild,
		Signal:               capability.ExitFlagged,
		RecommendedSuccessor: workflow.PhaseDeliberate,
	})
	if !invalid.SuccessorRejected || invalid.SuccessorValid || invalid.TransitionClaimed {
		t.Fatalf("invalid successor recommendation = %+v", invalid)
	}
	if !strings.Contains(invalid.SuccessorReason, string(workflow.SuccessorValidationInvalidEdge)) {
		t.Fatalf("invalid successor reason = %q, want invalid edge", invalid.SuccessorReason)
	}
}

func TestCapabilityPolicyBoundaryStaysPureAndRecommendationOnly(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "capability.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse capability.go: %v", err)
	}
	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
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
		if imports[forbidden] {
			t.Fatalf("capability policy imports forbidden IO or ownership package %q", forbidden)
		}
	}

	source, err := os.ReadFile("capability.go")
	if err != nil {
		t.Fatalf("read capability.go: %v", err)
	}
	for _, forbidden := range []string{
		"exec.Command",
		"os.Read",
		"os.Write",
		"http.Get",
		"http.Post",
		"TransitionTo",
		"SetPhase",
		"RegisterCapability",
		"Plugin",
		"MCP",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("capability policy contains execution or deferred marker %q", forbidden)
		}
	}
}
