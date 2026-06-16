# Agent Guide

This file orients coding agents (Claude Code and others) working in this
repository. It is intentionally short.

## Start here

1. Read [`CONTRIBUTING.md`](CONTRIBUTING.md) — project layout, the build/test/
   lint/docs commands, the coding conventions, and the commit/PR expectations
   every change must follow.
2. Then read, only as the task needs them, the docs under [`docs/`](docs/):
   [`technical-design.md`](docs/technical-design.md) (architecture and the
   `internal/` packages — read before changing core behavior),
   [`installation.md`](docs/installation.md) (install / setup / distribution UX),
   [`read-only-mode.md`](docs/read-only-mode.md) (the write-safety posture), and
   [`releasing.md`](docs/releasing.md) (versioning, tagging, the release/CI
   workflows — read before cutting a release or touching `.github/workflows/`).

## What this is

`jenkins-cli` is an agent-facing CLI for a developer's Jenkins debugging
workflow: discover jobs and builds, read status / logs / stages / tests /
changes, and trigger or stop builds. It is a Go + Cobra CLI that mirrors the
architecture of the sibling `confluence-cli` / `bitbucket-cli` / `openobserve-cli`.

## Layout

- `cmd/jenkins-cli` — entry point; `cmd/gen-docs` — CLI reference generator.
- `internal/app` — one file per noun (job, build, queue, auth, config, doctor,
  skill); `root.go` assembles the tree; `context.go` holds the shared `appState`;
  `build_log.go` is the streaming `build log --follow`.
- `internal/apiclient` — the Jenkins HTTP surface and models. One file per
  resource (jobs, builds, console, pipeline, tests, changes, queue, writes);
  `paths.go` maps human job paths and build refs to Jenkins URLs; `models.go`
  normalizes Jenkins JSON (color → status); `readonly.go` blocks the writes.
- `internal/errors` — the `CLIError` model + exit-code map (0–11).
- `internal/output` — JSON / table / ndjson rendering, `{items,next,has_more}`.
- `internal/config`, `internal/auth` — layered config + keychain credentials.
- `internal/timeutil` — Jenkins epoch-millis → ISO instant + relative phrase.
- `skills/jenkins` — the companion Skill, embedded into the binary.

## Ground rules

- Run `make test` and `make build` before claiming a change is complete.
- stdout is data only; errors / notices / `--verbose` go to stderr.
- Never commit credentials, `.env`, or build artifacts.

## Discoverability — no dead-end inputs

**Every identifier a command accepts as input must be discoverable through
another command in this CLI.** A job path → `job list` / `job get`. A build
number → `build list`. A queue id → `queue list`. When you add a command or flag
that takes a new kind of input, also provide (or point its error `next_steps` at)
the command that lists values of that kind.

## When extending the control plane (P1)

Jenkins' control plane (create / configure jobs, credentials, nodes, CASC) is the
roadmap. New write commands must: add `--dry-run` (emit the would-be request via
a pure `*Plan` builder in `apiclient`, so it works even in read-only mode),
require `--yes` for destructive ops, route through `apiclient.NewReadOnly`
(override the new method to return `READONLY_BLOCKED`), fetch a CSRF crumb for
POSTs, and keep the `{items,has_more}` + structured-error contract.
