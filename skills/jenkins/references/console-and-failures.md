# Diagnosing a failed build

## 1. Select and pin the run

Use the build the user identified. If the request concerns the latest run,
inspect `last` first; it may be running or successful. `lastFailed` selects the
most recent failure, which can be older:

```bash
jenkins-cli build get my-app lastFailed
jenkins-cli build list my-app --limit 10
```

Capture `number` and `url` from the selected build. The examples below assume
that number is **128**; substitute the returned number throughout. Do not keep
using a moving permalink across stages, tests, logs and changes.

`build get` returns `result`, `building`, `started_at`, `started_ago`, duration,
node and trigger `causes`. A running build may not have a final result yet.

## 2. Localize the failure

```bash
jenkins-cli build stages my-app 128
jenkins-cli build tests my-app 128 --failed-only
```

A stage's `FAILED` status locates a failure; it does not establish its root
cause. `NOT_PIPELINE` means the stages endpoint returned 404, not that the job
is necessarily freestyle. Verify this same build exists and check `job get`
for its `kind`; stage reporting may be unavailable. Similarly, `NO_TEST_REPORT`
does not prove whether tests ran or passed. Continue with the selected run's
console when stage/test evidence is unavailable.

## 3. Read a bounded excerpt

Start with a one-shot log read. In Bash, preserve the CLI's failure status when
piping output:

```bash
set -o pipefail
jenkins-cli build log my-app 128 | tail -n 80
```

`tail` limits displayed lines, not the download: the CLI fetches all remaining
text from the supplied byte offset. There are no `--tail` or log `--limit`
flags. Expand the excerpt only when the missing context affects the diagnosis.
Inspect and redact sensitive values before including an excerpt in a reply. If
a local log file is needed, restrict its permissions and remove it after use.

For a running build, stderr includes `_notice.next_start`; reuse that returned
offset for the **same numeric build** on the next one-shot call. For example,
if the notice returned `500000`:

```bash
jenkins-cli build log my-app 128 --start 500000
```

Only use `--follow` when the user requested monitoring. Before starting, set a
host-side deadline, output budget and stop condition (completion, the requested
event, cancellation or deadline). `--timeout` bounds individual requests and
does not end the overall loop. Stop it with SIGINT/SIGTERM when the budget is
reached and report the last observed state; use bounded one-shot polling when
the host cannot supervise a stream. Never stop the Jenkins build merely to
stop watching it.

```bash
jenkins-cli build log my-app 128 --start 500000 --follow
```

## 4. Inspect changes without assuming causation

```bash
jenkins-cli build changes my-app 128
```

The changeset lists commits Jenkins reports for that run (id, author, message
and affected paths). Compare relevant diffs with the failure evidence before
identifying a suspected cause; an empty changeset does not establish that no
code or environment changed. Link the exact build and state any uncertainty.

## Build references

Read commands accept a number or `last` (default), `lastSuccessful`,
`lastFailed`, `lastCompleted`, `lastStable`. Use these selectors for discovery,
then pin `number`. See [pipelines-and-tests.md](pipelines-and-tests.md) for
queue handoff and test-report details.
