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

func TestM5CommandRoutesAreClosedPolicyRecommendations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  CommandRoute
	}{
		{name: "status", input: "/status", want: CommandRouteStatus},
		{name: "help", input: "/help", want: CommandRouteHelp},
		{name: "history", input: "/history", want: CommandRouteHistory},
		{name: "diff", input: "/diff", want: CommandRouteDiff},
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

func TestM5SlashAndShortcutRoutesShareRoute(t *testing.T) {
	t.Parallel()

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

func TestM5CommandBoundaryRejectsDeferredFamilies(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"/status now",
		"/help commands",
		"/quit --force",
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
		{prefix: "ctrl+x", key: "status"},
		{prefix: "ctrl+c", key: "q"},
		{prefix: "", key: "q"},
	} {
		if got, ok := RecommendShortcut(shortcut.prefix, shortcut.key); ok {
			t.Fatalf("RecommendShortcut(%q, %q) = %+v, want no route", shortcut.prefix, shortcut.key, got)
		}
	}
}

func TestM5CommandBoundaryStaysPureAndClosed(t *testing.T) {
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
