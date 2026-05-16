# Aila Roadmap

This roadmap turns Aila's product and architecture documents into small vertical
slices. It is not a date plan. It is the implementation sequence for growing
Aila from documentation and scaffolding into a terminal coding agent that can be
used to develop Aila itself.

The source documents remain authoritative for their domains:

- `README.md` defines product behavior and user promises.
- `ARCHITECTURE.md` defines package boundaries, event flow, effects, state, and
  anti-patterns.
- `docs/workflow-architecture.md` defines workflow protocol and FSM invariants.
- `docs/tui-testing.md` defines the TUI testing procedure.

## Roadmap Status

This section is the handoff point for the next developer or agent. Update it in
the same change that completes a milestone so nobody has to scan the full file to
find the next slice.

| Field                    | Value                                                                                                                                                                                                                                                                                                                                                          |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Last completed milestone | Milestone 22: Permission Model For Read-Only Work                                                                                                                                                                                                                                                                                                              |
| Next milestone           | Milestone 23: Agent Event Adapter With Fake Provider                                                                                                                                                                                                                                                                                                           |
| Active milestone         | none                                                                                                                                                                                                                                                                                                                                                           |
| Last updated             | 2026-05-16                                                                                                                                                                                                                                                                                                                                                     |
| Last validation          | M22 read-only autonomy decision-record tests, app/runtime decision metadata tests, blocked-read-decision render/semantic fixture snapshots plus existing autonomy-display fixture, PTY smoke skipped because no visible autonomy interaction was added, targeted tests, full `mage check`, architecture checklist evidence, and `git diff --check` all passed. |

Completion log:

| Milestone | Status    | Date       | Validation summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| --------- | --------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 0         | completed | 2026-05-11 | Source docs and TUI procedure exist; `mage check` passes baseline.                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| 1         | completed | 2026-05-12 | Go skeleton packages and idle-empty semantic fixture validated.                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| 2         | completed | 2026-05-12 | Static TUI shell renders and quits through PTY smoke.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| 3         | completed | 2026-05-12 | Fixture harness snapshots validated; PTY skipped, no terminal behavior changed.                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| 4         | completed | 2026-05-12 | Basic input loop validated with PTY submit smoke and full `mage check`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| 5         | completed | 2026-05-12 | Minimal command router validated with fixtures, parity tests, PTY smoke, and full `mage check`.                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| 6         | completed | 2026-05-12 | Resize/layout responsiveness validated with fixture matrix, PTY resize smoke, methodology hardening, approved human visual review, and full `mage check`.                                                                                                                                                                                                                                                                                                                                                                                  |
| 7         | completed | 2026-05-13 | CLI command shape validated with cmd/app targeted tests, TUI package tests, package no-IO guards, PTY startup/quit smoke, `go test ./...`, and `mage check`; human visual review skipped because no layout or hierarchy changed.                                                                                                                                                                                                                                                                                                           |
| 8         | completed | 2026-05-13 | Config display stub validated with app-owned display defaults/injection tests, TUI display fixtures and semantic snapshots, PTY startup label smoke, `go test ./...`, and `mage check`; human visual review skipped because fixed-size snapshots showed only label text/display-field changes and no layout, hierarchy, or narrow-screen regression.                                                                                                                                                                                       |
| 9         | completed | 2026-05-13 | Config creation and XDG paths validated with app/cmd config path, load, startup, and command tests; TUI display/semantic checks; temp-XDG PTY startup/config-creation smoke; `go test ./...`; `git diff --check`; post-closeout `agentera plan --format json`; and full `mage check`.                                                                                                                                                                                                                                                      |
| 9A        | completed | 2026-05-13 | Provider gateway and credential boundary validated with targeted app/agent/cmd/TUI tests, approved optional DeepSeek smoke against `https://api.deepseek.com/v1/chat/completions`, `go test ./...`, `git diff --check`, `mage check` with live smoke skipped by default, Agentera plan-empty closeout, boundary/secret inspection, and no push.                                                                                                                                                                                            |
| 10        | completed | 2026-05-13 | Workflow phase kernel validated with targeted workflow/app/TUI/cmd tests, deterministic fixture/snapshot evidence, bounded no-arg PTY startup smoke, `go test ./...`, Task 5 boundary/source inspection, `git diff --check`, post-closeout Agentera plan-empty check, full `mage check`, and no push.                                                                                                                                                                                                                                      |
| 11        | completed | 2026-05-13 | Workflow transition table validated with targeted workflow/app/TUI/cmd tests, `go test ./...`, Task 5 source-boundary inspection, `git diff --check`, post-closeout Agentera plan-empty check, full `mage check`, and no push.                                                                                                                                                                                                                                                                                                             |
| 12        | completed | 2026-05-13 | Runtime message loop validated with targeted runtime/app/TUI/cmd tests, deterministic runtime fixtures, bounded PTY prompt smoke through the runtime path, source-boundary inspection, `go test ./...`, `git diff --check`, post-closeout Agentera plan-empty check, full `mage check`, and no push.                                                                                                                                                                                                                                       |
| 13        | completed | 2026-05-15 | Queue visibility validated with queued-message fixture metadata and 80x24 plain/ANSI/semantic snapshots, runtime/app/TUI/keybinding regression tests, submit-while-active PTY smoke, Inspektera-evaluated task evidence, statechart-MVU/TUI-presentation-only checklist evidence, targeted runtime/app/TUI/cmd tests, full `mage check`, `git diff --check`, and no push.                                                                                                                                                                  |
| 14        | completed | 2026-05-15 | Interrupt message path validated with Ctrl-C and ctrl+x c keybinding tests, interrupt-canceling and interrupt-canceled fixtures and 80x24 plain/ANSI/semantic snapshots, live interrupt PTY smoke, M13 queue PTY smoke rerun, Inspektera Tasks 1-5 evidence, targeted runtime/app/TUI/cmd tests, full `mage check`, `git diff --check`, and no push.                                                                                                                                                                                       |
| 15        | completed | 2026-05-15 | Project store bootstrap validated with internal/state layout/resolver/open/write tests, app-owned startup status tests, store-initialized/store-uninitialized/store-degraded TUI fixtures and 80x24 plain/ANSI/semantic snapshots, startup store PTY smoke, Inspektera Tasks 1-5 evidence, targeted state/app/TUI/cmd tests, full `mage check`, `git diff --check`, and no push.                                                                                                                                                           |
| 15A       | completed | 2026-05-15 | Diagnostics and recovery boundary validated with typed diagnostic/recovery contract tests, corrupt/partial/versioned metadata recovery tests, bounded `--debug` output and redaction tests, runtime panic/cancellation and signal shutdown diagnostic tests, diagnostic-ready/corrupt-state-recovery/graceful-shutdown fixtures and 80x24 plain/ANSI/semantic snapshots, SIGTERM/debug smoke, Inspektera Tasks 1-7 evidence, targeted tests, full `mage check`, `git diff --check`, and no push.                                           |
| 16        | completed | 2026-05-16 | Session snapshot resume validated with internal/state current snapshot contract/read/write tests, app-owned explicit persistence and continue/resume startup tests, idle-with-memory 120x50 plain/ANSI/semantic fixture snapshots, bounded PTY resume/no-memory smoke for `continue` and `--continue`, CLI `-c` runner coverage, Inspektera Tasks 1-6 evidence, targeted state/app/TUI/cmd tests, full `mage check`, architecture checklist evidence, `git diff --check`, and no push.                                                     |
| 17        | completed | 2026-05-16 | History view for fake events validated with internal/history event contract tests, internal/state fake history read/append/recovery tests, app-owned explicit history recording tests, `/history` and `ctrl+x h` route/navigation/keybinding tests, history-view 100x30 plain/ANSI/semantic fixture snapshots, bounded PTY history and empty-history smoke, Inspektera Tasks 1-6 evidence, targeted history/state/app/policy/TUI/cmd tests, full `mage check`, architecture checklist evidence, `git diff --check`, and no push.           |
| 18        | completed | 2026-05-16 | Read tool validated with internal/tools contract/executor tests, app/runtime read-effect and read-only permission tests, TUI read rendering and semantic tests, read-result and tool-running plain/ANSI/semantic fixture snapshots, explicit PTY smoke skip because no visible read command path exists, targeted internal tests, full `mage check`, architecture checklist evidence, `git diff --check`, and no push.                                                                                                                     |
| 19        | completed | 2026-05-16 | Find and grep tools validated with internal/tools search contract/executor/bounds/no-mutation tests, runtime/app search proposal/effect/result tests, permission read-only classification tests, file-search/content-search/search-running plain/ANSI/semantic fixture snapshots, explicit PTY smoke skip because no visible search command path exists, targeted internal tests, full `mage check`, architecture checklist evidence, `git diff --check`, and no push.                                                                     |
| 20        | completed | 2026-05-16 | Safe bash inspection validated with internal/tools bash contract/executor/bounds/no-mutation tests, runtime/app bash proposal/effect/result tests, permission read-only metadata tests, command-result/command-failure/command-tool-running/tool-failed plain/ANSI/semantic fixture snapshots, explicit PTY smoke skip because no visible shell command path exists, targeted internal tests, full `mage check`, architecture checklist evidence, `git diff --check`, and new-file whitespace checks.                                      |
| 21        | completed | 2026-05-16 | Fetch tool validated with internal/tools fetch contract/fake-client executor/error/bounds tests, runtime/app fetch proposal/effect/result tests, permission read-only network metadata tests, fetch-success/fetch-failure/fetch-tool-running plain/ANSI/semantic fixture snapshots, explicit PTY smoke skip because no visible fetch command path exists, targeted internal tests, full `mage check`, architecture checklist evidence, `git diff --check`, and new-file whitespace checks.                                                 |
| 22        | completed | 2026-05-16 | Permission model for read-only work validated with internal/permission decision-record tests, runtime read/search/bash/fetch result metadata retention tests, app allowed/denied/no-fabricated-decision dispatch tests, blocked-read-decision plain/ANSI/semantic fixture snapshots plus existing autonomy-display fixture, explicit PTY smoke skip because no visible autonomy interaction was added, targeted internal tests, full `mage check`, architecture checklist evidence, `git diff --check`, and new-fixture whitespace checks. |

Status values:

- `next`: the next milestone to implement.
- `active`: implementation has started but is not validated.
- `completed`: exit condition and TUI validation gate passed.
- `blocked`: implementation cannot continue; record the blocker and the last
  passing validation.
- `skipped`: milestone was deliberately bypassed; record the reason and the
  replacement milestone.

Milestone completion protocol:

1. Satisfy the milestone exit condition.
2. Run the milestone's TUI validation gate at the current product state.
3. Run `mage check`.
4. Update `Last completed milestone`, `Next milestone`, `Active milestone`,
   `Last updated`, and `Last validation` above.
5. Add one row to the completion log with the milestone number, status, date, and
   validation summary.
6. Do not mark a milestone `completed` when PTY smoke, semantic snapshots, render
   snapshots, update tests, or `mage check` were skipped without a documented
   reason.

## Slice Completion Template

Use this template when opening or finishing a roadmap slice. Keep the completed
version in the commit, PR, or implementation handoff notes.

```text
Milestone:
Scope:
Primary moving part:
Source docs checked:
TUI fixtures added or updated:
Semantic snapshots added or updated:
Render snapshots added or updated:
Update/keybinding tests added or updated:
PTY smoke scenarios run or skipped with reason:
Agentic validation performed:
Human visual review performed or skipped with reason:
Architecture checklist result:
Verification command:
Roadmap status updated:
```

Completed Milestone 13 Slice Completion Template:

```text
Milestone: Milestone 13: Queue Visibility
Scope: Show queued user input while fake work is active; no Milestone 14 interrupt behavior.
Primary moving part: Runtime/app-owned queued intent exposed through TUI rendering and semantic state.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: queued-message fixture.
Semantic snapshots added or updated: queued-message semantic snapshot with queued count and default queue action.
Render snapshots added or updated: queued-message 80x24 plain/ANSI render snapshots.
Update/keybinding tests added or updated: runtime/app/TUI/keybinding regression tests for queued input visibility and stable existing behavior.
PTY smoke scenarios run or skipped with reason: submit-while-active PTY smoke run through cmd/aila.
Agentic validation performed: Inspektera Task 1-5 evidence recorded; Task 6 audit re-ran targeted tests, mage check, and git diff --check.
Human visual review performed or skipped with reason: Skipped; evidence is deterministic fixture/render/semantic snapshot and PTY smoke for queue visibility.
Architecture checklist result: Statechart-MVU and TUI presentation-only boundary evidence recorded; no provider/tool/persistence/plugin/MCP/M14 behavior added by Task 6.
Verification command: go test ./internal/runtime ./internal/app ./internal/tui ./cmd/aila -count=1; mage check; git diff --check.
Roadmap status updated: M13 complete, M14 next, active milestone none.
```

Completed Milestone 14 Slice Completion Template:

```text
Milestone: Milestone 14: Interrupt Message Path
Scope: Add a visible typed interrupt path for fake active work; no real model/shell/tool/provider cancellation or Milestone 15 persistence behavior.
Primary moving part: App/runtime-owned interrupt request and fake canceling/canceled state surfaced through presentation-only TUI keybindings, rendering, semantic snapshots, fixtures, and PTY smoke.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: interrupt-canceling and interrupt-canceled fixtures.
Semantic snapshots added or updated: interrupt-canceling and interrupt-canceled semantic snapshots with machine-readable interrupt state and lower_layer_cancellation_executed=false.
Render snapshots added or updated: interrupt-canceling and interrupt-canceled 80x24 plain/ANSI render snapshots.
Update/keybinding tests added or updated: runtime interrupt tests, app interrupt handoff tests, TUI Ctrl-C and ctrl+x c keybinding tests, interrupt render/semantic tests, and M13 queue stability tests.
PTY smoke scenarios run or skipped with reason: live interrupt PTY smoke run through TestM14InterruptActiveWorkPTYSmoke; M13 submit-while-active queue smoke rerun to prove ordinary active-window text still queues by default.
Agentic validation performed: Inspektera Tasks 1-5 passed; Task 6 final freshness will be evaluated after this update.
Human visual review performed or skipped with reason: skipped; deterministic render/semantic snapshots and PTY smoke cover the visible interrupt states.
Architecture checklist result: passed; interrupt decisions remain app/runtime-owned, TUI emits messages/renders injected state only, and no real IO cancellation, provider/tool/shell cancellation, permission, persistence, plugin, MCP, or M15 behavior was added.
Verification command: go test ./internal/runtime ./internal/app ./internal/tui ./cmd/aila -count=1; mage check; git diff --check.
Roadmap status updated: M14 complete, M15 next, active milestone none.
```

Completed Milestone 15 Slice Completion Template:

```text
Milestone: Milestone 15: Project Store Layout Bootstrap
Scope: Create `.aila/` project state through a narrow internal/state boundary; no sessions, replay, context compaction, diagnostics/recovery, permissions, history/undo, plugins, MCP, or Milestone 16+ behavior.
Primary moving part: internal/state-owned project store layout, resolver, initialization, and owned artifact writes surfaced through app-owned path-safe TUI status data.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: store-initialized, store-uninitialized, and store-degraded fixtures; existing queued-message and interrupt fixtures rerun for stability.
Semantic snapshots added or updated: store-initialized, store-uninitialized, and store-degraded semantic snapshots with project_store region and session project_store fields.
Render snapshots added or updated: store-initialized, store-uninitialized, and store-degraded 80x24 plain/ANSI render snapshots with path-safe actionable store status detail.
Update/keybinding tests added or updated: internal/state store tests, app startup/status tests, TUI render/semantic fixture tests, cmd PTY workspace-isolation and startup store smoke tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: TestM15ProjectStoreStartupPTYSmoke run through cmd/aila; existing prompt submit, status/help/quit, resize, queue, and interrupt PTY regressions rerun through targeted tests and mage check.
Agentic validation performed: Inspektera Tasks 1-5 passed; Task 6 final validation ran targeted state/app/TUI/cmd tests, mage check, and git diff --check.
Human visual review performed or skipped with reason: skipped; deterministic 80x24 plain/ANSI render snapshots, semantic snapshots, and PTY smoke cover the visible store status states without layout/hierarchy ambiguity.
Architecture checklist result: passed; store layout and writes remain in internal/state, app owns startup/status conversion, TUI consumes strings/semantic fields only, and no session/replay/context-compaction/diagnostic/provider/tool/permission/history/plugin/MCP behavior was added.
Verification command: go test ./internal/state ./internal/app ./internal/tui ./cmd/aila -count=1; mage check; git diff --check.
Roadmap status updated: M15 complete, M15A next, active milestone none.
```

Completed Milestone 15A Slice Completion Template:

```text
Milestone: Milestone 15A: Diagnostics And State Recovery Boundary
Scope: Make startup, runtime, state, debug, and signal failures inspectable with typed bounded diagnostics and safe recovery guidance; no session resume, event replay, context compaction, destructive recovery execution, provider fallback, permission policy changes, undo, plugins, MCP, or Milestone 16+ behavior.
Primary moving part: Typed internal/diagnostic records carried through state opening, app startup/status/debug output, runtime failure/cancellation messages, signal-triggered shutdown, deterministic TUI diagnostics, and bounded terminal smoke tests.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: diagnostic-ready, corrupt-state-recovery, and graceful-shutdown fixtures; queue, interrupt, and store-status fixtures rerun for stability.
Semantic snapshots added or updated: diagnostic-ready, corrupt-state-recovery, and graceful-shutdown semantic snapshots with severity, source, recovery action, affected artifact, user-input-needed, and bounded message fields.
Render snapshots added or updated: diagnostic-ready, corrupt-state-recovery, and graceful-shutdown 80x24 plain/ANSI render snapshots with path-safe actionable diagnostic/recovery/shutdown status.
Update/keybinding tests added or updated: internal/diagnostic contract/redaction tests, internal/state corrupt metadata recovery tests, app startup/debug/shutdown tests, runtime panic/cancellation diagnostic tests, TUI diagnostic render/semantic fixture tests, and cmd SIGTERM/debug smoke tests; no new user keybinding was added.
PTY smoke scenarios run or skipped with reason: TestM15AShutdownDiagnosticsPTYSmoke sent SIGTERM through cmd/aila and verified clean diagnostic shutdown; TestM15ADebugDiagnosticsSmoke verified bounded structured redacted non-interactive debug output. Existing prompt submit, status/help/quit, queue, interrupt, and store startup PTY regressions rerun through targeted tests and mage check.
Agentic validation performed: Inspektera Tasks 1-7 passed, including retries for metadata propagation, redaction coverage, degraded shutdown diagnostics, and complete PTY output/no-mutation evidence; Task 8 final validation ran targeted state/runtime/app/TUI/cmd tests, mage check, and git diff --check.
Human visual review performed or skipped with reason: skipped; deterministic 80x24 plain/ANSI render snapshots, semantic snapshots, and bounded PTY/debug smoke cover the visible diagnostic, recovery, and shutdown states without layout/hierarchy ambiguity.
Architecture checklist result: passed; diagnostics are typed product state, state owns metadata validation, app/runtime own failure and shutdown message conversion, TUI consumes injected display/semantic fields only, and no session/replay/context-compaction/destructive-repair/provider-fallback/permission-policy/undo/plugin/MCP behavior was added.
Verification command: go test ./internal/state ./internal/runtime ./internal/app ./internal/tui ./cmd/aila -count=1; mage check; git diff --check.
Roadmap status updated: M15A complete, M16 next, active milestone none.
```

Completed Milestone 16 Slice Completion Template:

```text
Milestone: Milestone 16: Session Snapshot Resume
Scope: Save and resume the current fake session from `.aila/sessions/current.json`; no event replay, session index/history browsing, context compaction, stale checks, real provider/tool history, approval persistence, mutation history, undo, provider fallback, permission policy changes, plugins, MCP, workflow DSL behavior, or Milestone 17+ behavior.
Primary moving part: State-owned current session snapshot contract/read/write APIs plus app-owned explicit snapshot persistence and continue startup wiring that inject resumed fake memory into presentation-only TUI render and semantic state.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: idle-with-memory fixture; queue, interrupt, store-status, and diagnostic fixtures rerun for stability.
Semantic snapshots added or updated: idle-with-memory semantic snapshot with memory source, session id, resumed transcript turns, queued count, blockers, concerns, diagnostics, runtime/status fields, and bounded redacted path-safe text.
Render snapshots added or updated: idle-with-memory 120x50 plain/ANSI render snapshots showing resumed chat, queued input, diagnostics, blockers, concerns, and status context without hardcoded TUI filesystem state.
Update/keybinding tests added or updated: internal/state session snapshot contract/read/write/symlink/recovery tests, internal/app explicit persistence and continue startup tests, internal/tui injected-memory render/semantic/sanitizer and boundary tests, cmd continue alias/flag runner tests, and cmd PTY resume/no-memory smoke tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: TestM16ContinueResumePTYSmoke covered `aila continue` and `aila --continue` from a seeded temp workspace snapshot; TestM16ContinueNoMemoryPTYSmoke covered missing-memory startup for both shapes; `-c` was covered by CLI runner tests rather than PTY because the resume behavior is the same app-owned startup path.
Agentic validation performed: Inspektera Tasks 1-6 passed, including fixes for traversal validation, symlinked session directory escape, runtime/status sanitization, sanitizer ordering, and PTY output/no-mutation evidence; Task 7 freshness ran targeted state/app/TUI/cmd tests, full `mage check`, and `git diff --check` without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic 120x50 plain/ANSI render snapshots, semantic snapshots, and bounded PTY resume smoke cover the visible resumed-memory state without requiring exploratory visual judgment.
Architecture checklist result: passed; snapshot filesystem IO stays in internal/state behind app-owned commands/results, deterministic runtime/TUI updates remain filesystem-free, TUI consumes injected presentation fields only, recovery remains diagnostic/non-destructive, and no event replay, session history/index, context compaction, real provider/tool history, permission-policy, undo, plugin, MCP, workflow DSL, or Milestone 17+ behavior was added.
Verification command: go test ./internal/state ./internal/app ./internal/tui ./cmd/aila -count=1; mage check; git diff --check.
Roadmap status updated: M16 complete, M17 next, active milestone none; no commit or push performed.
```

Completed Milestone 17 Slice Completion Template:

```text
Milestone: Milestone 17: History View For Fake Events
Scope: Record and inspect fake prompt, response, command, and runtime events through `.aila/history/fake-events.jsonl`; no undo/redo, mutation records, approval persistence, real model/tool/provider history, shell command history, read/find/grep/bash tools, event replay, session restore/selection, context compaction, stale checks, provider fallback, permission policy changes, plugins, MCP, workflow DSL behavior, or Milestone 18+ behavior.
Primary moving part: internal/history fake event contract plus internal/state fake history append/read APIs, app-owned explicit recording/read commands, and injected read-only TUI history view state.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: history-view fixture; queue, interrupt, idle-with-memory, store-status, diagnostic, status/help, and no-history evidence rerun for stability.
Semantic snapshots added or updated: history-view semantic snapshot with stable fake run/session/event IDs, event kinds, selected history item, focus state, read_only=true, undo_enabled=false, and bounded redacted path-safe text.
Render snapshots added or updated: history-view 100x30 plain/ANSI render snapshots showing prompt/response/command/runtime summaries, selected-item details, read-only state, and sanitized adversarial text.
Update/keybinding tests added or updated: internal/history event contract and sanitizer tests, internal/state fake history read/append/recovery/path-safety tests, internal/app explicit history persistence/read command tests, internal/policy `/history` and `ctrl+x h` parity tests, internal/tui history navigation/selection/focus/empty/bounded-list tests, TUI boundary tests, and cmd PTY history smoke tests.
PTY smoke scenarios run or skipped with reason: TestM17HistoryViewPTYSmoke opened seeded fake history through `/history`; TestM17HistoryEmptyPTYSmoke opened empty history through `ctrl+x h`; both used bounded PTY sessions, isolated temp workspace/config state, leak checks, cleanup assertions, and no undo/replay assertions.
Agentic validation performed: Inspektera Tasks 1-6 passed, including retry fixes for C1 terminal-control stripping and append atomicity; Task 7 freshness ran targeted history/state/app/policy/TUI/cmd tests, full `mage check`, and `git diff --check` without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic 100x30 plain/ANSI render snapshots, semantic snapshots, update/keybinding tests, and bounded PTY smoke cover the visible read-only history state without requiring exploratory visual judgment.
Architecture checklist result: passed; history event shaping stays in internal/history, durable history IO stays in internal/state, app owns explicit recording/read commands and passive diagnostics, deterministic runtime updates and TUI rendering remain filesystem-free, TUI consumes injected display/semantic fields only, workflow phase is not mutated, and no undo/replay/mutation/real-tool-history/read-tool/plugin/MCP/M18+ behavior was added.
Verification command: go test ./internal/history ./internal/state ./internal/app ./internal/policy ./internal/tui ./cmd/aila -count=1; go test ./cmd/aila -run 'TestM17History(View|Empty)PTYSmoke' -count=1; mage check; git diff --check -- ROADMAP.md .agentera/progress.yaml internal/app/fake_history.go internal/tui/render.go internal/history internal/state internal/policy internal/app internal/tui cmd/aila.
Roadmap status updated: M17 complete, M18 next, active milestone none; no commit or push performed.
```

Completed Milestone 18 Slice Completion Template:

```text
Milestone: Milestone 18: Read Tool
Scope: Add the first read-only workspace file read tool through explicit runtime/app effects; no find, grep, bash, fetch, edit/write, mutation history, undo/replay, context compaction, provider fallback, plugins, MCP, workflow DSL, marketplace behavior, or Milestone 19+ behavior.
Primary moving part: internal/tools read contract and executor, runtime read proposal/effect/result messages, app-owned read effect dispatch, minimal read-only permission classification, and injected TUI read result presentation.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: read-result fixture and canonical tool-running fixture.
Semantic snapshots added or updated: read-result semantic snapshot with read_tool path, requested/effective ranges, preview lines, truncation marker, completed state, and read_only=true; tool-running semantic snapshot with read_tool running status, target path, requested range, completed=false, and no completed result fields.
Render snapshots added or updated: read-result 120x34 plain/ANSI snapshots showing internal/tui/render.go lines 18-19, exact requested/effective line ranges, truncation metadata, and read-only status; tool-running 100x30 plain/ANSI snapshots showing the running read target/range and completed=false.
Update/keybinding tests added or updated: internal/tools read validation/execution/bounds/no-mutation tests, internal/runtime read proposal/effect/result/queue/interrupt tests, internal/app read dispatch and TUI mapping tests, internal/permission read-only classification tests, internal/tui read render/semantic/redaction/fixture tests, and PTY-skip command-input regression tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: skipped by roadmap gate because M18 exposes no visible interactive read command path; TestM18ReadPTYSmokeDecision proves `/read`, `/read internal/tui/render.go`, and read-like prompt input do not invoke visible read state.
Agentic validation performed: Orkestrera Tasks 1-6 evidence recorded; independent reviews found and closed active-read fake interrupt handling, read render redaction gaps, fixture line-reference drift, canonical tool-running naming, and new-file whitespace coverage; final validation ran targeted tests, full `mage check`, Agentera plan/progress validation through the installed Agentera CLI, tracked-file `git diff --check`, and new-file whitespace checks without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic read-result and tool-running plain/ANSI render snapshots plus semantic snapshots cover the visible read states without requiring exploratory visual judgment.
Architecture checklist result: passed; read validation/execution stays in internal/tools behind app-owned effects, deterministic runtime updates remain filesystem-free, app dispatch owns guarded IO and read-only permission classification, TUI consumes injected presentation fields only, workflow phase is not mutated, read failures are bounded/non-destructive, and no mutation/search/shell/network/plugin/MCP/M19+ behavior was added.
Verification command: go test ./internal/tools ./internal/runtime ./internal/app ./internal/permission ./internal/tui -count=1; go test ./internal/... -count=1; mage check; git diff --check -- ROADMAP.md .agentera/plan.yaml .agentera/progress.yaml internal/tools internal/runtime internal/app internal/permission internal/tui; git diff --check --no-index /dev/null <each new permission/read fixture file>; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera plan --format json; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera progress --format json.
Roadmap status updated: M18 complete, M19 next, active milestone none; no commit or push performed.
```

Completed Milestone 19 Slice Completion Template:

```text
Milestone: Milestone 19: Find And Grep Tools
Scope: Add read-only workspace file discovery and content search through explicit runtime/app effects; no bash, fetch, edit/write, mutation history, undo/replay, context compaction, stale checks, source indexing, provider fallback, plugins, MCP, workflow DSL, marketplace behavior, or Milestone 20+ behavior.
Primary moving part: internal/tools find/grep contracts and executors, runtime search proposal/effect/result messages, app-owned search effect dispatch, read-only permission classification, and injected TUI search result presentation.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: file-search-result, content-search-result, and search-tool-running fixtures; M18 read-result and tool-running fixture tests rerun for stability.
Semantic snapshots added or updated: file-search-result semantic snapshot with find paths, omitted-result metadata, completed state, and read_only=true; content-search-result semantic snapshot with grep query/include filter, paths, 1-based line numbers, bounded previews, omitted file/result metadata, truncation marker, and read_only=true; search-tool-running semantic snapshot with running grep query/include filter and completed=false.
Render snapshots added or updated: file-search-result 120x34 plain/ANSI snapshots, content-search-result 120x34 plain/ANSI snapshots, and search-tool-running 100x30 plain/ANSI snapshots showing safe workspace paths, line references for grep, omitted-result markers, truncation metadata, and read-only status.
Update/keybinding tests added or updated: internal/tools find/grep validation/execution/bounds/no-mutation tests, internal/runtime search proposal/effect/result/queue tests, internal/app search dispatch and TUI mapping tests, internal/permission read-only classification tests, internal/tui search render/semantic/redaction/fixture tests, and PTY-skip command-input regression tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: skipped by roadmap gate because M19 exposes no visible interactive search command path; TestM19SearchPTYSmokeDecision proves `/find`, `/grep TODO`, `find internal/**/*.go`, and `grep TODO internal/**/*.go` do not invoke visible search state.
Agentic validation performed: Orkestrera Tasks 1-6 completed; final validation ran targeted tests, full internal tests, full `mage check` with longer timeout after an initial timeout, tracked-file `git diff --check`, and new-file whitespace checks without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic file-search, content-search, and search-running plain/ANSI render snapshots plus semantic snapshots cover the visible search states without requiring exploratory visual judgment.
Architecture checklist result: passed; find/grep validation and execution stay in internal/tools behind app-owned effects, deterministic runtime updates remain filesystem-free, app dispatch owns guarded IO and read-only permission classification, TUI consumes injected presentation fields only, workflow phase is not mutated, failures are bounded/non-destructive, and no shell/network/mutation/indexing/plugin/MCP/M20+ behavior was added.
Verification command: go test ./internal/tools ./internal/runtime ./internal/app ./internal/permission ./internal/tui -count=1; go test ./internal/... -count=1; mage check; git diff --check -- ROADMAP.md .agentera/plan.yaml .agentera/progress.yaml internal/tools internal/runtime internal/app internal/permission internal/tui; git diff --check --no-index /dev/null <each new search/tool fixture file>.
Roadmap status updated: M19 complete, M20 next, active milestone none; no commit or push performed.
```

Completed Milestone 20 Slice Completion Template:

```text
Milestone: Milestone 20: Safe Bash Inspection
Scope: Add narrowly classified read-only shell inspection through explicit runtime/app effects; no arbitrary shell, fetch, edit/write, mutation history, approval persistence, undo/replay, context compaction, stale checks, source indexing, provider fallback, plugins, MCP, workflow DSL, marketplace behavior, or Milestone 21+ behavior.
Primary moving part: internal/tools bash inspection contract and argv executor, runtime bash proposal/effect/result messages, app-owned bash effect dispatch, read-only permission metadata, and injected TUI command result/failure presentation.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: command-result, command-failure, command-tool-running, and canonical tool-failed fixtures.
Semantic snapshots added or updated: command-result semantic snapshot with bash_tool argv, working_dir, command_family, expected_effect, exit_code, stdout, truncation, completed state, and read_only=true; command-failure and tool-failed semantic snapshots with failure status, stderr, truncation, error metadata, and redaction; command-tool-running semantic snapshot with running command metadata and completed=false.
Render snapshots added or updated: command-result and command-failure 120x34 plain/ANSI snapshots plus command-tool-running 100x30 plain/ANSI snapshots and tool-failed 120x34 plain/ANSI snapshots showing safe command argv, status, exit code, output/error lines, truncation metadata, and read-only status.
Update/keybinding tests added or updated: internal/tools bash validation/execution/bounds/no-mutation tests, internal/runtime bash proposal/effect/result/queue tests, internal/app bash dispatch and TUI mapping tests, internal/permission read-only bash metadata tests, internal/tui command render/semantic/redaction/fixture tests, and PTY-skip command-input regression tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: skipped by roadmap gate because M20 exposes no visible interactive shell prefix or slash command path; TestM20BashPTYSmokeDecision proves `/bash pwd`, `! git status`, `bash git status`, and `git status --short` do not invoke visible bash state.
Agentic validation performed: Orkestrera Tasks 1-5 completed; final validation ran targeted tests, full internal tests, full `mage check`, tracked-file `git diff --check`, and new-file whitespace checks without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic command-result, command-failure, command-running, and tool-failed plain/ANSI render snapshots plus semantic snapshots cover the visible command states without requiring exploratory visual judgment.
Architecture checklist result: passed; safe command validation/execution stays in internal/tools behind app-owned effects, deterministic runtime updates remain command-execution-free, app dispatch owns guarded IO and read-only permission classification, TUI consumes injected presentation fields only, workflow phase is not mutated, failures are bounded/non-destructive, git parent discovery is constrained to the declared workspace, and no arbitrary shell/network/mutation/approval-persistence/plugin/MCP/M21+ behavior was added.
Verification command: go test ./internal/tools ./internal/runtime ./internal/app ./internal/permission ./internal/tui -count=1; go test ./internal/... -count=1; mage check; git diff --check -- ROADMAP.md .agentera/plan.yaml .agentera/progress.yaml internal/tools internal/runtime internal/app internal/permission internal/tui; git diff --check --no-index /dev/null <each new bash/command fixture file>; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera plan --format json; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera progress --format json.
Roadmap status updated: M20 complete, M21 next, active milestone none.
```

Completed Milestone 21 Slice Completion Template:

```text
Milestone: Milestone 21: Fetch Tool
Scope: Add a tested read-only network fetch boundary through explicit runtime/app effects; no provider/model networking coupling, edit/write, mutation history, approval persistence, undo/replay, context compaction, stale checks, source indexing, provider fallback, plugins, MCP, workflow DSL, marketplace behavior, or Milestone 22+ behavior.
Primary moving part: internal/tools fetch contract and fakeable HTTP executor, runtime fetch proposal/effect/result messages, app-owned fetch effect dispatch with injectable client, read-only permission metadata, and injected TUI fetch success/failure presentation.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: fetch-success, fetch-failure, and fetch-tool-running fixtures.
Semantic snapshots added or updated: fetch-success semantic snapshot with fetch_tool URL, method, result status, HTTP status, content type, preview lines, omitted-byte marker, truncation, completed state, and read_only=true; fetch-failure semantic snapshot with http_error status, HTTP status, bounded preview, error metadata, omitted content metadata, redaction, and read_only=true; fetch-tool-running semantic snapshot with URL, method, running status, completed=false, and read_only=true.
Render snapshots added or updated: fetch-success, fetch-failure, and fetch-tool-running 120x44 plain/ANSI snapshots showing URL, status, HTTP metadata, preview/error lines, omitted content markers, truncation metadata, and read-only status.
Update/keybinding tests added or updated: internal/tools fetch validation/execution/fake-client/error/bounds tests, internal/runtime fetch proposal/effect/result/queue tests, internal/app fetch dispatch and TUI mapping tests with injectable fake client, internal/permission read-only fetch metadata tests, internal/tui fetch render/semantic/redaction/fixture tests, and PTY-skip command-input regression tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: skipped by roadmap gate because M21 exposes no visible interactive fetch prefix or slash command path; TestM21FetchPTYSmokeDecision proves `/fetch https://example.com`, `fetch https://example.com`, and `curl https://example.com` do not invoke visible fetch state.
Agentic validation performed: Orkestrera Tasks 1-5 completed; final validation ran targeted tests, full internal tests, full `mage check`, tracked-file `git diff --check`, and new-file whitespace checks without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic fetch-success, fetch-failure, and fetch-running plain/ANSI render snapshots plus semantic snapshots cover the visible fetch states without requiring exploratory visual judgment.
Architecture checklist result: passed; fetch validation/execution stays in internal/tools behind app-owned effects, deterministic runtime updates remain network-free, app dispatch owns guarded IO, injectable network client, and read-only permission classification, TUI consumes injected presentation fields only, workflow phase is not mutated, failures are bounded/non-destructive, tests do not require live network, provider/model networking remains separate, and no mutation/indexing/plugin/MCP/M22+ behavior was added.
Verification command: go test ./internal/tools ./internal/runtime ./internal/app ./internal/permission ./internal/tui -count=1; go test ./internal/... -count=1; mage check; git diff --check -- ROADMAP.md .agentera/plan.yaml .agentera/progress.yaml internal/tools internal/runtime internal/app internal/permission internal/tui; git diff --check --no-index /dev/null <each new fetch/tool fixture file>; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera plan --format json; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera progress --format json.
Roadmap status updated: M21 complete, M22 next, active milestone none.
```

Completed Milestone 22 Slice Completion Template:

```text
Milestone: Milestone 22: Permission Model For Read-Only Work
Scope: Record and expose read-only autonomy decisions for existing read, find, grep, safe bash inspection, and fetch paths; no approval prompts, write classes, edit/write tools, mutation history, approval persistence, undo/replay, context compaction, stale checks, source indexing, provider fallback, plugins, MCP, workflow DSL, marketplace behavior, or Milestone 23+ behavior.
Primary moving part: internal/permission DecisionRecord contract, runtime ToolDecision result metadata, app-owned decision mapping for read/search/bash/fetch effects, and injected TUI decision presentation/semantic snapshots.
Source docs checked: AGENTS.md, ROADMAP.md, docs/notes/PROMPT.md, ARCHITECTURE.md, docs/workflow-architecture.md, docs/tui-testing.md, README.md, .agentera/plan.yaml, .agentera/progress.yaml.
TUI fixtures added or updated: blocked-read-decision fixture added; existing autonomy-display fixture retained for current-autonomy display evidence.
Semantic snapshots added or updated: blocked-read-decision semantic snapshots expose session autonomy=off, read_tool failed permission_denied state, decision source, denied allow state, automatic=false, approval_required=true, operation_kind=read, tool, target, expected_effect, reversible, and reason.
Render snapshots added or updated: blocked-read-decision 80x44, 100x44, 120x44, and 160x45 plain/ANSI snapshots show failed read-only status, current autonomy, decision source, denied state, approval-required state, operation metadata, target, expected effect, reversibility, and reason.
Update/keybinding tests added or updated: internal/permission decision-record tests, internal/runtime read/search/bash/fetch decision-retention tests, internal/app allowed/denied/no-fabricated-decision dispatch and TUI mapping tests, internal/tui blocked-decision render/semantic/redaction/fixture tests, and PTY-skip autonomy-input regression tests; no new keybinding behavior was added.
PTY smoke scenarios run or skipped with reason: skipped by roadmap gate because M22 adds no visible autonomy interaction, approval prompt, or command path; TestM22DecisionPTYSmokeDecision proves `/autonomy off`, `autonomy read`, and `approve read` do not invoke visible decision/tool state.
Agentic validation performed: Orkestrera Tasks 1-4 completed; final validation ran targeted tests, full internal tests, full `mage check`, tracked-file `git diff --check`, and new-fixture whitespace checks without bypassing gates.
Human visual review performed or skipped with reason: skipped; deterministic blocked-read-decision plain/ANSI render snapshots plus semantic snapshots cover the visible blocked decision state without requiring exploratory visual judgment.
Architecture checklist result: passed; permission policy stays in internal/permission, deterministic runtime updates only retain typed metadata, app dispatch owns guarded IO and decision mapping, validation failures before a decision do not fabricate decision metadata, TUI consumes injected display fields only, workflow phase is not mutated, and no approval/write/mutation/provider/plugin/MCP/M23+ behavior was added.
Verification command: go test ./internal/permission ./internal/runtime ./internal/app ./internal/tui -count=1; go test ./internal/... -count=1; mage check; git diff --check -- ROADMAP.md .agentera/plan.yaml .agentera/progress.yaml internal/permission internal/runtime internal/app internal/tui; git diff --check --no-index /dev/null <each new blocked-read-decision fixture file>; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera plan --format json; uv run /home/jgabor/.local/share/agentera/app/scripts/agentera progress --format json.
Roadmap status updated: M22 complete, M23 next, active milestone none.
```

If a slice cannot satisfy the TUI validation gate, either the slice is too large,
the user-visible state is too implicit, or the test harness must be extended
first.

## Roadmap Rules

- Build vertical slices, not isolated layers. Each milestone should leave Aila
  more usable or more testable than before.
- Keep each milestone small. A milestone should introduce one primary moving
  part, plus only the supporting test and UI surfaces needed to validate it.
- Preserve the statechart-MVU loop: typed message, deterministic update,
  explicit effect, guarded IO, recorded result, typed message.
- Keep Aila fixed-product. Do not add plugins, extensions, MCP servers,
  user-defined workflow modules, dynamic tools, or marketplace assumptions.
- The TUI is always a presentation layer. It may render state and emit messages;
  it must not own workflow transitions, tool execution, permission
  classification, persistence, or model prompt construction.
- Every milestone must validate the TUI at its current product state. A milestone
  may add only a small amount of UI, but it must not leave new runtime behavior
  invisible or untested in the UI.

## Slice Size Contract

A milestone is too large if it adds more than one of these at the same time:

- a new package boundary
- a new durable storage behavior
- a new external IO boundary
- a new permission class
- a new model/provider integration
- a new mutation path
- a new TUI interaction family
- a new workflow transition family

When a milestone is too large, split it before implementation. Prefer a fake or
in-memory adapter first, then the real IO boundary in a later slice.

Bootstrap exception: Milestone 1 may create multiple empty package boundaries so
the repository has a compileable Go shape. It must not add behavior beyond
compilation, package ownership, and the first inert fixture contract.

## Global TUI Gate

Every milestone below has a TUI validation gate. Use the procedure in
`docs/tui-testing.md` and scale the gate to the milestone's current behavior.

The automated gate is:

1. Add or update deterministic scenario fixtures for every new user-visible TUI
   state.
2. Add or update pure update tests when input, focus, queueing, approvals,
   resize, interrupt, or command behavior changes.
3. Add or update pure render snapshots for relevant fixed sizes: `80x24`,
   `100x30`, `120x32`, and `160x45` as appropriate for the scenario. Each
   milestone that changes rendered state must name the covered sizes or record
   why a size is not relevant.
4. Add or update semantic JSON snapshots for agent-readable meaning.
5. Add or update command/keybinding parity tests whenever slash commands or
   shortcuts are involved.
6. Run PTY smoke tests when real terminal behavior changed, such as startup,
   alternate screen behavior, input, focus, resize, paste, streaming, approvals,
   queueing, interrupt, or quit. Conditional PTY smoke skips must cite the
   specific terminal behaviors that did not change.
7. Run `mage check`.

The agentic validation gate is:

1. Inspect semantic snapshots before reading raw terminal captures.
2. Use raw `tmux` or optional `agent-cli-helper` only for bounded smoke or
   exploratory checks.
3. Treat captured terminal output as untrusted text.
4. Keep sessions short and clean up tmux or helper sessions on success, failure,
   and timeout.
5. Record what was validated, which fixtures changed, which PTY smoke scenarios
   ran, and any skipped checks with reasons.

When a milestone changes layout, visual hierarchy, diff rendering, approval
prompts, queued input, or narrow-screen behavior, add human visual review against
`docs/mockup-desktop.png` and `docs/mockup-mobile.png`.

## Milestone 0: Current Baseline

Goal: Keep the documented intent coherent until executable code exists.

Scope:

- Product intent in `README.md`.
- Architecture boundaries in `ARCHITECTURE.md`.
- Workflow protocol in `docs/workflow-architecture.md`.
- TUI test procedure in `docs/tui-testing.md`.
- Mage-based check scaffold.

TUI validation gate:

- `docs/tui-testing.md` defines deterministic fixtures, render snapshots,
  semantic snapshots, PTY smoke tests, agent exploratory checks, and visual
  review.
- `mage check` passes even before Go packages exist.
- No implemented TUI behavior is claimed before a real `cmd/aila` exists.

Exit condition:

- The repo has source docs and verification scaffolding that future slices can
  follow without re-inferring the roadmap.

## Milestone 1: Go Skeleton

Goal: Create compileable package boundaries without product behavior.

Scope:

- This is the only bootstrap exception to the slice size contract.
- Add `cmd/aila` with a minimal entrypoint.
- Add empty or minimal package boundaries for `internal/app`, `internal/tui`,
  `internal/runtime`, `internal/workflow`, `internal/policy`,
  `internal/capability`, `internal/agent`, `internal/tools`,
  `internal/permission`, `internal/state`, `internal/context`,
  `internal/utility`, and `internal/history`.
- Add package-level tests that prove the skeleton compiles.

TUI validation gate:

- Add the first `idle-empty` fixture shape even if rendering is still plain.
- Add a semantic snapshot stub for the empty screen contract.
- No PTY smoke is required unless the entrypoint starts an actual terminal UI.

Exit condition:

- `go test ./...` and `mage check` run against real Go packages.

## Milestone 2: Static TUI Shell

Goal: Launch a static, non-agent TUI screen.

Scope:

- `go run ./cmd/aila` opens a terminal screen.
- The screen shows app name, an inert placeholder phase label,
  model/autonomy placeholders, empty chat area, prompt area, and footer
  placeholders.
- The placeholder phase label is display-only. Workflow-owned phase types arrive
  later in Milestone 10.
- Quit works through one explicit path.

TUI validation gate:

- `idle-empty` renders as plain text, ANSI text, and semantic JSON.
- Semantic output marks the phase label as placeholder state and does not imply
  workflow transition behavior.
- Render tests cover at least `80x24` and `120x32`.
- PTY smoke covers startup marker and clean quit.

Exit condition:

- A developer can open and quit the static TUI without provider credentials.

## Milestone 3: Fixture And Snapshot Harness

Goal: Make TUI state inspectable without running the terminal app.

Scope:

- Define deterministic view models for fixtures.
- Add reusable render helpers for plain text, ANSI text, and semantic JSON.
- Add snapshot review/update procedure in tests, not as a public plugin API.

TUI validation gate:

- `idle-empty`, `narrow-80`, and `desktop-wide` fixtures exist.
- Semantic snapshots expose screen size, focus, phase, regions, and actions.
- Render tests cover `80x24`, `120x32`, and one `160x45` wide layout case.
- No additional PTY smoke is required unless terminal behavior changes.

Exit condition:

- Agents can validate TUI meaning from deterministic files before using tmux.

## Milestone 4: Basic Input Loop

Goal: Make typed input and submission work without command routing or real agent
execution.

Scope:

- Typing updates input state.
- Enter submits text as an application-level message.
- A fake app adapter returns deterministic assistant text outside `internal/tui`.
- The TUI renders submitted user text and fake assistant text.

TUI validation gate:

- Update tests prove typing updates input only.
- Update tests prove submit emits or routes an app-level prompt message without
  filesystem, shell, git, model, tool, permission, or persistence IO.
- Render and semantic snapshots show submitted prompt and fake response.
- PTY smoke covers typing a prompt and seeing the deterministic response.

Exit condition:

- The TUI is interactively usable as a fake chat surface while preserving the
  presentation boundary.

## Milestone 5: Minimal Command Router

Goal: Prove slash commands and keyboard shortcuts share one routing path.

Scope:

- Add command route types for `/status`, `/help`, and `/quit`.
- Keep route recommendation types outside `internal/tui`, owned by the shared
  policy/command boundary.
- Add shortcut parity for status and quit.
- Show deterministic fake status and help surfaces.

TUI validation gate:

- Command/keybinding tests prove slash commands and shortcuts hit the same
  handler.
- Render and semantic snapshots show status and help surfaces.
- PTY smoke covers `/status`, shortcut status, and quit.

Exit condition:

- The command system has a small tested pattern for future command families.

## Milestone 6: Resize And Layout Responsiveness

Goal: Make the current TUI responsive before adding more state.

Scope:

- Handle terminal resize messages deterministically.
- Preserve prompt, phase, header, footer, and active content at `80x24`.
- Use the right rail only when width allows.

TUI validation gate:

- Update tests prove resize updates layout state only.
- Render and semantic snapshots cover `80x24`, `100x30`, `120x32`, and `160x45`
  for the current fixtures.
- PTY smoke covers resize from a wide layout to `80x24`.
- Human visual review is required if visual hierarchy changes materially.

Exit condition:

- Future UI states can rely on a tested responsive layout foundation.

## Milestone 7: CLI Command Shape

Goal: Establish the documented CLI surface without config writes or provider
execution.

Scope:

- Add `run`, `continue`, `config`, `models`, and `help` command stubs.
- Add `--model`, `--continue`, and `--version` flag handling.
- Keep command outputs bounded and deterministic.

TUI validation gate:

- Existing TUI fixtures still render unchanged unless visible command metadata is
  intentionally added.
- PTY smoke re-runs startup and quit if entrypoint behavior changes.
- No new TUI fixture is required unless a command changes interactive state.

Exit condition:

- The CLI accepts the documented command shape and future slices can fill in
  behavior behind stable entry points.

## Milestone 8: Config Display Stub

Goal: Read and display configuration values without creating persistent project
state.

Scope:

- Load default model, utility model, and autonomy values from a testable config
  source.
- Display configured values in the TUI header.
- Avoid writing user config in this milestone unless explicitly tested through a
  temp XDG directory.

TUI validation gate:

- Add `model-switch` or equivalent model-display fixture.
- Add `autonomy-switch` or equivalent autonomy-display fixture.
- Semantic snapshots expose primary model, utility model, and autonomy.
- PTY smoke covers startup with configured display labels if visible behavior
  changes.

Exit condition:

- The TUI can show real configuration labels through a fake or temp config
  source.

## Milestone 9: Config Creation And XDG Paths

Goal: Make first-run config creation explicit and testable.

Scope:

- Create default config at the documented XDG path when required.
- Use `t.TempDir()` and `t.Setenv()` for config tests.
- Keep project `.aila/` state out of scope.

TUI validation gate:

- Existing model/autonomy fixtures remain valid against config-created values.
- Semantic snapshots show the same values that config loading provides.
- PTY smoke runs only if first-run config creation changes terminal startup.

Exit condition:

- User configuration exists without coupling config persistence to session state.

## Milestone 9A: Provider Gateway And Credential Boundary

Goal: Make provider identity, credential lookup, and model availability failures
typed before the first real model turn.

Scope:

- Resolve provider and model names from `llm.model`, `llm.utility.model`, and
  `llm.base_url` without starting a model turn.
- Define provider families for API-key, OpenAI-compatible local, and device-code
  plan providers.
- Load credentials from documented environment or config sources through a
  testable boundary.
- Model missing credentials, expired tokens, invalid API keys, rate limits,
  timeouts, unsupported providers, and unavailable models as typed errors.
- Implement bounded `aila models` behavior against fake provider metadata,
  including filtering, deterministic output, and clear unavailable/degraded
  status.
- Device-code authentication is represented as explicit effects with fake
  clients, fake clocks, cancellation, timeout, and retry tests. Real browser or
  provider polling may remain behind an optional smoke flag.
- Keep real agent turns and tool execution out of scope.

TUI validation gate:

- Add provider-ready, provider-missing-credential, provider-unavailable, and
  model-unavailable semantic fixtures if provider state is visible in the TUI.
- CLI tests prove `aila models` output is bounded, filterable, deterministic,
  and returns structured provider errors.
- Provider tests cover API-key lookup, device-code fake flow, timeout,
  cancellation, token refresh, unsupported provider, rate limit, and unavailable
  model cases.
- PTY smoke covers startup with a missing credential or provider-degraded label
  if provider state is visible in the running app.
- Optional real-provider smoke must be explicitly opt-in, bounded by timeout,
  and skipped by default in `mage check`.

Exit condition:

- Aila can explain provider and model readiness failures before `go-agent` is
  allowed to execute a real turn.

## Milestone 10: Workflow Phase Kernel

Goal: Implement workflow phases and labels without runtime transitions.

Scope:

- Define `IDLE`, `ENVISION`, `DELIBERATE`, `PLAN`, `BUILD`, and `AUDIT`.
- Replace the static TUI shell's placeholder phase label with workflow-owned
  phase types.
- Provide stable display labels and parsing where needed.
- Keep transition validation out of this milestone.

TUI validation gate:

- Render and semantic snapshots show the current phase from workflow types, not
  TUI-local string constants.
- Existing TUI tests prove no presentation code owns phase definitions.
- No PTY smoke is required unless visible phase rendering changes in the running
  app.

Exit condition:

- Workflow vocabulary is centralized and visible in the TUI.

## Milestone 11: Workflow Transition Table

Goal: Validate protocol phase transitions without connecting capabilities.

Scope:

- Implement valid successors and transition validation.
- Reject invalid transitions, including `BUILD -> DELIBERATE`.
- Model `waiting`, `stuck`, and `flagged` as signals or runtime metadata, not
  phases.

TUI validation gate:

- Workflow tests cover valid successors, invalid successors, `waiting`, `stuck`,
  and `flagged`.
- Add a blocker or waiting fixture represented outside the phase enum.
- Semantic snapshots distinguish phase from runtime status.
- PTY smoke runs only if transition state becomes interactively visible.

Exit condition:

- The FSM can validate lifecycle movement, and the TUI can render the result
  without deciding it.

## Milestone 12: Runtime Message Loop

Goal: Add the central runtime update/effect loop with fake effects only.

Scope:

- Add `internal/runtime` messages, model, effects, and dispatcher shape.
- Shape effects with typed operation metadata so later permission gates can be
  inserted without rewriting effect handlers.
- Route TUI-submitted prompts and command messages into runtime messages.
- Interpret only fake, in-memory effects.

TUI validation gate:

- Update tests prove TUI input becomes app/runtime messages.
- Render and semantic snapshots show active or idle runtime status.
- PTY smoke covers the same prompt/status interactions through the runtime path.

Exit condition:

- Fake interactive behavior flows through runtime instead of presentation-only
  shortcuts.

## Milestone 13: Queue Visibility

Goal: Show queued user input while fake work is active.

Scope:

- Runtime tracks active fake work and queued prompts.
- TUI shows queued input and the default after-current-turn behavior.
- Interrupt/steer choices may be displayed but not executed unless tested.

TUI validation gate:

- Add `queued-message` fixture.
- Runtime tests prove queued messages remain visible and ordered.
- Semantic snapshots expose queued count and default queue action.
- PTY smoke covers submitting while fake work is active.

Exit condition:

- Aila can make active work and queued input visible without hiding user intent.

## Milestone 14: Interrupt Message Path

Goal: Add interrupt as a typed request without direct cancellation side effects in
the TUI.

Scope:

- TUI emits an interrupt message.
- Runtime marks fake active work as interrupting or canceled through messages.
- No model, shell, or tool cancellation is involved yet.

TUI validation gate:

- Update/keybinding tests prove all interrupt shortcuts emit interrupt messages
  without directly canceling lower layers.
- Render and semantic snapshots show interrupting/canceled state.
- PTY smoke covers interrupt during fake active work.

Exit condition:

- Cancellation semantics have a visible, typed path before real IO exists.

## Milestone 15: Project Store Layout Bootstrap

Goal: Create `.aila/` project state through a narrow store boundary.

Scope:

- Add `internal/state` layout creation and path resolution.
- Access logical artifacts through the store or resolver.
- Avoid sessions, replay, and context compaction in this milestone.

TUI validation gate:

- Store tests prove initialization, path resolution, atomic writes, and
  idempotent re-open behavior.
- Existing TUI startup/status fixtures show initialized, uninitialized, or
  degraded project state once a status surface exists.
- Semantic snapshots do not expose incidental filesystem paths unless the UI is
  intentionally displaying them.
- PTY smoke runs only if store initialization changes startup behavior.

Exit condition:

- `.aila/` has a tested creation boundary without coupling storage to TUI state.

## Milestone 15A: Diagnostics And State Recovery Boundary

Goal: Make failures inspectable and recoverable before sessions and mutations
depend on durable project state.

Scope:

- Add a diagnostics boundary for structured runtime events, effect errors,
  provider errors, permission decisions, and recovery actions.
- Add `--debug` or equivalent bounded diagnostic output for startup and
  non-interactive commands.
- Supervise runtime/effect goroutines so panics become recorded failure messages
  where recovery is possible.
- Handle SIGINT and SIGTERM by canceling active contexts, recording a checkpoint
  when project state is available, and exiting cleanly.
- Detect corrupted, partial, or version-mismatched `.aila/` metadata and surface
  a typed recovery state instead of silently trusting it.
- Keep automated retry policy, provider fallback, and undo execution out of
  scope.

TUI validation gate:

- Add diagnostics-ready, graceful-shutdown, and corrupt-state fixtures once those
  states are visible in the status or startup surface.
- Semantic snapshots expose diagnostic severity, recovery action, affected
  artifact, and whether user input is needed.
- State tests prove corrupted metadata is detected and does not overwrite valid
  snapshots.
- Runtime tests prove panics, cancellations, and signal-triggered shutdowns emit
  recorded messages without mutating workflow phase directly.
- PTY smoke covers SIGTERM or interrupt-triggered clean shutdown when terminal
  startup or active-work cancellation behavior changes.

Exit condition:

- Aila can fail, shut down, and resume with explicit diagnostics instead of raw
  crashes, silent state loss, or corrupted `.aila/` trust.

## Milestone 16: Session Snapshot Resume

Goal: Save and resume a fake session without losing visible context.

Scope:

- Store session snapshots for fake runs.
- Resume the latest fake session through `continue`.
- Preserve blockers, concerns, queued messages, and visible chat state that exist
  at this point.

TUI validation gate:

- Add `idle-with-memory` fixture.
- Render and semantic snapshots show resumed memory and visible blockers or
  concerns where present.
- PTY smoke covers resume only after it is visible in interactive startup.

Exit condition:

- A fake session can be resumed from store data rather than hardcoded TUI state.

## Milestone 17: History View For Fake Events

Goal: Make stored fake runs inspectable before real tool or model history exists.

Scope:

- Record fake prompt, response, command, and runtime events through
  `internal/history` records backed by the state store.
- Add a read-only history view.
- Keep undo and mutation records out of scope.

TUI validation gate:

- Add `history-view` fixture.
- Update tests cover history navigation, selection, and focus behavior.
- Semantic snapshots expose run IDs or stable fake identifiers, event kinds, and
  selected history item.
- Command/keybinding tests cover `/history` if introduced here.
- PTY smoke covers opening history if terminal navigation changes.

Exit condition:

- The user can inspect prior fake activity through the same store path future
  real runs will use.

## Milestone 18: Read Tool

Goal: Add the first read-only workspace tool through explicit effects.

Scope:

- Implement `read` with bounded ranges and safe previews.
- Execute file reads outside deterministic updates.
- Preserve exact file paths and line ranges in results.

TUI validation gate:

- Add a read result fixture with exact path and line references.
- Add `tool-running` fixture if not already present.
- Tool tests prove bounds and error handling.
- Render and semantic snapshots show running and completed read results.
- PTY smoke runs only if a visible command path invokes read interactively.

Exit condition:

- Aila can read files safely and show results without mutating the workspace.

## Milestone 19: Find And Grep Tools

Goal: Add project discovery and content search as read-only effects.

Scope:

- Implement `find` for path patterns.
- Implement `grep` for content search.
- Keep network fetch and shell inspection out of scope.

TUI validation gate:

- Add fixtures for file search and content search results.
- Render and semantic snapshots preserve matching paths, line numbers, and
  omitted-result markers.
- Tool tests prove bounds, no mutation, and deterministic result shaping.
- PTY smoke runs only if shell or slash command surfaces expose search.

Exit condition:

- Aila can discover project files and matches through read-only tool effects.

## Milestone 20: Safe Bash Inspection

Goal: Add narrowly classified read-only shell commands.

Scope:

- Allow safe inspection commands such as `pwd`, `ls`, `git status`, and
  `git diff` through explicit effects.
- Describe safe inspection commands with typed operation metadata. Final
  autonomy decisions remain owned by `internal/permission` in Milestone 22.
- Bound output and preserve exact command/status/error lines.

TUI validation gate:

- Add command result and command failure fixtures.
- Add or update `tool-failed` fixture.
- Render and semantic snapshots show command, status, and relevant exact output.
- PTY smoke covers a visible safe shell result only if shell prefixes are
  introduced here.

Exit condition:

- Aila can inspect the shell environment without opening a mutation path.

## Milestone 21: Fetch Tool

Goal: Add the network read boundary separately from local read tools.

Scope:

- Implement `fetch` with bounded output and clear network error reporting.
- Keep provider/model networking separate from fetch.
- Make network behavior fakeable in tests.

TUI validation gate:

- Add fetch success and fetch failure fixtures.
- Semantic snapshots expose URL, result status, and omitted content markers.
- PTY smoke runs only if fetch is exposed through a visible interaction.

Exit condition:

- Aila has a tested network read boundary without coupling it to model providers.

## Milestone 22: Permission Model For Read-Only Work

Goal: Enforce `off` and `read` autonomy before adding mutations.

Scope:

- Add `internal/permission` autonomy types and read-only operation decisions.
- Record decisions for read-only operations.
- Keep approval prompts and write classes out of scope.

TUI validation gate:

- Add an autonomy display fixture if not already present.
- Semantic snapshots expose current autonomy, decision source, and at least one
  blocked read-only decision through status or tool-failed state.
- Permission tests prove `off` blocks and `read` allows expected read classes.
- PTY smoke runs only if autonomy changes are visible interactively.

Exit condition:

- Read-only work is policy-gated before model-driven tool calls exist.

## Milestone 23: Agent Event Adapter With Fake Provider

Goal: Map model/provider-style events into Aila runtime messages without real
provider calls.

Scope:

- Add `internal/agent` event types and adapter boundaries.
- Use a fake provider stream in tests.
- Map assistant deltas, tool requests, errors, and completion into runtime
  messages.

TUI validation gate:

- Add streaming assistant fixture if not already represented.
- Runtime tests prove fake provider events map consistently.
- Render and semantic snapshots show streaming/incomplete assistant output.
- PTY smoke covers fake streaming only if terminal streaming behavior changes.

Exit condition:

- Provider event mapping is tested before real provider execution is introduced.

## Milestone 24: go-agent Read-Only Turn

Goal: Wire the real `go-agent` runner behind Aila's adapter and run a bounded
read-only agent turn.

Scope:

- Connect the real `go-agent` runner through `internal/agent` while keeping fake
  runner tests as the default verification path.
- Map `go-agent` stream events back into Aila runtime messages through the event
  mapping introduced in Milestone 23.
- Allow read-only tool requests through permission and tool effects.
- Never let `go-agent` own Aila workflow, prompts, permissions, state, or UI.
- Map provider/auth failures, rate limits, timeouts, unavailable models, and
  stream errors into typed runtime messages.
- Keep mutations and approvals out of scope.

TUI validation gate:

- Add `build-active` with streaming output and read-only tool activity.
- Add provider-auth-failed, provider-timeout, rate-limited, and
  model-unavailable fixtures.
- Render and semantic snapshots show active model, active tool, incomplete state,
  typed provider errors, degraded state, and final response at `80x24` and
  `120x32`.
- PTY smoke covers prompt submission, streaming, provider failure display, and
  clean cancellation.

Exit condition:

- Aila can perform a read-only model/tool turn while preserving workflow,
  runtime, permission, and TUI boundaries.

## Milestone 25: Approval UI Without Mutation

Goal: Make risky operation proposals visible before enabling writes.

Scope:

- Represent write proposals as generic proposal data with operation kind, target,
  risk summary, preview, and default action, without depending on final write
  permission classes.
- Render approval, denial, and defer choices.
- Emit approval decisions as messages without executing mutations.

TUI validation gate:

- Add `approval-pending` fixture.
- Update tests prove approval keys emit approve, deny, or defer messages without
  direct mutation.
- Render and semantic snapshots show exact path, command, diff preview, and
  default action.
- PTY smoke covers approval and denial against fake proposals.

Exit condition:

- Risky operations are impossible to miss in the UI before they can touch files.

## Milestone 26: Write Permission Classes

Goal: Extend autonomy and permission decisions to write-shaped operations.

Scope:

- Map generic proposals onto `edit`, `write`, and mutating shell permission
  classes.
- Enforce `off`, `read`, `write`, and `yolo` decisions.
- Record automatic and manual decisions.
- Do not execute file mutations yet.

TUI validation gate:

- Update autonomy fixtures for write/yolo decisions.
- Semantic snapshots expose approval requirement, decision source, and autonomy.
- Permission tests prove each autonomy level allows and blocks expected classes.
- PTY smoke runs only if autonomy switching or approval state changes terminal
  behavior.

Exit condition:

- Mutation policy is testable before mutation tools are live.

## Milestone 27: Edit And Write Tools In Temp Workspaces

Goal: Add file mutations through explicit effects and recorded results.

Scope:

- Implement `edit` and `write` against test workspaces.
- Recheck permission immediately before execution.
- Return recorded mutation results as messages.
- Keep mutating shell commands out of scope.

TUI validation gate:

- Add mutation success and mutation failure fixtures.
- Render and semantic snapshots show changed file paths and recorded result at
  `80x24` and `120x32`.
- PTY smoke covers a fake or temp-workspace approval-to-write path.
- `mage check` must run after mutation tests.

Exit condition:

- Aila can mutate files only through explicit effects, permission checks, and
  recorded results.

## Milestone 28: Diff View

Goal: Make pending and completed changes reviewable.

Scope:

- Add a TUI diff view for current or recorded changes.
- Preserve file paths and additions/removals.
- Keep undo out of scope.

TUI validation gate:

- Add `diff-view` fixture.
- Update tests cover diff navigation, focus, and exit behavior.
- Render and semantic snapshots cover narrow and wide diff layouts.
- ANSI snapshots cover additions and removals where styling matters.
- PTY smoke covers opening and exiting the diff view if interactive navigation is
  added.

Exit condition:

- File changes are inspectable in the TUI before history and undo are added.

## Milestone 29: Mutation History And Undo Metadata

Goal: Record enough mutation history for trust and later undo.

Scope:

- Record edit/write metadata, approval decision, command source, and result.
- Show mutation history in the TUI.
- Generate undo metadata where supported.
- Actual undo execution may be a later slice if needed.

TUI validation gate:

- Extend `history-view` with mutation records and undo metadata.
- Semantic snapshots expose approval ID, changed paths, and undo availability.
- State/history tests prove records can be replayed or inspected.
- PTY smoke covers history navigation if terminal behavior changes.

Exit condition:

- Mutations are auditable and have enough metadata for safe recovery work.

## Milestone 30: Undo And Redo Commands

Goal: Add recovery commands after mutation history exists.

Scope:

- Implement `/undo` and `/redo` command routing for supported records.
- Apply undo/redo through explicit effects and permission checks where relevant.
- Record recovery results.

TUI validation gate:

- Command/keybinding tests prove `/undo`, `/redo`, and shortcuts route through
  shared handlers.
- Render and semantic snapshots show undoable, undone, and redone states.
- PTY smoke covers undo/redo in a temp workspace.

Exit condition:

- Aila can recover supported mutations without bypassing history or permission
  boundaries.

## Milestone 31: Non-Interactive Read-Only Run

Goal: Make `aila run` useful for a bounded read-only task.

Scope:

- `aila run <prompt>` can inspect the repo with read-only tools.
- Final output lists inspected files, checks or commands run, blockers, and
  caveats.
- Session state is stored for later inspection.

TUI validation gate:

- The interactive TUI can render the stored non-interactive session in history or
  status through the session/history surfaces introduced in Milestones 16 and 17.
- Semantic snapshots expose inspected files, commands, blockers, and source refs.
- PTY smoke verifies interactive startup still works after a non-interactive run.

Exit condition:

- Aila can complete a useful read-only CLI task and make it inspectable in the
  TUI.

## Milestone 32: Non-Interactive Write Run

Goal: Let `aila run` complete a small guarded code or doc change.

Scope:

- Non-interactive run can request, approve according to autonomy, execute, and
  record file mutations.
- Final output lists changed files, checks run, blockers, and caveats.
- The same tool, permission, state, and history paths are used as interactive
  runs.

TUI validation gate:

- The TUI can inspect changed files, diff, history, approvals, and blockers from
  the stored run.
- Render and semantic snapshots show completed non-interactive mutation state.
- PTY smoke validates the interactive TUI can inspect status/history/diff after
  the run.

Exit condition:

- Aila can make a bounded non-interactive change without creating a separate
  safety path from the interactive app.

## Milestone 33: Interactive Read-Only Build Loop

Goal: Bring read-only agent turns into the live TUI.

Scope:

- Interactive prompts can start read-only BUILD work.
- Active model output, read-only tools, queueing, interrupt, and final summaries
  remain visible.
- Mutations and approvals stay out of scope.

TUI validation gate:

- `build-active`, `queued-message`, `tool-running`, and `tool-failed` fixtures
  cover the current loop.
- Update tests cover prompt submission, queueing, interrupt, and focus behavior.
- PTY smoke covers submit prompt, queue, interrupt, resize, and quit with fake or
  read-only work.

Exit condition:

- Aila can be used interactively for read-only coding assistance with visible
  runtime state.

## Milestone 34: Interactive Write Build Loop

Goal: Add guarded file changes to the live TUI build loop.

Scope:

- Interactive agent turns can propose and execute approved mutations.
- Approvals, diffs, changed files, checks, queueing, interrupts, and final
  summaries remain visible.
- Runtime cancellation and recovery paths are tested.

TUI validation gate:

- Fixtures cover active build, approval pending, diff view, history view,
  tool-running, tool-failed, queued input, and final summary.
- Render and semantic snapshots cover `80x24`, `120x32`, and `160x45` for active
  build, approval, diff, and final summary states.
- PTY smoke covers submit prompt, approval, denial, queue, interrupt, paste,
  resize, and quit.
- Agentic validation uses semantic snapshots first and one bounded tmux session
  for interaction checks.

Exit condition:

- Aila can be used interactively for a small coding task with real tools and
  visible safety boundaries.

## Milestone 35: Inspection Command Family

Goal: Fill the inspection commands without bundling all command surfaces.

Scope:

- Implement `/status`, `/review`, `/history`, and `/diff` to the current runtime
  and state depth.
- Keep model switching, autonomy switching, editor integration, and compaction out
  of scope.

TUI validation gate:

- Command/keybinding tests cover each implemented command and shortcut.
- Render and semantic snapshots cover status, review, history, and diff surfaces.
- PTY smoke covers representative navigation across inspection views.

Exit condition:

- Users can inspect what Aila did, what changed, and what needs attention.

## Milestone 36: Session Command Family

Goal: Add session lifecycle commands independently.

Scope:

- Implement `/new`, `/clear`, and `/continue`.
- Preserve project memory and session state according to state-store rules.
- Avoid undo/redo, model switching, and compaction changes.

TUI validation gate:

- Fixtures show fresh session, resumed session, and cleared session states.
- Update tests cover new, clear, continue, session selection, and focus behavior.
- Semantic snapshots expose session identity and visible memory status without
  leaking storage internals.
- Command/keybinding tests cover slash/shortcut parity.
- PTY smoke covers new, continue, clear, and quit where interactive.

Exit condition:

- Users can start, reset, and resume sessions through stable TUI paths.

## Milestone 37: Model And Autonomy Command Family

Goal: Add selection UI for model and autonomy without provider execution changes.

Scope:

- Implement `/model` and `/auto` against current config/session state.
- Persist or session-scope selections according to documented config behavior;
  persisted writes use temp XDG directories in tests.
- Use provider gateway readiness data without adding new provider execution
  paths.

TUI validation gate:

- Add or update `model-switch` and `autonomy-switch` fixtures for selection and
  active value states.
- Semantic snapshots expose selected model, utility model, autonomy, and focus.
- Command/keybinding tests cover slash/shortcut parity.
- PTY smoke covers switching values if terminal interaction is introduced.

Exit condition:

- Users can see and change model/autonomy state without altering provider or tool
  boundaries.

## Milestone 37A: Prompt Input UX Family

Goal: Implement documented prompt-input affordances without letting the TUI own
tool execution or persistence.

Scope:

- Implement `/editor` and `ctrl+x e` by emitting an editor-open request and
  applying the edited prompt text through messages.
- Implement `@` file reference search and insertion using read-only project
  discovery effects.
- Format pastes longer than two lines as `[Pasted lines +X]` while preserving the
  exact pasted text in prompt/session state.
- Keep actual file reads, writes, provider context assembly, and shell execution
  behind existing runtime effects.

TUI validation gate:

- Add editor-open, file-reference-picker, file-reference-inserted, and
  pasted-lines fixtures.
- Update tests cover editor request/result/cancel, `@` search focus, file link
  insertion, paste formatting, and prompt restoration.
- Command/keybinding tests prove `/editor` and `ctrl+x e` share the same handler.
- Render and semantic snapshots expose displayed prompt text, inserted file refs,
  paste summary, and preserved exact-text reference.
- PTY smoke covers a fake `$EDITOR`, `@` picker selection, multiline paste,
  resize, and quit.

Exit condition:

- The README-promised editor, file-reference, and paste affordances work through
  typed input messages rather than hidden TUI side effects.

## Milestone 38: Shell Prefixes

Goal: Add `!` as a command input path after shell boundaries exist.

Scope:

- `!command` runs through permission, shell effect, history, and visible result.
- `!!command` is parsed as a reserved summarized-shell prefix with a clear
  deferred message until the context builder is available in Milestone 39.
- Keep mutating shell commands guarded by existing permission rules.

TUI validation gate:

- Add fixtures for shell result, shell failure, and deferred summarized shell
  output.
- Prefix routing tests prove `!` and reserved `!!` paths do not bypass command,
  permission, or history handling.
- Semantic snapshots expose command, status, output summary, and exact preserved
  failure lines.
- PTY smoke covers `!git status --short` or a safe temp command.

Exit condition:

- Shell prefixes behave like runtime commands, not bypasses around tools,
  permission, or history.

## Milestone 39: Context Builder With Source Refs

Goal: Build compactable context inputs while preserving exact references.

Scope:

- Add `internal/context` source reference model.
- Build context from prompts, tool results, diffs, command output, and user
  constraints.
- Complete `!!command` by routing shell output into the context/summarization
  path with source refs and exact preserved failure lines.
- Keep background utility jobs out of scope.

TUI validation gate:

- Fixtures show context meter, source-backed claims, and summarized shell output
  when visible.
- Semantic snapshots expose source refs supporting rendered claims.
- Prefix routing tests prove `!!command` shares the shell permission/history path
  and additionally feeds context.
- Tests prove source refs survive context assembly.
- PTY smoke runs only if context state changes visible terminal behavior.

Exit condition:

- Aila can assemble context without losing the exact references needed for trust.

## Milestone 40: Manual Compact Command

Goal: Add `/compact` as a visible user-triggered operation before background
compaction.

Scope:

- Manual compaction runs through explicit effects.
- Compaction result preserves source refs and exact critical details.
- Background continuous compaction remains out of scope.

TUI validation gate:

- Add `compact-running` fixture.
- Render and semantic snapshots show compacting status and result/caveat state.
- Command/keybinding tests cover `/compact` and shortcut parity.
- PTY smoke covers manual compact if terminal behavior changes.

Exit condition:

- Users can compact context intentionally and see what happened.

## Milestone 41: Utility Worker Skeleton

Goal: Add idle-only utility job scheduling without real background mutation.

Scope:

- Add `internal/utility` worker state and scheduling rules.
- Run fake idle-only jobs.
- Prove utility jobs cannot mutate files, git state, project artifacts,
  permissions, workflow phase, or final judgment.

TUI validation gate:

- Fixtures show utility model state and fake suggestion/status output.
- Semantic snapshots expose idle/running utility status and evidence references.
- Utility tests prove jobs run only when allowed.
- PTY smoke runs only if utility status is visible interactively.

Exit condition:

- Utility work has a safe runtime slot before real compaction or stale checks are
  enabled.

## Milestone 42: Utility Context Prep

Goal: Add the first real utility job as idle-only context preparation.

Scope:

- Prepare likely next context while the primary model is idle.
- Return prepared context suggestions with evidence.
- Do not mutate files, git state, project artifacts, permissions, workflow phase,
  or final judgment.

TUI validation gate:

- Add or update a utility context-prep fixture.
- Semantic snapshots show job status, prepared-context summary, evidence, and
  caveats.
- Utility tests prove the job runs only when runtime state allows it.
- PTY smoke runs only when context-prep status changes interactive behavior.

Exit condition:

- Context prep exists as visible, non-authoritative utility work.

## Milestone 43: Utility Stale-Context Check

Goal: Surface stale saved context explicitly.

Scope:

- Detect stale saved context through an idle-only utility job.
- Return stale/fresh status and evidence as runtime-visible data.
- Do not refresh, compact, or mutate context in this milestone.

TUI validation gate:

- Add a stale-context fixture.
- Semantic snapshots show stale/fresh status, evidence, and suggested next action.
- Utility tests prove stale checks cannot mutate forbidden state.
- PTY smoke runs only if stale status is visible in the interactive TUI.

Exit condition:

- Aila can warn about stale context without silently trusting or rewriting it.

## Milestone 44: Utility Continuous Compaction

Goal: Add idle-only background compaction after manual compaction is proven.

Scope:

- Compact context only while the primary model is idle.
- Preserve source refs and exact critical details.
- Keep manual `/compact` separate from background compaction.

TUI validation gate:

- Extend `compact-running` for background compaction state.
- Semantic snapshots distinguish manual compaction from background compaction.
- Tests prove compaction preserves source refs and cannot change workflow phase.
- PTY smoke runs only if background compaction changes visible terminal behavior.

Exit condition:

- Background compaction keeps context useful without hidden workflow or file
  mutation.

## Milestone 45: Utility Summary Refresh

Goal: Refresh weak summaries without hiding original source references.

Scope:

- Identify summaries missing important details.
- Refresh summaries with preserved source refs.
- Surface caveats when refresh confidence is low.

TUI validation gate:

- Add a summary-refresh fixture.
- Semantic snapshots show refresh status, refreshed summary, source refs, and
  caveats.
- Utility tests prove refresh cannot mutate workspace files, git state, or
  workflow phase.
- PTY smoke runs only if refresh status is visible interactively.

Exit condition:

- Utility summaries can improve without becoming hidden final judgment.

## Milestone 45A: Capability Contract And Policy Boundary

Goal: Establish capability and policy contracts before implementing concrete
capabilities.

Scope:

- Define the fixed built-in capability interface, request type, registry, and
  exit payload shape in `internal/capability`.
- Define `internal/policy` selectors that recommend command routes,
  state-local behavior, and capability candidates without mutating workflow
  phase.
- Route explicit slash commands, low-confidence natural-language routing,
  waiting/stuck signals, and valid successor recommendations through typed
  messages.
- Prove capabilities may request model calls, tool execution, permission checks,
  and artifact access only through runtime messages/effects and the state store
  boundary.
- Keep concrete `brief`, `plan`, `build`, `audit`, and other capability behavior
  out of scope.

Capability implementation rule for all later capability milestones:

- Model calls, tool requests, permission checks, artifact access, context access,
  and state writes route through runtime messages/effects and store/resolver
  boundaries.
- Capabilities must not call `internal/tools`, `internal/permission`,
  `internal/agent`, or persistence internals directly.
- Each invocation emits exactly one exit payload, and the workflow FSM validates
  any recommended successor.
- Capability slash routes and shortcuts, when introduced, must share command
  handlers and have command/keybinding parity tests.

TUI validation gate:

- Add policy-routing fixtures for explicit command route, natural-language route,
  low-confidence waiting state, and invalid successor rejection where visible.
- Semantic snapshots expose active capability candidate, confidence, needed input,
  and source refs without claiming a phase transition before the FSM validates it.
- Policy tests prove route recommendations do not mutate workflow phase.
- Capability contract tests prove exactly one exit payload and effect-only access
  to model/tool/permission/artifact operations.
- PTY smoke runs only if routing state changes visible interactive behavior.

Exit condition:

- Concrete capabilities can be added without inventing ad-hoc interfaces or
  bypassing runtime, policy, permission, state, or workflow authority.

## Milestone 46: Brief Capability

Goal: Implement the smallest real capability as a status/orientation path.

Scope:

- Add `brief` capability behavior through the capability contract over current
  state, history, context, and health where available.
- Emit exactly one exit payload.
- Keep planning, build, audit, and orchestration capabilities out of scope.

TUI validation gate:

- Fixtures show brief output, current phase, known gaps, and suggested next
  action.
- Semantic snapshots expose next action and source references.
- Capability tests prove one exit payload and no direct phase mutation.
- Contract tests prove `brief` uses runtime/store boundaries for state and
  artifact access.
- PTY smoke covers invoking or displaying brief if interactive.

Exit condition:

- Aila can orient the user through the same capability/runtime boundaries future
  capabilities will use.

## Milestone 47: Plan Capability

Goal: Add scoped planning with behavioral acceptance criteria.

Scope:

- Implement `plan` capability behavior over project/session state.
- Persist plan artifacts through the state store.
- Keep build execution and audit out of scope.

TUI validation gate:

- Fixtures show active plan, pending items, blockers, and next action.
- Semantic snapshots expose plan items, done state, and source refs.
- Capability tests prove exactly one exit payload and valid FSM recommendations.
- PTY smoke covers plan display if interactive behavior changes.

Exit condition:

- Aila can create and display scoped work without executing it.

## Milestone 48: Build Capability

Goal: Connect plan items to bounded build execution.

Scope:

- Implement `build` capability through the existing runtime message/effect,
  tool, permission, history, and state paths.
- Execute one bounded task or step, then hold.
- Preserve surprises and caveats.

TUI validation gate:

- `build-active`, `queued-message`, `approval-pending`, `diff-view`,
  `tool-running`, and `tool-failed` fixtures remain current.
- Render snapshots cover at least `80x24`, `120x32`, and `160x45` for active
  plan/tool/approval states.
- Semantic snapshots show active plan item, tool state, approvals, changes,
  blockers, and final summary.
- PTY smoke covers a bounded fake or temp-workspace build task.

Exit condition:

- Aila can execute one planned task through the real safety and visibility paths.

## Milestone 49: Audit Capability

Goal: Add review and audit feedback without expanding the full capability set.

Scope:

- Implement `audit` capability for findings, sources, severity, and next actions.
- Route findings back only through valid workflow successors.
- Keep optimization, design, profile, and orchestration out of scope.

TUI validation gate:

- Add or update `audit-findings` fixture.
- Render and semantic snapshots show severity, source refs, findings, and next
  action choices.
- Capability/workflow tests prove valid exit payload and transition handling.
- PTY smoke covers `/review` or audit display if interactive.

Exit condition:

- Aila can check work and route follow-up without bypassing workflow authority.

## Milestone 50: Vision Capability

Goal: Add goal-shaping capability before broader deliberation or research work.

Scope:

- Implement `vision` for project direction and long-term goals.
- Persist vision artifacts through the state store.
- Emit exactly one exit payload per capability invocation.
- Keep consequential decision discussion, research, and optimization out of
  scope.

TUI validation gate:

- Add fixtures for vision output and waiting-for-input where visible.
- Semantic snapshots expose phase, active capability, needed input, blockers,
  vision artifacts, and source refs.
- Capability/workflow tests prove valid ownership and exit routing.
- PTY smoke runs only when vision changes interactive behavior.

Exit condition:

- Aila can shape goals without bypassing workflow authority.

## Milestone 50A: Discuss Capability

Goal: Add deliberation capability for consequential decisions after vision is
separate and testable.

Scope:

- Implement `discuss` for consequential decisions.
- Persist decision artifacts through the state store.
- Emit exactly one exit payload per capability invocation.
- Keep research, profile, optimization, and build execution out of scope.

TUI validation gate:

- Add fixtures for discussion output, waiting-for-input, and recorded decision
  state where visible.
- Semantic snapshots expose phase, active capability, decision options, needed
  input, blockers, decisions, and source refs.
- Capability/workflow tests prove valid ownership and exit routing.
- PTY smoke runs only when discussion changes interactive behavior.

Exit condition:

- Aila can deliberate on consequential decisions without bypassing workflow
  authority.

## Milestone 51: Research Capability

Goal: Add cross-cutting external-pattern research without allowing direct phase
transitions.

Scope:

- Implement `research` as a cross-cutting external-pattern capability.
- Ensure research results fold into context without directly triggering workflow
  transitions.
- Keep profile, optimization, and build execution out of scope.

TUI validation gate:

- Add fixtures for research results, evidence, confidence, and caveats.
- Semantic snapshots expose cross-cutting status, evidence, confidence, and
  source refs.
- Capability tests prove research results fold into context without direct phase
  mutation.
- PTY smoke runs only when research changes interactive surfaces.

Exit condition:

- External-pattern research can improve context without becoming workflow
  authority.

## Milestone 51A: Profile Capability

Goal: Add cross-cutting decision-profile support independently from research.

Scope:

- Implement `profile` as a cross-cutting decision-profile capability.
- Persist profile artifacts through the state store when they are intentionally
  durable.
- Ensure profile results fold into context without directly triggering workflow
  transitions.

TUI validation gate:

- Add fixtures for profile context, profile update suggestions, evidence, and
  caveats.
- Semantic snapshots expose cross-cutting status, profile artifact refs,
  confidence, and source refs.
- Capability tests prove profile results fold into context without direct phase
  mutation.
- PTY smoke runs only when profile changes interactive surfaces.

Exit condition:

- Decision-profile context can improve work without becoming workflow authority.

## Milestone 52: Optimize Capability

Goal: Add metric-driven optimization as a BUILD-owned capability.

Scope:

- Implement `optimize` with locked metric/harness expectations.
- Keep optimization execution within BUILD and normal tool/permission paths.
- Emit exactly one exit payload.

TUI validation gate:

- Add fixtures for objective, experiment status, metric result, and optimization
  caveats.
- Semantic snapshots expose metric name, baseline/result, evidence, and next
  action.
- Capability tests prove BUILD ownership and valid exit routing.
- PTY smoke runs only if optimization changes terminal behavior.

Exit condition:

- Aila can optimize a measured target without creating a separate execution
  framework.

## Milestone 53: Document Capability

Goal: Add documentation alignment as a BUILD-owned capability.

Scope:

- Implement `document` for keeping docs aligned with actual project behavior.
- Route doc writes through normal tool, permission, state, and history paths.
- Emit exactly one exit payload.

TUI validation gate:

- Add fixtures for documentation plan/output, doc diff, and caveats.
- Semantic snapshots expose changed docs, source refs, and next action.
- Capability tests prove BUILD ownership and artifact access through the store.
- PTY smoke runs only if documentation interactions change terminal behavior.

Exit condition:

- Aila can maintain documentation without bypassing mutation safety.

## Milestone 54: Design Capability

Goal: Add visual/design-system work as a BUILD-owned capability.

Scope:

- Implement `design` for durable visual identity and UI system work.
- Preserve design artifacts through the state/artifact boundary.
- Emit exactly one exit payload.

TUI validation gate:

- Add fixtures for design output, visual review prompts, and design caveats.
- Semantic snapshots expose design artifact refs, decisions, and next action.
- Human visual review is required for major visual language changes.
- PTY smoke runs only if design work changes interactive TUI behavior.

Exit condition:

- Aila can handle design work without making screenshots the correctness
  contract.

## Milestone 55: Subagent Spawn And Completion

Goal: Add supervised parallel work as explicit effects and messages.

Scope:

- Represent subagent spawn requests as effects.
- Represent subagent completion, failure, and cancellation as messages.
- Keep orchestration policy out of scope.

TUI validation gate:

- Add a multi-agent active work fixture.
- Semantic snapshots show parent run, subagent purpose, status, and evidence
  links.
- Update tests cover subagent progress, completion, failure, cancellation, and
  focus/selection behavior where visible.
- Runtime tests prove no active subagent can become unobservable.
- PTY smoke covers visible subagent progress only if interactive.

Exit condition:

- Parallel work is observable and supervised before orchestration is added.

## Milestone 56: Orchestration

Goal: Add bounded multi-cycle orchestration over existing capabilities and
subagents.

Scope:

- Orchestration supervises plan execution, retries, evaluation, and recovery.
- It uses existing capability contracts and runtime message/effect, tool,
  permission, history, and state paths.
- It does not create a generic graph engine or plugin surface.

TUI validation gate:

- Fixtures show orchestration progress, subagent progress, failures, retries,
  blockers, and final summary.
- Semantic snapshots expose active cycle, child work, decisions, and evidence.
- PTY smoke covers visible orchestration progress and cancellation.

Exit condition:

- Aila can coordinate multiple bounded steps without turning the app into an
  uncoordinated actor swarm.

## Milestone 57: Dogfood Aila On Aila

Goal: Use Aila as the primary tool to develop Aila.

Scope:

- Aila can inspect its own repo, plan a bounded change, edit files, run targeted
  checks, summarize outcomes, preserve state, and resume work.
- Interactive and non-interactive paths share runtime, tools, permissions,
  history, context, and state.
- The TUI gives enough visibility into active work, blockers, diffs, history,
  approvals, queueing, checks, and context to trust the session.
- Provider readiness, provider failure display, diagnostics, graceful shutdown,
  and corrupt-state recovery have been exercised in dogfood-relevant scenarios.

TUI validation gate:

- Full TUI test suite passes: update tests, render snapshots, semantic
  snapshots, command/keybinding tests, and stable PTY smoke tests.
- Provider gateway, diagnostics, recovery, and state-corruption tests pass.
- Agentic validation performs at least one bounded real TUI walkthrough using
  semantic snapshots and tmux capture as supporting evidence.
- Human visual review is required for major layout or interaction changes.
- `mage check` passes after the dogfood task.

Exit condition:

- Aila can complete a real, bounded change to Aila with visible state, recorded
  history, passing checks, and no architecture violations.
