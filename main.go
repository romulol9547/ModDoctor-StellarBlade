// Mod Doctor - finds Stellar Blade by itself and shows a mod report in the browser.
// Read-only: it never changes any game file.
package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const version = "0.2.2"

//go:embed ui.html
var uiHTML []byte

type state struct {
	mu     sync.Mutex
	Game   string `json:"game"`
	Crash  string `json:"crash"`
	Method string `json:"method"`
	Ver    string `json:"version"`
}

var (
	st       = &state{Ver: version}
	token    string
	lastPing = time.Now()
	pinged   bool
	pingMu   sync.Mutex
)

func main() {
	gameFlag := flag.String("game", "", "game folder (skip auto-detection)")
	crashFlag := flag.String("crash", "", "crash reports folder")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser")
	detectOnly := flag.Bool("detect", false, "only print where the game is and exit")
	flag.Parse()
	setupConsole()

	fmt.Println("ModDoctor Stellar Blade v" + version)
	fmt.Println("Procurando o jogo... / Looking for the game...")

	if *gameFlag != "" {
		if g := normalizeGame(*gameFlag); g != "" {
			st.Game, st.Method = g, "flag"
		}
	} else {
		st.Game, st.Method = findGame()
	}
	if *crashFlag != "" {
		st.Crash = *crashFlag
	} else {
		st.Crash = defaultCrashDir()
	}
	if st.Game != "" {
		fmt.Println("Jogo encontrado / Game found: " + st.Game)
	} else {
		fmt.Println("Jogo nao encontrado automaticamente / Game not found automatically")
	}

	if *detectOnly {
		fmt.Println("method=" + st.Method + " crash=" + st.Crash)
		return
	}

	b := make([]byte, 16)
	rand.Read(b)
	token = hex.EncodeToString(b)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("Erro / Error:", err)
		messageBox("ModDoctor Stellar Blade", "Erro / Error: "+err.Error())
		return
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", ln.Addr().(*net.TCPAddr).Port, token)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(uiHTML)
	})
	mux.HandleFunc("/api/info", auth(apiInfo))
	mux.HandleFunc("/api/list", auth(apiList))
	mux.HandleFunc("/api/stat", auth(apiStat))
	mux.HandleFunc("/api/file", auth(apiFile))
	mux.HandleFunc("/api/pick", auth(apiPick))
	mux.HandleFunc("/api/ping", auth(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))

	fmt.Println()
	fmt.Println("O relatorio vai abrir no navegador. Se nao abrir, copie este endereco:")
	fmt.Println("The report will open in your browser. If it does not, copy this address:")
	fmt.Println("  " + url)
	fmt.Println()
	fmt.Println("Deixe esta janela aberta enquanto usa. Feche para sair.")
	fmt.Println("Keep this window open while you use it. Close it to exit.")

	go watchdog()
	go http.Serve(ln, mux)
	if !*noBrowser {
		if showUI(url) { // own window; closing it closes the program
			os.Exit(0)
		}
		openBrowser(url) // fallback: no WebView2 on this PC
	}
	select {}
}

// closes the program a while after the browser tab stops pinging
func watchdog() {
	for {
		time.Sleep(20 * time.Second)
		pingMu.Lock()
		idle := pinged && time.Since(lastPing) > 4*time.Minute
		pingMu.Unlock()
		if idle {
			os.Exit(0)
		}
	}
}

func auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != token {
			http.Error(w, "forbidden", 403)
			return
		}
		pingMu.Lock()
		lastPing, pinged = time.Now(), true
		pingMu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func apiInfo(w http.ResponseWriter, r *http.Request) {
	st.mu.Lock()
	defer st.mu.Unlock()
	writeJSON(w, st)
}

// resolve root=game|crash + relative path p, refusing anything outside the root
func resolve(r *http.Request) (string, bool) {
	st.mu.Lock()
	root := map[string]string{"game": st.Game, "crash": st.Crash}[r.URL.Query().Get("root")]
	st.mu.Unlock()
	if root == "" {
		return "", false
	}
	p := r.URL.Query().Get("p")
	for _, part := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
		if part == ".." {
			return "", false
		}
	}
	full := filepath.Join(root, filepath.FromSlash(p))
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return full, true
}

type entry struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

func apiList(w http.ResponseWriter, r *http.Request) {
	full, ok := resolve(r)
	if !ok {
		writeJSON(w, []entry{})
		return
	}
	des, _ := os.ReadDir(full)
	out := []entry{}
	for _, d := range des {
		e := entry{Name: d.Name(), Kind: "file"}
		info, err := os.Stat(filepath.Join(full, d.Name())) // follows links
		if err != nil {
			continue
		}
		if info.IsDir() {
			e.Kind = "directory"
		} else {
			e.Size, e.Mtime = info.Size(), info.ModTime().UnixMilli()
		}
		out = append(out, e)
	}
	writeJSON(w, out)
}

func apiStat(w http.ResponseWriter, r *http.Request) {
	full, ok := resolve(r)
	res := map[string]any{"kind": nil}
	if ok {
		if info, err := os.Stat(full); err == nil {
			if info.IsDir() {
				res["kind"] = "directory"
			} else {
				res["kind"], res["size"], res["mtime"] = "file", info.Size(), info.ModTime().UnixMilli()
			}
		}
	}
	writeJSON(w, res)
}

func apiFile(w http.ResponseWriter, r *http.Request) {
	full, ok := resolve(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	buf := make([]byte, 64*1024)
	var sent int64
	for sent < 32<<20 { // 32 MB cap
		n, err := f.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			sent += int64(n)
		}
		if err != nil {
			break
		}
	}
}

func apiPick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "post only", 405)
		return
	}
	picked := pickFolder()
	res := map[string]any{"picked": picked != "", "valid": false}
	if picked != "" {
		if g := normalizeGame(picked); g != "" {
			st.mu.Lock()
			st.Game, st.Method = g, "picked"
			st.mu.Unlock()
			res["valid"] = true
		}
	}
	writeJSON(w, res)
}

func waitEnter() {
	fmt.Println("Pressione Enter para sair / Press Enter to exit")
	fmt.Scanln()
}
