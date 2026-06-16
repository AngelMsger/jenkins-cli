# Changelog

All notable changes to `jenkins-cli` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-06-17

### Fixed

- **npm package was uninstallable.** The launcher shim `build/npm/bin/jenkins-cli.js`
  — which `package.json`'s `bin` points at and which execs the downloaded
  platform binary — was missing from the package, so `npm i -g
  @angelmsger/jenkins-cli` produced a dead command (e.g. a failing Volta shim).
  The launcher is now included.

## [0.1.0] - 2026-06-17

Initial release — an agent-facing CLI for a developer's Jenkins debugging
workflow.

### Added

- **Job discovery.** `job list` (the high-signal map, `?tree=`-shaped) lists
  jobs, folders and multibranch projects with their type and normalized status;
  `--folder` lists inside a container and `--depth` recurses. `job get` returns
  one job's parameters, health, last-build pointers, and branch / PR child jobs.
- **Build inspection.** `build list` (history), `build get` (result, timing,
  node, trigger cause, parameters), `build log` (raw console text; `--follow`
  streams a running build via the progressive log until it finishes), `build
  stages` (Pipeline per-stage status — which stage failed), `build tests`
  (`--failed-only` for the failing cases), `build changes` (SCM commits) and
  `build artifacts`. A build reference is a number or a permalink keyword
  (`last`, `lastSuccessful`, `lastFailed`, `lastCompleted`, `lastStable`).
- **Builds: trigger & stop (writes).** `job build` triggers a build (with
  `--param KEY=VALUE`), `build stop` aborts a running one, and `queue cancel`
  drops a queued one. Each supports `--dry-run` (a credentials-free request
  preview), routes through the read-only wrapper (`READONLY_BLOCKED` unless
  `--allow-writes`), and attaches a CSRF crumb automatically.
- **Queue.** `queue list` and `queue get` show builds waiting to run and why.
- **The CLI owns Jenkins' footguns.** Human job paths (`folder/job/PR-12`) map to
  Jenkins' `/job/.../job/...` URLs; the `color` field becomes a stable `status`;
  epoch-millisecond timestamps return as both an ISO instant and a relative
  phrase; freestyle vs Pipeline changeset shapes are handled transparently.
- **Auth.** Jenkins username + API token (recommended) or password, encoded as
  HTTP Basic, stored in the OS keychain with a `0600` file fallback, plus
  `JENKINS_*` environment passthrough for headless / agent use. `auth status`
  reports the authenticated user via Jenkins `/me`.
- **Setup & diagnostics.** `config init` (interactive TUI with `--pretty`, plain
  pipe-friendly wizard otherwise), `config show` / `contexts` / `use-context`,
  multiple kubectl-style named contexts, `auth login` / `status` / `logout`, and
  `doctor`.
- **Agent-friendly output.** JSON by default, a `{items, has_more}` list
  envelope, `--format json|table|ndjson`, `--fields` projection, and structured
  errors (`category` / `code` / `hint` / `next_steps`) mapped to stable exit
  codes (0–11). Common LLM argv slips (`--jobName`, `--limit100`) are corrected
  before parsing and echoed as a `_notice` on stderr.
- **Companion Skill.** A `jenkins` Skill embedded in the binary and deployed with
  `skill install`, with `references/` covering getting-started, diagnosing a
  failed build, pipelines & tests, and errors / exit codes.
- **Distribution.** npm (`@angelmsger/jenkins-cli`), `go install`, prebuilt
  release binaries and `make install`. A generated CLI reference (`docs/cli/`)
  and a GitHub Pages landing page.

[Unreleased]: https://github.com/AngelMsger/jenkins-cli/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/AngelMsger/jenkins-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/AngelMsger/jenkins-cli/releases/tag/v0.1.0
