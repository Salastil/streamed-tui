package internal

import (
	"errors"
	"os/exec"
)

// openBrowser tries to open the embed URL in the system browser.
func openBrowser(link string) error {
	if link == "" {
		return errors.New("empty URL")
	}
	return exec.Command("xdg-open", link).Start()
}
