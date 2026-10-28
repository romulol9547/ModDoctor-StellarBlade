//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
)

// Non-Windows versions exist only so the program can be tested on Linux.
func setupConsole()                     {}
func regValue(path, name string) string { return "" }
func windowsHints() []hint { // test format: tag|value;tag|value
	var res []hint
	for _, x := range strings.Split(os.Getenv("HINTS_TEST"), ";") {
		if i := strings.Index(x, "|"); i > 0 {
			res = append(res, hint{x[:i], x[i+1:]})
		}
	}
	return res
}
func pickFolder() string { return os.Getenv("PICK_TEST") }
func drives() []string {
	if s := os.Getenv("DRIVES_TEST"); s != "" {
		return strings.Split(s, ";")
	}
	return nil
}
func openBrowser(url string) { exec.Command("xdg-open", url).Start() }

func showUI(url string) bool        { return false }
func messageBox(title, text string) {}
