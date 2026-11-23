package app

import (
	"os/exec"
	"runtime"
)

// openBrowser hands a Jenkins URL to the desktop. Some Linux launchers wait
// until the browser exits, so never wait for it before prompting for the token.
func openBrowser(page string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", page)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", page)
	default:
		cmd = exec.Command("xdg-open", page)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
