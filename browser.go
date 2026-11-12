package main

import (
	"os"
	"os/exec"
	"runtime"
)

// openBrowser opens u in the user's browser where there is one to open:
// $BROWSER, else the opener of the system, which on Linux needs a display.
// It reports whether it started one.
func openBrowser(u string) bool {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("BROWSER") != "":
		cmd = exec.Command(os.Getenv("BROWSER"), u)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", u)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "":
		cmd = exec.Command("xdg-open", u)
	default:
		return false
	}
	if cmd.Start() != nil {
		return false
	}
	go cmd.Wait()
	return true
}
