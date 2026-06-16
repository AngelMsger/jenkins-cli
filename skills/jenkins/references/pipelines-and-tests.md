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
which stage failed. It applies to Pipeline / multibranch jobs only; a freestyle
job returns `NOT_PIPELINE`.

## Test results

```
jenkins-cli build tests my-app lastCompleted               # totals + every case
jenkins-cli build tests my-app lastCompleted --failed-only  # just the failures
```

Returns `total` / `passed` / `failed` / `skipped`, then the individual cases.
`--failed-only` keeps just the failing/regressed cases — each with its
`class_name`, `name`, `error_details` and `error_stack` — which is the
high-signal view when triaging. `NO_TEST_REPORT` means the build published no
test results (no JUnit step, or it failed before tests ran) — read the console
log instead.

## Triggering and stopping builds

Triggering and stopping are **writes**. Preview them first with `--dry-run`
(which sends nothing and needs no special permission):

```
jenkins-cli job build my-app --param BRANCH=main --param CLEAN=true --dry-run
jenkins-cli job build my-app --param BRANCH=main            # actually trigger
jenkins-cli build stop my-app 128                           # abort build #128
jenkins-cli queue cancel 51                                 # drop a queued build
```

In a read-only session these are blocked (`READONLY_BLOCKED`) until you add
`--allow-writes`. A successful `job build` returns the queue item the build was
scheduled into; poll it with `queue get <id>`, or watch the build with
`build log --follow` once it starts.
