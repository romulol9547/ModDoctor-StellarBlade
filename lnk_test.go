package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func fakeLnk(ansiPath, uniRel string) []byte {
	b := make([]byte, 0x4C)
	binary.LittleEndian.PutUint32(b[0:], 0x4C)
	binary.LittleEndian.PutUint32(b[0x14:], 0x2|0x8|0x80) // LinkInfo, RelativePath, Unicode
	li := make([]byte, 0x1C)
	binary.LittleEndian.PutUint32(li[4:], 0x1C)
	binary.LittleEndian.PutUint32(li[8:], 1)
	binary.LittleEndian.PutUint32(li[0x10:], 0x1C)
	// Windows-1252 path with "ô"
	for _, r := range ansiPath {
		li = append(li, byte(r))
	}
	li = append(li, 0)
	binary.LittleEndian.PutUint32(li[0:], uint32(len(li)))
	b = append(b, li...)
	u := utf16.Encode([]rune(uniRel))
	b = append(b, byte(len(u)), 0)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

func TestLnk(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "Stellar Blade.lnk")
	os.WriteFile(p, fakeLnk(`D:\Jogos\Rômulo\Stellar Blade\SB.exe`, `..\..\Jogos\Rômulo\Stellar Blade\SB.exe`), 0644)
	got := shortcutTargetsOf(p)
	t.Logf("%q", got)
	ok := false
	for _, g := range got {
		if g == `D:\Jogos\Rômulo\Stellar Blade\SB.exe` && looksLikeGameExe(g) {
			ok = true
		}
	}
	if !ok {
		t.Fatal("path not found")
	}
	u := filepath.Join(d, "SB.url")
	os.WriteFile(u, []byte("[InternetShortcut]\r\nURL=steam://rungameid/3489700\r\n"), 0644)
	t.Logf("%q", shortcutTargetsOf(u))
	if g := shortcutTargetsOf(u); len(g) != 1 || cleanPath(g[0]) == "" {
		t.Fatal("url")
	}
	if cleanPath(`"C:\Games\SB.exe",0`) != `C:\Games\SB.exe` || cleanPath(`C:\Games\SB.exe,0`) != `C:\Games\SB.exe` {
		t.Fatal("clean")
	}
}
