# Changelog

## [Unreleased]

### Added

- Scheduled idle utility work after foreground prompt completion, covering context prep, stale-context checks, summary refresh, and next-action suggestions without mutating project state.
- Wired go-agent build turns with bounded system instructions, all seven fixed built-in tools, and registered tool-call dispatch through Aila runtime effects.
- Added graceful agent turn continuation with queued prompt draining and interrupt-to-cancel handoff.
- Wired all fixed built-in capabilities through the app-owned agent runner with capability-specific context, streamed output, typed exits, and bounded failure and cancellation handling.

### Fixed

- Scoped git status probing to the workspace directory so repo-local temporary directories do not inherit parent repository state during tests.

### Changed

- Added `ROADMAP.md` Milestone 2's static Bubble Tea TUI shell with validated `idle-empty` render and semantic snapshots.
- Established `ROADMAP.md` Milestone 1's compileable Go package skeleton and inert TUI fixture contract.
- Archived the completed model-backed capability runner plan so the next cycle can start the P6 idle utility worker work from clean Agentera state.
