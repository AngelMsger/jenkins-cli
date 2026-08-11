# jenkins-cli command reference

This index is generated from the CLI command tree — do not edit it by
hand; run `make docs`. The full reference, with every flag and example,
is published at <https://angelmsger.github.io/jenkins-cli/cli/>.

## auth

| Command | Description |
| --- | --- |
| [`jenkins-cli auth`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-auth) | Log in, check identity and log out |
| [`jenkins-cli auth login`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-auth-login) | Store credentials for the active context (interactive) |
| [`jenkins-cli auth logout`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-auth-logout) | Remove the stored credential for the active context |
| [`jenkins-cli auth status`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-auth-status) | Show the active identity and verify connectivity |

## build

| Command | Description |
| --- | --- |
| [`jenkins-cli build`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build) | Inspect builds: status, logs, stages, tests, changes |
| [`jenkins-cli build artifacts`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-artifacts) | List a build's archived artifacts |
| [`jenkins-cli build changes`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-changes) | Show the SCM commits in a build |
| [`jenkins-cli build get`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-get) | Get one build: result, timing, trigger cause |
| [`jenkins-cli build list`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-list) | List a job's recent builds (newest first) |
| [`jenkins-cli build log`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-log) | Print a build's console output |
| [`jenkins-cli build stages`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-stages) | Show a Pipeline build's stage-by-stage status |
| [`jenkins-cli build stop`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-stop) | Abort a running build (write) |
| [`jenkins-cli build tests`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-build-tests) | Show a build's test results |

## config

| Command | Description |
| --- | --- |
| [`jenkins-cli config`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-config) | Set up and inspect configuration and contexts |
| [`jenkins-cli config contexts`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-config-contexts) | List configured contexts and which one is current |
| [`jenkins-cli config init`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-config-init) | Interactively configure a context and store credentials |
| [`jenkins-cli config show`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-config-show) | Show the resolved configuration with field provenance |
| [`jenkins-cli config use-context`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-config-use-context) | Set the current context |

## doctor

| Command | Description |
| --- | --- |
| [`jenkins-cli doctor`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-doctor) | Check configuration, credentials and connectivity |

## job

| Command | Description |
| --- | --- |
| [`jenkins-cli job`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-job) | Discover jobs and trigger builds |
| [`jenkins-cli job build`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-job-build) | Trigger a build of a job (write) |
| [`jenkins-cli job get`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-job-get) | Get one job, including parameters and last-build pointers |
| [`jenkins-cli job list`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-job-list) | List jobs (the discovery map) |

## queue

| Command | Description |
| --- | --- |
| [`jenkins-cli queue`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-queue) | Inspect and manage the build queue |
| [`jenkins-cli queue cancel`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-queue-cancel) | Cancel a queued build before it starts (write) |
| [`jenkins-cli queue get`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-queue-get) | Get one queue item |
| [`jenkins-cli queue list`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-queue-list) | List builds waiting in the queue |

## skill

| Command | Description |
| --- | --- |
| [`jenkins-cli skill`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill) | Install the companion Skill for coding agents (Claude Code, Codex, Grok Build) |
| [`jenkins-cli skill install`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill-install) | Deploy the embedded Skill into a coding agent's skills directory |
| [`jenkins-cli skill path`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill-path) | Print where the Skill would be installed, and whether it is |
| [`jenkins-cli skill show`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill-show) | Print the embedded SKILL.md to stdout |
| [`jenkins-cli skill status`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill-status) | Report whether the companion Skill is loaded and installed |
| [`jenkins-cli skill uninstall`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-skill-uninstall) | Remove the companion Skill from a coding agent's skills directory |

## version

| Command | Description |
| --- | --- |
| [`jenkins-cli version`](https://angelmsger.github.io/jenkins-cli/cli/#jenkins-cli-version) | Print version information |

