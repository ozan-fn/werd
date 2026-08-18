package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gogpu/systray"
	"github.com/gorilla/websocket"
)

//go:embed all:web/dist
var distFS embed.FS

var (
	root      string
	logDir    string
	httpdRoot string
	httpdBin  string
	mdbBinDir string
	mariadbd  string
	mdbAdmin  string
	mdbIni    string
)

func init() {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	root = filepath.Dir(exe)
	logDir    = filepath.Join(root, "var", "logs")
	httpdRoot = filepath.Join(root, "bin", "httpd-2.4.66-251206-Win64-VS17", "Apache24")
	httpdBin  = filepath.Join(httpdRoot, "bin", "httpd.exe")
	mdbBinDir = filepath.Join(root, "bin", "mariadb-12.3.2-winx64", "bin")
	mariadbd  = filepath.Join(mdbBinDir, "mariadbd.exe")
	mdbAdmin  = filepath.Join(mdbBinDir, "mariadb-admin.exe")
	mdbIni    = filepath.Join(root, "bin", "mariadb-12.3.2-winx64", "my.ini")
}

func noWindow(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// ---- hub ----

type hub struct {
	mu      sync.Mutex
	clients map[*client]bool
}

func newHub() *hub { return &hub{clients: map[*client]bool{}} }

func (h *hub) add(c *client) { h.mu.Lock(); h.clients[c] = true; h.mu.Unlock() }
func (h *hub) del(c *client) { h.mu.Lock(); delete(h.clients, c); h.mu.Unlock() }

func (h *hub) broadcast(v any) {
	b, _ := json.Marshal(v)
	h.mu.Lock()
	for c := range h.clients {
		select {
		case c.send <- b:
		default:
		}
	}
	h.mu.Unlock()
}

type client struct {
	ws   *websocket.Conn
	send chan []byte
	h    *hub
	sm   *svcMgr
}

func (c *client) writePump() {
	defer c.ws.Close()
	for b := range c.send {
		c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := c.ws.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}

func (c *client) readPump() {
	defer func() { c.h.del(c); c.ws.Close() }()
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Action  string `json:"action"`
			Service string `json:"service"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		switch m.Action {
		case "start":
			c.sm.start(m.Service)
		case "stop":
			c.sm.stop(m.Service)
		case "startAll":
			c.sm.startAll()
		case "stopAll":
			c.sm.stopAll()
		}
	}
}

// ---- services ----

type svc struct {
	name   string
	running bool
	mu     sync.Mutex
	proc   *exec.Cmd
}

type svcMgr struct {
	h  *hub
	sv map[string]*svc
}

func newSvcMgr(h *hub) *svcMgr {
	return &svcMgr{
		h: h,
		sv: map[string]*svc{
			"apache":  {name: "apache", running: procRunning("httpd.exe")},
			"mariadb": {name: "mariadb", running: procRunning("mariadbd.exe")},
		},
	}
}

func imageFor(service string) string {
	if service == "mariadb" {
		return "mariadbd"
	}
	return "httpd"
}

func procRunning(image string) bool {
	procs, _ := procPIDs(image + ".exe")
	return len(procs) > 0
}

func procPIDs(image string) ([]int, error) {
	out, err := noWindow(exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/FO", "CSV", "/NH")).Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, l := range strings.Split(string(out), "\n") {
		parts := strings.Split(l, ",")
		if len(parts) < 2 || !strings.Contains(parts[0], image) {
			continue
		}
		pid := strings.Trim(parts[1], `" `)
		if n, err := strconv.Atoi(pid); err == nil {
			pids = append(pids, n)
		}
	}
	return pids, nil
}

type procSample struct {
	pid int
	cpu float64
	ram float64
}

func procStats(image string) []procInfo {
	s1 := sampleProc(image)
	if len(s1) == 0 {
		return nil
	}
	time.Sleep(900 * time.Millisecond)
	s2 := sampleProc(image)
	res := make([]procInfo, 0, len(s2))
	for _, p2 := range s2 {
		cpu := 0.0
		if p1, ok := s1[p2.pid]; ok && p2.cpu >= p1.cpu {
			cpu = (p2.cpu - p1.cpu) / 0.9 * 100
		}
		if cpu < 0 {
			cpu = 0
		}
		res = append(res, procInfo{Pid: p2.pid, CPU: math.Round(cpu*10) / 10, RAM: math.Round(p2.ram*10) / 10})
	}
	return res
}

func sampleProc(image string) map[int]procSample {
	script := "$p=Get-Process -Name '" + image + "' -ErrorAction SilentlyContinue;" +
		"if($p){$p|Select-Object Id,CPU,WorkingSet64|ConvertTo-Json -Compress}else{'[]'}"
	out, err := noWindow(exec.Command("powershell", "-NoProfile", "-Command", script)).Output()
	if err != nil {
		return nil
	}
	raw := strings.TrimSpace(string(out))
	raw = strings.TrimPrefix(raw, "\uFEFF")
	if raw == "" || raw == "[]" {
		return nil
	}
	items := []struct {
		Id           int     `json:"Id"`
		CPU          float64 `json:"CPU"`
		WorkingSet64 int64   `json:"WorkingSet64"`
	}{}
	if strings.HasPrefix(raw, "[") {
		json.Unmarshal([]byte(raw), &items)
	} else {
		var one struct {
			Id           int     `json:"Id"`
			CPU          float64 `json:"CPU"`
			WorkingSet64 int64   `json:"WorkingSet64"`
		}
		if json.Unmarshal([]byte(raw), &one) == nil {
			items = append(items, one)
		}
	}
	res := map[int]procSample{}
	for _, it := range items {
		res[it.Id] = procSample{pid: it.Id, cpu: it.CPU, ram: float64(it.WorkingSet64) / 1048576}
	}
	return res
}

type procInfo struct {
	Pid int     `json:"pid"`
	CPU float64 `json:"cpu"`
	RAM float64 `json:"ram"`
}

func (m *svcMgr) status(service string) (bool, []procInfo) {
	s := m.sv[service]
	procs := procStats(imageFor(service))
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if !running && len(procs) > 0 {
		running = true
	}
	return running, procs
}

func (m *svcMgr) pushStatus() {
	for name := range m.sv {
		running, procs := m.status(name)
		m.h.broadcast(map[string]any{"type": "status", "service": name, "running": running, "procs": procs})
	}
}

func (m *svcMgr) start(service string) {
	s := m.sv[service]
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		running, procs := m.status(service)
		m.h.broadcast(map[string]any{"type": "status", "service": service, "running": running, "procs": procs})
		return
	}
	var cmd *exec.Cmd
	switch service {
	case "apache":
		cmd = noWindow(exec.Command(httpdBin, "-d", httpdRoot))
		cmd.Dir = httpdRoot
	case "mariadb":
		cmd = noWindow(exec.Command(mariadbd, "--defaults-file="+mdbIni))
		cmd.Dir = mdbBinDir
	}
	logFile := logDir + "/" + service + ".log"
	lw := &lineWriter{svc: m.h, tag: service, file: logFile}
	cmd.Stdout = lw
	cmd.Stderr = lw
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		m.h.broadcast(map[string]any{"type": "error", "service": service, "message": err.Error()})
		m.h.broadcast(map[string]any{"type": "status", "service": service, "running": false, "procs": []procInfo{}})
		return
	}
	s.proc = cmd
	s.running = true
	s.mu.Unlock()
	running, procs := m.status(service)
	m.h.broadcast(map[string]any{"type": "status", "service": service, "running": running, "procs": procs})
	go func() {
		cmd.Wait()
		s.mu.Lock()
		s.running = false
		s.proc = nil
		s.mu.Unlock()
		running, procs := m.status(service)
		m.h.broadcast(map[string]any{"type": "status", "service": service, "running": running, "procs": procs})
	}()
}

func (m *svcMgr) stop(service string) {
	s := m.sv[service]
	if s == nil {
		return
	}
	switch service {
	case "apache":
		if err := noWindow(exec.Command("taskkill", "/F", "/IM", "httpd.exe")).Run(); err != nil {
			log.Printf("[apache] stop: %v", err)
		}
	case "mariadb":
		if err := noWindow(exec.Command(mdbAdmin, "-u", "root", "shutdown")).Run(); err != nil {
			log.Printf("[mariadb] admin shutdown: %v", err)
		}
		if err := noWindow(exec.Command("taskkill", "/F", "/IM", "mariadbd.exe")).Run(); err != nil {
			log.Printf("[mariadb] stop: %v", err)
		}
	}
	s.mu.Lock()
	s.running = false
	s.proc = nil
	s.mu.Unlock()
	running, procs := m.status(service)
	if running {
		m.h.broadcast(map[string]any{"type": "error", "service": service, "message": "stop failed: process still running"})
	}
	m.h.broadcast(map[string]any{"type": "status", "service": service, "running": running, "procs": procs})
}

func (m *svcMgr) startAll() {
	m.start("mariadb")
	m.start("apache")
}

func (m *svcMgr) stopAll() {
	m.stop("apache")
	m.stop("mariadb")
}

type lineWriter struct {
	svc  *hub
	tag  string
	file string
	buf  bytes.Buffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	all := w.buf.Bytes()
	for {
		i := bytes.IndexByte(all, '\n')
		if i < 0 {
			break
		}
		line := string(all[:i])
		w.buf.Next(i + 1)
		all = w.buf.Bytes()
		if strings.TrimSpace(line) != "" {
			w.svc.broadcast(map[string]any{"type": "log", "service": w.tag, "line": line})
			if f, err := os.OpenFile(w.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				io.WriteString(f, line+"\n")
				f.Close()
			}
		}
	}
	return len(p), nil
}

// ---- static with precompressed ----

func staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if !strings.Contains(filepath.Base(p), ".") {
			p = "index.html"
		}
		ct := mime.TypeByExtension(filepath.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		enc := r.Header.Get("Accept-Encoding")
		for _, c := range []struct {
			code, ext string
		}{{"br", ".br"}, {"gzip", ".gz"}} {
			if strings.Contains(enc, c.code) {
				if b, err := distFS.ReadFile("web/dist/" + p + c.ext); err == nil {
					w.Header().Set("Content-Type", ct)
					w.Header().Set("Content-Encoding", c.code)
					w.Header().Set("Vary", "Accept-Encoding")
					w.Write(b)
					return
				}
			}
		}
		b, err := distFS.ReadFile("web/dist/" + p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Write(b)
	})
}

func copyConfig(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		log.Printf("[config] read %s: %v", src, err)
		return
	}
	data = bytes.ReplaceAll(data, []byte("{{ROOT}}"), []byte(filepath.ToSlash(root)))
	if err := os.WriteFile(dst, data, 0644); err != nil {
		log.Printf("[config] write %s: %v", dst, err)
	}
}

func copyConfigs() {
	os.MkdirAll(logDir, 0755)
	copyConfig(filepath.Join(root, "config", "httpd.conf"), filepath.Join(httpdRoot, "conf", "httpd.conf"))
	copyConfig(filepath.Join(root, "config", "php.ini"), filepath.Join(root, "bin", "php-8.4.23-Win32-vs17-x64", "php.ini"))
	copyConfig(filepath.Join(root, "config", "my.ini"), filepath.Join(root, "bin", "mariadb-12.3.2-winx64", "my.ini"))
}

func main() {
	copyConfigs()
	h := newHub()
	sm := newSvcMgr(h)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &client{ws: conn, send: make(chan []byte, 64), h: h, sm: sm}
		h.add(c)
		go c.writePump()
		sm.pushStatus()
		c.readPump()
		close(c.send)
	})
	http.Handle("/", staticHandler())

	port := os.Getenv("WERD_PORT")
	if port == "" {
		port = "8090"
	}
	addr := "127.0.0.1:" + port
	url := "http://" + addr
	log.Printf("WERD Panel: %s", url)

	srv := &http.Server{Addr: addr, Handler: nil}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v (is %s in use? set WERD_PORT)", err, addr)
			sm.stopAll()
			os.Exit(1)
		}
	}()

	go func() {
		for range time.Tick(3 * time.Second) {
			sm.pushStatus()
		}
	}()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		sm.stopAll()
		srv.Close()
		os.Exit(0)
	}()

	runTray(sm, url)
}

// ---- tray + console ----

func openBrowser(url string) {
	noWindow(exec.Command("cmd", "/c", "start", url)).Start()
}

func trayIcon() []byte {
	if b, err := distFS.ReadFile("web/dist/favicon.png"); err == nil {
		return b
	}
	return nil
}

func runTray(sm *svcMgr, url string) {
	quit := make(chan struct{})
	tray := systray.New()

	menu := systray.NewMenu()
	menu.Add("Open Panel", func() { openBrowser(url) })
	menu.AddSeparator()
	menu.Add("Stop All Services", func() { sm.stopAll() })
	menu.AddSeparator()
	menu.Add("Quit", func() {
		tray.Remove()
		close(quit)
		go sm.stopAll()
	})

	tray.SetTooltip("WERD Panel")
	tray.SetIcon(trayIcon())
	tray.SetMenu(menu)
	tray.OnClick(func() { openBrowser(url) })
	tray.OnDoubleClick(func() { openBrowser(url) })
	tray.Show()

	tray.ShowNotification("WERD Panel", "Running in tray — " + url)
	tray.Run()

	<-quit
	os.Exit(0)
}