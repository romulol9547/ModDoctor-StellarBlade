//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/jchv/go-webview2"
)

// the window must be created on the program's main thread
func init() { runtime.LockOSThread() }

// showUI opens the program's own window. Returns false if WebView2 is missing.
func showUI(url string) bool {
	data := ""
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		data = filepath.Join(la, "ModDoctorStellarBlade")
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  data,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "ModDoctor Stellar Blade", Width: 1100, Height: 820, Center: true,
		},
	})
	if w == nil {
		return false
	}
	mainWindow = uintptr(w.Window())
	w.Bind("mdOpen", func(u string) { openBrowser(u) }) // Nexus links open in the normal browser
	w.Navigate(url)
	w.Run()
	w.Destroy()
	return true
}
