# Diagnosing a failed build

The fastest path from "the build is red" to a root cause.

## 1. Find the failing build

```
jenkins-cli build get my-app lastFailed     # the most recent failure
jenkins-cli build list my-app --limit 10     # or scan recent history
```

`build get` returns the `result` (FAILURE / UNSTABLE / ABORTED), when it started
(`started_at` plus a relative `started_ago`), how long it ran, the node it ran
on, and what triggered it (`causes`).

## 2. Localize the failure (Pipeline jobs)

```
jenkins-cli build stages my-app lastFailed
```

Each stage comes back with its `status` and `duration`. Scan for the stage whose
status is `FAILED` — that tells you *where* it broke (checkout? build? a specific
test stage? deploy?) before you read a line of log. `NOT_PIPELINE` means it's a
freestyle job; skip straight to the console.

## 3. Read the console output

```
jenkins-cli build log my-app lastFailed                  # whole log (plain text)
jenkins-cli build log my-app lastFailed | tail -n 80     # the tail is usually enough
jenkins-cli build log my-app lastFailed --start 500000   # resume from an offset
```

`build log` prints raw console text to stdout, so pipe it through `grep`/`tail`.
For a build that is still running, it prints what exists so far and emits a
`_notice` on stderr with `next_start` (the byte offset to resume from). To watch
a running build to completion:

```
jenkins-cli build log my-app --follow      # streams new output until done; Ctrl-C to stop
```

## 4. If tests failed

```
jenkins-cli build tests my-app lastFailed --failed-only
```

See [pipelines-and-tests.md](pipelines-and-tests.md).

## 5. What went into the build

```
jenkins-cli build changes my-app lastFailed
```

Lists the SCM commits the build picked up (id, author, message, affected paths)
— useful for "which change broke it" once you know the build is the first red
one.

## Build references

Everywhere a `[ref]` is accepted you can pass a build number or a permalink
keyword: `last` (default), `lastSuccessful`, `lastFailed`, `lastCompleted`,
`lastStable`. So `build log my-app lastFailed` always targets the latest failure
without you needing its number.
