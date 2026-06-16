# @angelmsger/jenkins-cli

npm distribution wrapper for **jenkins-cli** — an agent-facing CLI for inspecting
[Jenkins](https://www.jenkins.io/) jobs and builds. Installing this package
downloads the prebuilt Go binary for your platform from the matching GitHub
Release.

```bash
npm install -g @angelmsger/jenkins-cli
jenkins-cli --help
```

If installed with `--ignore-scripts`, the binary is fetched lazily on first run.

See the [project README](https://github.com/AngelMsger/jenkins-cli) for usage.
