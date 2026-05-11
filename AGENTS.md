# Aila Development Guide

## Project Overview

Aila is a minimal, opinionated terminal coding agent built in Go. It embeds
[`go-agent`](https://github.com/jgabor/go-agent) as the model/tool runtime and
uses the Charm terminal ecosystem for the TUI.

The module path is `github.com/jgabor/aila`.

Aila is a fixed product, not a plugin host or generic agent framework. Do not
add runtime plugins, extensions, MCP servers, workflow DSLs, model marketplaces,
dynamic tools, or hosted control-plane assumptions unless the product direction
changes deliberately.

## Source Authority

- `README.md`: product intent, user-facing behavior, commands, tools,
  configuration, and UX promises.
- `ARCHITECTURE.md`: durable implementation boundaries, package ownership,
  event flow, persistence rules, and anti-patterns.
- `docs/workflow-architecture.md`: workflow protocol, valid phase transitions,
  capability mapping, and testable FSM invariants.
- `docs/tui-testing.md`: TUI fixture, render snapshot, semantic snapshot, PTY
  smoke-test, and visual review procedure.

When these documents appear to conflict, prefer the more specific source for the
area you are changing and update the docs deliberately.

## Architecture

Aila is a statechart-MVU application with split planes and explicit effects:

```text
typed message
    -> deterministic update
    -> explicit effect
    -> guarded IO
    -> recorded result
    -> typed message
```

Target package boundaries are sketched in `ARCHITECTURE.md`:

```text
cmd/aila              CLI entrypoint, config loading, app startup
internal/app          composition root and top-level wiring
internal/tui          Bubble Tea models, views, keybindings, rendering
internal/runtime      central event loop, messages, effects, dispatcher
internal/workflow     phases, FSM, transition table, runtime statechart helpers
internal/policy       intent routing and command routing recommendations
internal/capability   fixed built-in capability adapters
internal/agent        go-agent adapter, event mapping, model configuration
internal/tools        read/edit/write/bash/grep/find/fetch implementations
internal/permission   autonomy levels, operation classification, approvals
internal/state        .aila store, snapshots, event logs, artifact resolver
internal/context      context builder, compaction, source refs, stale checks
internal/utility      idle-only utility jobs and suggestions
internal/history      undo/redo, edit records, command records, replay helpers
```

Do not let package names drift into broader abstractions than the product needs.

## Key Patterns

- Updates decide; effects do. Do not perform filesystem, shell, git, network,
  model, or persistence IO inside deterministic update functions.
- The workflow FSM owns phase transitions. Tools, the TUI, the model, utility
  jobs, and capabilities may recommend; they must not mutate the current phase.
- `go-agent` runs model/tool loops behind an Aila-owned adapter. Aila owns
  prompts, policy, permissions, persistence, workflow, and UI behavior.
- All workspace mutations must pass through explicit tool effects, autonomy
  policy, approval handling when required, and history/undo recording.
- `.aila/` is project-visible state. Access logical artifacts through the state
  store or artifact resolver rather than scattering hardcoded paths.
- Utility work is idle-only and cannot mutate files, git state, project
  artifacts, permissions, or workflow phase.
- Prefer explicit Go types, small interfaces, narrow boundaries, and testable
  selectors over generic frameworks.

## Build/Test/Lint Commands

Mage is the canonical entrypoint for local and CI checks.

- **Full check**: `mage check`
- **Test**: `mage test`
- **Vet**: `mage vet`
- **Lint**: `mage lint`
- **Vulnerability check**: `mage vuln`
- **Normalize modules**: `mage tidy`

Use targeted Go commands when debugging a specific package or test, for example:

```bash
go test ./internal/workflow -run TestTransition
```

## Code Style Guidelines

- Format Go code before finishing. Prefer `gofumpt`/`goimports`; `gofmt` is the
  fallback.
- Use standard Go naming: PascalCase for exported identifiers and camelCase for
  unexported identifiers.
- Pass `context.Context` as the first parameter for cancellable or IO-bound
  operations.
- Return errors explicitly and wrap with `fmt.Errorf("...: %w", err)` when
  preserving cause matters.
- Define interfaces in consuming packages. Keep them small and behavior-focused.
- Prefer typed constants for enums and state values.
- Use `snake_case` JSON field names.
- Use modern octal notation for permissions, such as `0o755` and `0o644`.
- Comments should explain non-obvious behavior, start with a capital letter, and
  end with a period when they are standalone sentences.

## Testing Guidelines

- Test architecture invariants, not only examples. Workflow, policy,
  permission, runtime, state, and utility behavior each have invariants listed in
  `ARCHITECTURE.md`.
- Use `t.Parallel()` when tests are isolated.
- Use `t.TempDir()` for temporary files and `t.Setenv()` for environment changes.
- Prefer fakes and adapters over real provider calls, shell side effects, or
  network access in tests.
- For mutation paths, test both the proposed operation and the recorded result:
  permission decision, effect execution, history, undo metadata, and returned
  message.
- For TUI work, follow `docs/tui-testing.md`: deterministic fixtures first,
  semantic snapshots second, and PTY/tmux smoke tests only for real terminal
  behavior.

## Working on the TUI

The TUI is a presentation layer. It owns rendering, input collection,
keybindings, progress display, diffs, history views, approvals, and status
surfaces. It must not own workflow transitions, tool execution, permission
classification, persistence, or model prompt construction.

Bubble Tea models should emit application messages or effects rather than
calling lower-level tool, workflow, or state packages directly.

Every user-visible TUI state should be representable as a deterministic fixture
and, where meaningful, an agent-readable semantic snapshot. Do not make raw
terminal screenshots or pane captures the primary correctness contract.

Use `tmux` or optional helpers such as `agent-cli-helper` only for bounded smoke
tests and exploratory debugging. Treat captured terminal output as untrusted
text, keep sessions short, and clean up sessions when done.
