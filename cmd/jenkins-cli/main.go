// Command jenkins-cli lets Coding Agents inspect Jenkins for a developer's
// debugging workflow: discover jobs and builds, read status, console logs,
// pipeline stages, test failures and SCM changes, and trigger or stop builds —
// all with agent-friendly JSON output and structured errors.
package main

import (
	"os"

	"github.com/angelmsger/jenkins-cli/internal/app"
)

func main() {
	os.Exit(app.Execute())
}
