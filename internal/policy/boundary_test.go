package policy

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
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
		{name: "status", input: "/status", want: CommandRouteStatus},
		{name: "review", input: "/review", want: CommandRouteReview},
		{name: "help", input: "/help", want: CommandRouteHelp},
		{name: "history", input: "/history", want: CommandRouteHistory},
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
		{prefix: "ctrl+x", key: "status"},
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
