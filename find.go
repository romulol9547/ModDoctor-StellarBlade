package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const steamAppID = "3489700" // Stellar Blade on Steam

// findGame tries every automatic method, in order. Works for Steam, Epic, GOG,
// other installers and copies of the game folder. Returns folder + method name.
func findGame() (string, string) {
	hints := windowsHints() // one PowerShell call: installed programs, running game, shortcuts, run history
	try := func(tag string) string {
		for _, h := range hints {
			if h.tag == tag {
				if g := normalizeGame(cleanPath(h.val)); g != "" {
					return g
				}
			}
		}
		return ""
	}
	// 1) Windows list of installed programs (any store or installer)
	if g := try("uninstall"); g != "" {
		return g, "registry"
	}
	// 2) Steam libraries (any disk)
	if g := findInSteamLibraries(""); g != "" {
		return g, "steamlib"
	}
	// 3) Epic Games launcher
	if g := findInEpic(); g != "" {
		return g, "launcher"
	}
	// 4) game open right now
	if g := try("process"); g != "" {
		return g, "process"
	}
	// 5) Desktop / Start menu shortcuts (direct .exe or steam://)
	for _, h := range hints {
		if h.tag != "shortcut" {
			continue
		}
		if m := regexp.MustCompile(`rungameid/(\d+)`).FindStringSubmatch(h.val); m != nil {
			if g := findInSteamLibraries(m[1]); g != "" {
				return g, "shortcut"
			}
			continue
		}
		if looksLikeGameExe(h.val) {
			if g := normalizeGame(cleanPath(h.val)); g != "" {
				return g, "shortcut"
			}
		}
	}
	// 6) programs Windows remembers you opened
	for _, h := range hints {
		if h.tag == "history" && looksLikeGameExe(h.val) {
			if g := normalizeGame(cleanPath(h.val)); g != "" {
				return g, "history"
			}
		}
	}
	// 7) search the disks
	if g := scanDrives(); g != "" {
		return g, "disk"
	}
	return "", ""
}

type hint struct{ tag, val string }

func looksLikeGameExe(p string) bool {
	low := strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	return strings.Contains(low, "sb-win64-shipping.exe") || strings.HasSuffix(low, `\sb.exe`) ||
		(strings.Contains(low, "stellar") && strings.Contains(low, ".exe"))
}

// "C:\x\game.exe",0  ->  C:\x\game.exe
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, `"`) {
		if i := strings.Index(p[1:], `"`); i >= 0 {
			return p[1 : i+1]
		}
	}
	if i := strings.LastIndex(p, ","); i > 0 && !strings.Contains(p[i:], `\`) {
		p = p[:i]
	}
	return strings.Trim(p, `" `)
}

func findInEpic() string {
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	files, _ := filepath.Glob(filepath.Join(pd, "Epic", "EpicGamesLauncher", "Data", "Manifests", "*.item"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m struct{ DisplayName, InstallLocation string }
		if json.Unmarshal(data, &m) == nil && strings.Contains(strings.ToLower(m.DisplayName), "stellar blade") {
			if g := normalizeGame(m.InstallLocation); g != "" {
				return g
			}
		}
	}
	return ""
}

func steamRoots() []string {
	var roots []string
	add := func(p string) {
		if p == "" {
			return
		}
		p = filepath.Clean(strings.ReplaceAll(p, "/", string(filepath.Separator)))
		for _, r := range roots {
			if strings.EqualFold(r, p) {
				return
			}
		}
		if isDir(p) {
			roots = append(roots, p)
		}
	}
	add(regValue(`HKCU\Software\Valve\Steam`, "SteamPath"))
	add(regValue(`HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"))
	add(regValue(`HKLM\SOFTWARE\Valve\Steam`, "InstallPath"))
	add(`C:\Program Files (x86)\Steam`)
	add(`C:\Program Files\Steam`)
	if h := os.Getenv("STEAM_ROOT_TEST"); h != "" { // used only by tests
		add(h)
	}
	return roots
}

var (
	reVdfPath  = regexp.MustCompile(`"path"\s+"([^"]+)"`)
	reAcfName  = regexp.MustCompile(`"name"\s+"([^"]*)"`)
	reAcfDir   = regexp.MustCompile(`"installdir"\s+"([^"]*)"`)
	reAcfAppID = regexp.MustCompile(`"appid"\s+"(\d+)"`)
)

// libraries = every "steamapps" folder Steam knows about
func steamLibraries() []string {
	var libs []string
	seen := map[string]bool{}
	add := func(p string) {
		sa := filepath.Join(p, "steamapps")
		k := strings.ToLower(sa)
		if !seen[k] && isDir(sa) {
			seen[k] = true
			libs = append(libs, sa)
		}
	}
	for _, root := range steamRoots() {
		add(root)
		for _, f := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			for _, m := range reVdfPath.FindAllStringSubmatch(string(data), -1) {
				add(filepath.FromSlash(strings.ReplaceAll(m[1], `\\`, `\`)))
			}
		}
	}
	return libs
}

// appID "" = look for Stellar Blade by id or by name
func findInSteamLibraries(appID string) string {
	for _, sa := range steamLibraries() {
		files, _ := filepath.Glob(filepath.Join(sa, "appmanifest_*.acf"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			s := string(data)
			id, name, dir := sub(reAcfAppID, s), sub(reAcfName, s), sub(reAcfDir, s)
			match := false
			if appID != "" {
				match = id == appID
			} else {
				match = id == steamAppID || strings.Contains(strings.ToLower(name), "stellar blade")
			}
			if match && dir != "" {
				if g := normalizeGame(filepath.Join(sa, "common", dir)); g != "" {
					return g
				}
			}
		}
		if appID == "" { // manifest missing: just look at common\StellarBlade
			if g := normalizeGame(filepath.Join(sa, "common", "StellarBlade")); g != "" {
				return g
			}
		}
	}
	return ""
}

// scanDrives looks on every disk (a few levels deep) for the game folder,
// whatever its name is. Skips Windows system folders.
var reStellar = regexp.MustCompile(`(?i)stellar\s*blade`)

func scanDrives() string {
	skip := map[string]bool{"windows": true, "$recycle.bin": true, "system volume information": true, "programdata": true,
		"appdata": true, "windowsapps": true, "$windows.~bt": true, "$winreagent": true, "recovery": true, "node_modules": true}
	for _, d := range drives() {
		visited := 0
		level := []string{d}
		for depth := 0; depth <= 5 && len(level) > 0 && visited < 60000; depth++ {
			var next []string
			for _, dir := range level {
				des, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, e := range des {
					if !e.IsDir() || skip[strings.ToLower(e.Name())] || strings.HasPrefix(e.Name(), ".") {
						continue
					}
					visited++
					c := filepath.Join(dir, e.Name())
					if isGameRoot(c) {
						return c
					}
					if reStellar.MatchString(e.Name()) {
						if g := normalizeGame(c); g != "" {
							return g
						}
					}
					next = append(next, c)
				}
			}
			level = next
		}
	}
	return ""
}

func isGameRoot(d string) bool {
	return isDir(filepath.Join(d, "SB", "Binaries")) || isDir(filepath.Join(d, "SB", "Content"))
}

// normalizeGame accepts the game folder, the .exe, any folder inside the game
// (like ue4ss) or a folder above it, and returns the folder that contains "SB".
func normalizeGame(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return ""
	}
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		p = filepath.Dir(p)
	}
	// up
	d := filepath.Clean(p)
	for i := 0; i < 8; i++ {
		if isGameRoot(d) {
			return d
		}
		if strings.EqualFold(filepath.Base(d), "SB") && isGameRoot(filepath.Dir(d)) {
			return filepath.Dir(d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	// down
	visited := 0
	var found string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if found != "" || depth > 4 || visited > 20000 {
			return
		}
		visited++
		des, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range des {
			if found != "" {
				return
			}
			if !e.IsDir() {
				continue
			}
			n := strings.ToLower(e.Name())
			if n == "windows" || n == "$recycle.bin" || n == "programdata" || strings.HasPrefix(n, ".") {
				continue
			}
			c := filepath.Join(dir, e.Name())
			if isGameRoot(c) {
				found = c
				return
			}
			walk(c, depth+1)
		}
	}
	walk(filepath.Clean(p), 0)
	return found
}

func defaultCrashDir() string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		return ""
	}
	c := filepath.Join(la, "SB", "Saved", "Crashes")
	if isDir(c) {
		return c
	}
	return ""
}

func isDir(p string) bool { i, err := os.Stat(p); return err == nil && i.IsDir() }

func sub(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
