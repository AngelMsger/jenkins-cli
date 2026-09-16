# Pipelines, multibranch, and test results

## Multibranch and folders

Jenkins nests jobs inside **folders**, and a **multibranch** project holds one
child job per branch / PR. The CLI models this as a slash-separated path:

```
jenkins-cli job list                       # top-level jobs and folders
jenkins-cli job list --folder my-app       # the branches / PRs of a multibranch project
jenkins-cli job list --folder my-team --depth 2   # recurse a folder two levels
jenkins-cli job get my-app                 # the project plus its branch jobs under jobs[]
```

For "how is each branch doing", prefer `job list --folder <project>`: it returns
the branches as a flat list, each with its `last_build` snapshot (result, start
time, duration, building) — so you see every branch's situation in one call.
`job get <project>` gives the same branches nested under `jobs[]`, plus the
project's own parameters and health.

Each listed branch has a human `name` (e.g. `feature/login`) and a ready-to-use
`path` (e.g. `my-app/feature%2Flogin`). To drill into a branch, pass its `path`
— Jenkins encodes a slash in a branch name as `%2F`, and the listing has already
done that for you, so don't hand-encode or rebuild the path from `name`:

```
jenkins-cli job get my-app/main                  # the main branch job
jenkins-cli build get my-app/PR-42 last          # latest build of PR-42
jenkins-cli build stages my-app/feature%2Flogin last   # use the path verbatim
```

## Pipeline stages

`build stages <path> [ref]` returns the Pipeline run's stages from Jenkins'
`wfapi`, each with `status` and `duration`. This is the quickest way to see
which stage failed. Resolve `[ref]` to a numeric build first. A stages-endpoint
404 becomes `NOT_PIPELINE`; verify the selected build and job kind before
concluding that stage reporting is unsupported.

## Test results

```
jenkins-cli build get my-app lastCompleted                 # capture number (e.g. 128)
jenkins-cli build tests my-app 128 --failed-only            # just the failures
```

Returns `total` / `passed` / `failed` / `skipped`, then the individual cases.
`--failed-only` keeps just the failing/regressed cases — each with its
`class_name`, `name`, `error_details` and `error_stack` — which is the
high-signal view when triaging. `NO_TEST_REPORT` means the report endpoint
returned 404. Verify the same build exists and inspect its console; the error
alone cannot tell whether tests ran, passed or failed before publishing results.

## Triggering, stopping and cancelling

Use the write authorization and scope rules in the main Skill. Inspect `job get`
for supported parameters before triggering; use the exact running build number
or queue id for a stop/cancel. Preview the intended write with `--dry-run`,
then perform the authorized action once:

```bash
jenkins-cli job build my-app --param BRANCH=main --dry-run
jenkins-cli job build my-app --param BRANCH=main
jenkins-cli build stop my-app 128 --dry-run
jenkins-cli build stop my-app 128
jenkins-cli queue cancel 51 --dry-run
jenkins-cli queue cancel 51
```

These are independent examples, not a sequence to execute together. In a
configured read-only session, an authorized write needs `--allow-writes`; do
not add it when the user requested inspection only. If a write's outcome is
uncertain, inspect state before retrying to avoid triggering duplicate builds.

## Follow the triggered build

A successful `job build` returns `queue.queue_id` and `queue.queue_url`. Poll
that id with `queue get <id>` at a bounded interval (for example, every five
seconds for up to a minute):

- `cancelled: true`: the queued request was cancelled; stop monitoring it.
- `executable.number` and `executable.url`: the assigned build. Reuse the
  original job path and this number for `build get` and `build log`.
- No executable and not cancelled: report `why`/`blocked`/`stuck` as available,
  then wait within the polling budget. This is not evidence of a started build.
- A 404 or missing queue id: the handoff is unavailable. Report that limitation;
  never substitute `last` or assume success, cancellation or permission to
  trigger again.

The queue item's existing `url` remains the **job URL**, not its assigned
build URL. Once a build number is known, default to a one-shot status/log read.
Use the [console monitoring rules](console-and-failures.md#3-read-a-bounded-excerpt)
for an explicitly requested follow-up stream.
