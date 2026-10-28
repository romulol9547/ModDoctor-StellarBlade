# ModDoctor Stellar Blade

Free tool for Stellar Blade (PC) mods. It finds the game folder by itself, lists the installed mods (UE4SS script mods, pak mods, CNS outfits), warns about conflicts and reads the UE4SS log and the game's crash reports.

Nexus Mods page: https://www.nexusmods.com/stellarblade/mods/4066

**Reads only.** It never changes, moves or deletes any file. It never connects to the internet (the report is served on 127.0.0.1 to its own window). No PowerShell, no scripts, no helper programs.

## What is in the release zip

- `ModDoctor Stellar Blade.exe` - built from this source code (Go)
- `WebView2Loader.dll` - unmodified file from Microsoft's WebView2 SDK (signed by Microsoft), used to draw the window

## Source files

| File | What it does |
|---|---|
| `main.go` | starts a local web server on 127.0.0.1 (random port + random token), serves `ui.html`, read-only file API limited to the game folder and the crash folder |
| `find.go` | finds the game: Windows installed-programs list, Steam libraries, Epic manifests, running game, shortcuts, Windows run history, disk search |
| `lnk.go` | reads the target path written inside `.lnk` / `.url` shortcuts |
| `sys_windows.go` | Windows calls (registry read, process list, folder dialog, open link in browser) |
| `ui_windows.go` | opens the program window (WebView2) |
| `ui.html` | the report page (all checks run here, in JavaScript) |
| `third_party/go-webview2` | copy of github.com/jchv/go-webview2 (MIT) with in-memory DLL loading removed |

## How to build

Requirements: Go 1.22 or newer.

```
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0
goversioninfo -64 -o resource_windows_amd64.syso versioninfo.json
GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-H windowsgui" -o "ModDoctor Stellar Blade.exe" .
```

On Windows (cmd): `set GOOS=windows` and `set GOARCH=amd64` before `go build`.

Then put `WebView2Loader.dll` (x64) from the Microsoft WebView2 SDK (NuGet package `Microsoft.Web.WebView2`, folder `build/native/x64`) next to the .exe.

Tests: `go test ./...`

Author: romulol95
