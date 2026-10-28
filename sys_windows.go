//go:build windows

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Everything here talks to Windows directly. No PowerShell, no scripts,
// no helper programs: antivirus tools treat those as suspicious.

var (
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procShellExecute      = shell32.NewProc("ShellExecuteW")
	procBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
	procGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree     = ole32.NewProc("CoTaskMemFree")
	procMessageBox        = user32.NewProc("MessageBoxW")
	procSetConsoleTitle   = kernel32.NewProc("SetConsoleTitleW")

	mainWindow uintptr // set by the UI so dialogs open on top of it
)

func setupConsole() {
	procSetConsoleTitle.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("ModDoctor Stellar Blade"))))
}

func rootKey(name string) (registry.Key, bool) {
	switch name {
	case "HKLM":
		return registry.LOCAL_MACHINE, true
	case "HKCU":
		return registry.CURRENT_USER, true
	}
	return 0, false
}

// regValue reads a text value like `HKLM\SOFTWARE\...` + name. "" if missing.
func regValue(path, name string) string {
	i := strings.Index(path, `\`)
	root, ok := rootKey(path[:i])
	if !ok {
		return ""
	}
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		k, err := registry.OpenKey(root, path[i+1:], registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue(name)
		k.Close()
		if err == nil && v != "" {
			return v
		}
	}
	return ""
}

var reStellarName = regexp.MustCompile(`(?i)stellar\s*blade`)

// windowsHints collects every place that may point to the game:
// installed programs, the running game, shortcuts, run history.
func windowsHints() []hint {
	var res []hint
	add := func(tag, v string) {
		if v = strings.TrimSpace(v); v != "" {
			res = append(res, hint{tag, v})
		}
	}

	// installed programs (any store or installer) + GOG
	type src struct {
		root registry.Key
		path string
		view uint32
	}
	for _, s := range []src{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_32KEY},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, 0},
		{registry.LOCAL_MACHINE, `SOFTWARE\GOG.com\Games`, registry.WOW64_32KEY},
	} {
		k, err := registry.OpenKey(s.root, s.path, registry.ENUMERATE_SUB_KEYS|s.view)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			sk, err := registry.OpenKey(s.root, s.path+`\`+n, registry.QUERY_VALUE|s.view)
			if err != nil {
				continue
			}
			disp, _, _ := sk.GetStringValue("DisplayName")
			gog, _, _ := sk.GetStringValue("gameName")
			if reStellarName.MatchString(disp) || reStellarName.MatchString(gog) || n == "Steam App "+steamAppID {
				for _, val := range []string{"InstallLocation", "path", "DisplayIcon", "UninstallString"} {
					if v, _, err := sk.GetStringValue(val); err == nil {
						add("uninstall", v)
					}
				}
			}
			sk.Close()
		}
	}

	// game open right now
	for _, p := range runningGamePaths() {
		add("process", p)
	}

	// desktop and start menu shortcuts
	var dirs []string
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Desktop, windows.FOLDERID_PublicDesktop, windows.FOLDERID_StartMenu, windows.FOLDERID_CommonStartMenu} {
		if p, err := windows.KnownFolderPath(id, 0); err == nil {
			dirs = append(dirs, p)
		}
	}
	for _, d := range dirs {
		count := 0
		filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			if count++; count > 3000 {
				return filepath.SkipAll
			}
			low := strings.ToLower(p)
			if strings.HasSuffix(low, ".lnk") || strings.HasSuffix(low, ".url") {
				for _, t := range shortcutTargetsOf(p) {
					add("shortcut", t)
				}
			}
			return nil
		})
	}

	// programs Windows remembers you opened
	for _, path := range []string{
		`Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Compatibility Assistant\Store`,
		`Software\Classes\Local Settings\Software\Microsoft\Windows\Shell\MuiCache`,
	} {
		k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(-1)
		k.Close()
		for _, n := range names {
			n = strings.TrimSuffix(strings.TrimSuffix(n, ".FriendlyAppName"), ".ApplicationCompany")
			if looksLikeGameExe(n) {
				add("history", n)
			}
		}
	}
	return res
}

// runningGamePaths returns the .exe path of a running Stellar Blade, if any
func runningGamePaths() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []string
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if name != "sb-win64-shipping.exe" && name != "sb.exe" {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, 1024)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			out = append(out, windows.UTF16ToString(buf[:n]))
		}
		windows.CloseHandle(h)
	}
	return out
}

// pickFolder shows the Windows folder dialog (last resort)
func pickFolder() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	defer windows.CoUninitialize()
	type browseInfo struct {
		Owner       uintptr
		Root        uintptr
		DisplayName *uint16
		Title       *uint16
		Flags       uint32
		Callback    uintptr
		LParam      uintptr
		Image       int32
	}
	name := make([]uint16, 260)
	bi := browseInfo{
		Owner:       mainWindow,
		DisplayName: &name[0],
		Title:       windows.StringToUTF16Ptr("Stellar Blade"),
		Flags:       0x0001 | 0x0040, // only folders, new style
	}
	pidl, _, _ := procBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 1024)
	if r, _, _ := procGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func drives() []string {
	var ds []string
	mask, _ := windows.GetLogicalDrives()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 || i < 2 { // skip A: and B:
			continue
		}
		d := string(rune('A'+i)) + `:\`
		if t := windows.GetDriveType(windows.StringToUTF16Ptr(d)); t == windows.DRIVE_FIXED || t == windows.DRIVE_REMOVABLE {
			ds = append(ds, d)
		}
	}
	return ds
}

// openBrowser opens a web address in the user's normal browser
func openBrowser(url string) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return
	}
	procShellExecute.Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("open"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(url))), 0, 0, 1)
}

func messageBox(title, text string) {
	procMessageBox.Call(mainWindow,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(text))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(title))), 0x40)
}
