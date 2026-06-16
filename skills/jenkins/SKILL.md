---
name: jenkins
version: 0.1.0
description: "Inspect Jenkins for a developer's debugging workflow from the command line: discover jobs, folders and multibranch branches/PRs; read build status and history; find why a build failed — which pipeline stage broke, the failing test cases, the console log, and the SCM commits in a build; list and inspect the build queue; and trigger or stop builds. Agent-friendly JSON with structured errors; works with any Jenkins instance. Use this skill when the user mentions Jenkins or CI, gives a Jenkins job or build URL, or asks: what is the build status, did the latest build pass or fail, when did it last succeed/fail, why is the build red / failing, show the console log / build output, which stage failed, which tests are failing, what changed in a build, is anything queued — or to trigger / rebuild / start / stop / abort / cancel a build. Set up with `jenkins-cli config init`, or JENKINS_URL / JENKINS_USER / JENKINS_TOKEN env vars. Inspection is read-only; build trigger and stop are writes that need --allow-writes."
metadata:
  requires:
    bins: ["jenkins-cli"]
  cliHelp: "jenkins-cli --help; jenkins-cli job --help; jenkins-cli build --help"
---

# jenkins

`jenkins-cli` inspects Jenkins for a developer's debugging workflow: discover
jobs, read build status and history, and find out *why* a build failed — the
failing stage, failing tests, console log and SCM changes. Output is JSON by
default; errors are JSON on stderr with a `category`, a `hint` and `next_steps`.
Reads are unrestricted; the two write commands (`job build`, `build stop`) and
`queue cancel` are gated by read-only mode.

## Golden rule — discover before you query

Don't invent a job path or a build number. Job paths are `folder/job[/branch]`
(each segment is a folder, job or multibranch branch). List first:

1. **Jobs** — `job list` shows jobs at the instance root; `--folder <path>`
   lists inside a folder, or the branches / PRs of a multibranch project;
   `--depth N` recurses. Every job carries a `last_build` snapshot (result,
   start time, duration, whether it's building), so one listing already shows
   each branch's build situation — no per-branch follow-up needed.
2. **A job** — `job get <path>` shows its parameters, health, last-build
   pointers, and (for multibranch) its branch / PR child jobs.
3. **Builds** — `build list <path>` shows the build history; a build reference
   is a number or a permalink keyword: `last`, `lastSuccessful`, `lastFailed`,
   `lastCompleted`, `lastStable` (default `last`).

## Decision tree

- **All branches of a multibranch project, with each one's build status** →
  `job list --folder <project>`. Each branch comes back with its `last_build`
  snapshot (result, when it ran, duration, whether it's building), so you answer
  "which branches are red / stale / still running" in one call — don't query
  branches one at a time.
- **What's the status / did it pass / when did it last succeed or fail** →
  `job get <path>` (last* pointers) or `build get <path> [ref]`. Use
  `build get <path> lastFailed` for the most recent failure.
- **Why did it fail / which stage broke** → `build stages <path> [ref]` for a
  Pipeline job (per-stage status), then `build log <path> [ref]` for the output.
  See [console-and-failures.md](references/console-and-failures.md).
- **Which tests are failing** → `build tests <path> [ref] --failed-only`.
  See [pipelines-and-tests.md](references/pipelines-and-tests.md).
- **Show the console / build output** → `build log <path> [ref]`; add `--follow`
  to stream a running build until it finishes (Ctrl-C to stop).
- **What changed in this build** → `build changes <path> [ref]` (commits).
- **What's waiting to run** → `queue list`, then `queue get <id>`.
- **Trigger / rebuild a job** → `job build <path> [--param K=V ...]` (a write;
  preview with `--dry-run`, run with `--allow-writes` if read-only).
- **Stop / abort a running build** → `build stop <path> <ref>` (a write).
- Nothing configured yet / auth fails →
  [getting-started.md](references/getting-started.md).
- Anything fails → read the error's `next_steps`. See
  [errors-and-exit-codes.md](references/errors-and-exit-codes.md).

## Guardrails

- **Reference real names only.** If you didn't get a job path from `job list` /
  `job get`, don't put it in a command. Paths are case-sensitive.
- **Drill in with the `path` field, not `name`.** Each listed job has a
  human-readable `name` (e.g. `feature/login`) and a ready-to-use `path`
  (e.g. `my-app/feature%2Flogin`, with slashes in a branch name already
  encoded). Pass the `path` to `job get` / `build …`; don't hand-encode or
  rebuild it from `name`.
- **Builds are big — start narrow.** Use `job get` / `build get` (compact) before
  `build log` (the full console). For a running build, `build log` returns what
  exists so far and tells you the offset to resume from; `--follow` to stream.
- **Writes are explicit.** `job build`, `build stop` and `queue cancel` mutate
  Jenkins. Preview with `--dry-run`; in read-only mode they are blocked until you
  pass `--allow-writes`.
- **Status is normalized.** Jenkins encodes outcome in a job's `color`; the CLI
  maps it to `status` (success / failure / unstable / building / disabled /
  not_built). Timestamps come back as both an ISO instant and a relative phrase.

## Commands

```
jenkins-cli job list [--folder <path>] [--depth N]   # discover jobs (the map)
jenkins-cli job get <path>                           # one job: params, branches, last builds
jenkins-cli build list <path> [--limit N]            # build history (newest first)
jenkins-cli build get <path> [lastFailed]            # one build: result, timing, cause
jenkins-cli build log <path> [ref] [--follow]        # console output (stream with --follow)
jenkins-cli build stages <path> [ref]                # Pipeline stage status — which stage failed
jenkins-cli build tests <path> [ref] --failed-only   # failing test cases
jenkins-cli build changes <path> [ref]               # SCM commits in the build
jenkins-cli build artifacts <path> [ref]             # archived artifacts
jenkins-cli queue list                               # builds waiting to run
jenkins-cli job build <path> --param K=V --dry-run   # trigger a build (write; preview first)
jenkins-cli build stop <path> <number>               # abort a running build (write)
jenkins-cli auth status                              # who am I / can I reach the server
jenkins-cli doctor                                   # diagnose config / creds / connectivity
```

## Agent-facing conventions

- stdout is data only; diagnostics, notices and errors go to stderr.
- Exit codes are stable and categorized (0 ok, 2 usage, 3 config, 4 auth,
  5 permission, 6 not found, …); see
  [errors-and-exit-codes.md](references/errors-and-exit-codes.md).
- Lists come back as `{ "items": [...], "has_more": false }`.
- `--fields a,b.c` projects output to just those dot-paths to save tokens.
- `build log` prints raw console text (grep-able); everything else is JSON.
