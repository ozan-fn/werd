package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	gort "runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gogpu/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows/registry"
)

type App struct {
	ctx       context.Context
	root      string
	logDir    string
	httpdRoot string
	httpdBin  string
	mdbBinDir string
	mysqld    string
	mdbAdmin  string
	mdbIni    string
	sm        *svcMgr
}

func findRoot() string {
	start, _ := os.Executable()
	if start == "" {
		start, _ = os.Getwd()
	}
	for d := filepath.Dir(start); ; d = filepath.Dir(d) {
		if isRoot(d) {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			return start
		}
	}
}

func isRoot(d string) bool {
	for _, n := range []string{"bin", "config", "var"} {
		if _, err := os.Stat(filepath.Join(d, n)); err != nil {
			return false
		}
	}
	return true
}

func NewApp() *App {
	root := findRoot()
	a := &App{
		root:      root,
		logDir:    filepath.Join(root, "var", "logs"),
		httpdRoot: filepath.Join(root, "bin", "httpd-2.4.66-251206-Win64-VS17", "Apache24"),
		httpdBin:  filepath.Join(root, "bin", "httpd-2.4.66-251206-Win64-VS17", "Apache24", "bin", "httpd.exe"),
		mdbBinDir: filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin"),
		mysqld:    filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin", "mysqld.exe"),
		mdbAdmin:  filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin", "mysqladmin.exe"),
		mdbIni:    filepath.Join(root, "bin", "mysql-8.4.11-winx64", "my.ini"),
	}
	a.sm = newSvcMgr(a)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.copyConfigs()
	time.Sleep(150 * time.Millisecond)
	a.sm.pushStatus()
	go func() {
		for range time.Tick(3 * time.Second) {
			a.sm.pushStatus()
		}
	}()
	go a.runTray()
}

func (a *App) runTray() {
	gort.LockOSThread()
	defer gort.UnlockOSThread()
	tray := systray.New()
	menu := systray.NewMenu()
	menu.Add("Stop Services and Quit", func() {
		a.sm.stopAll()
		a.quit()
	})
	menu.AddSeparator()
	menu.Add("Quit", func() {
		a.quit()
	})
	tray.SetTooltip("WERD Panel")
	tray.SetIcon(trayIcon)
	tray.SetMenu(menu)
	tray.Show()
	tray.Run()
}

func (a *App) quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func (a *App) emit(event string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, event, data)
	}
}

// ---- bound methods ----

func (a *App) Start(service string) { a.sm.start(service) }
func (a *App) Stop(service string)  { a.sm.stop(service) }

func (a *App) Restart(service string) {
	a.sm.stop(service)
	a.sm.start(service)
}

func (a *App) StartAll() { a.sm.startAll() }
func (a *App) StopAll()  { a.sm.stopAll() }

func (a *App) ListProjects() []Project { return loadProjects(a.root) }

func (a *App) AddProject(path, url string) []Project {
	if path == "" {
		return loadProjects(a.root)
	}
	ps := loadProjects(a.root)
	ps = append(ps, Project{ID: strconv.FormatInt(time.Now().UnixNano(), 10), Path: path, URL: url})
	saveProjects(a.root, ps)
	writeVhosts(a.root)
	a.sm.restartApache()
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) UpdateUrl(id, url string) []Project {
	ps := loadProjects(a.root)
	for i := range ps {
		if ps[i].ID == id {
			ps[i].URL = url
		}
	}
	saveProjects(a.root, ps)
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) RemoveProject(id string) []Project {
	ps := loadProjects(a.root)
	out := ps[:0]
	for _, p := range ps {
		if p.ID != id {
			out = append(out, p)
		}
	}
	saveProjects(a.root, out)
	writeVhosts(a.root)
	a.sm.restartApache()
	out = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": out})
	return out
}

func (a *App) PickFolder() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Pilih folder project"})
	if err != nil {
		return ""
	}
	return dir
}

func (a *App) GetPhpPath() bool { return phpInUserPath(a.root) }

func (a *App) OpenURL(url string) {
	if a.ctx != nil {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}

func (a *App) SetPhpPath(on bool) bool {
	setPhpInUserPath(a.root, on)
	return phpInUserPath(a.root)
}

// ---- projects ----

type Project struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func projectsFile(root string) string {
	os.MkdirAll(filepath.Join(root, "var"), 0755)
	return filepath.Join(root, "var", "projects.json")
}

func loadProjects(root string) []Project {
	data, err := os.ReadFile(projectsFile(root))
	if err != nil {
		return nil
	}
	var ps []Project
	if json.Unmarshal(data, &ps) != nil {
		return nil
	}
	return ps
}

func saveProjects(root string, ps []Project) {
	b, _ := json.MarshalIndent(ps, "", "  ")
	os.WriteFile(projectsFile(root), b, 0644)
}

// ---- vhosts ----

func projectHost(path string) string {
	base := filepath.Base(filepath.Clean(path))
	re := regexp.MustCompile(`[^a-zA-Z0-9.-]+`)
	return strings.ToLower(re.ReplaceAllString(base, "-")) + ".localhost"
}

func writeVhosts(root string) {
	os.MkdirAll(filepath.Join(root, "var"), 0755)
	var b strings.Builder
	for _, p := range loadProjects(root) {
		host := projectHost(p.Path)
		dir := p.Path
		if s, err := os.Stat(filepath.Join(dir, "public")); err == nil && s.IsDir() {
			dir = filepath.Join(dir, "public")
		}
		dirSlashed := filepath.ToSlash(dir)
		b.WriteString("\n<VirtualHost *:80>\n")
		b.WriteString("    ServerName " + host + "\n")
		b.WriteString("    ServerAlias www." + host + "\n")
		b.WriteString("    DocumentRoot \"" + dirSlashed + "\"\n")
		b.WriteString("    <Directory \"" + dirSlashed + "\">\n")
		b.WriteString("        Options Indexes FollowSymLinks\n")
		b.WriteString("        AllowOverride All\n")
		b.WriteString("        Require all granted\n")
		b.WriteString("    </Directory>\n")
		b.WriteString("</VirtualHost>\n")
	}
	if err := os.WriteFile(filepath.Join(root, "var", "vhosts.conf"), []byte(b.String()), 0644); err != nil {
		log.Printf("[vhosts] write: %v", err)
	}
}

// ---- config ----

func copyConfig(root, src, dst string) {
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

func (a *App) copyConfigs() {
	os.MkdirAll(a.logDir, 0755)
	os.MkdirAll(filepath.Join(a.root, "var", "www"), 0755)
	copyConfig(a.root, filepath.Join(a.root, "config", "httpd.conf"), filepath.Join(a.httpdRoot, "conf", "httpd.conf"))
	copyConfig(a.root, filepath.Join(a.root, "config", "php.ini"), filepath.Join(a.root, "bin", "php-8.4.23-Win32-vs17-x64", "php.ini"))
	copyConfig(a.root, filepath.Join(a.root, "config", "my.ini"), filepath.Join(a.root, "bin", "mysql-8.4.11-winx64", "my.ini"))
	copyConfig(a.root, filepath.Join(a.root, "config", "config.inc.php"), filepath.Join(a.root, "bin", "phpMyAdmin-5.2.3-english", "config.inc.php"))
	writeVhosts(a.root)
}

// ---- services ----

type svc struct {
	name    string
	running bool
	mu      sync.Mutex
	proc    *exec.Cmd
}

type svcMgr struct {
	a    *App
	emit func(event string, data any)
	sv   map[string]*svc
}

func newSvcMgr(a *App) *svcMgr {
	return &svcMgr{
		a:    a,
		emit: a.emit,
		sv: map[string]*svc{
			"apache":  {name: "apache", running: procRunning("httpd.exe")},
			"mariadb": {name: "mariadb", running: procRunning("mysqld.exe")},
		},
	}
}

func noWindow(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func imageFor(service string) string {
	if service == "mariadb" {
		return "mysqld"
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

type procInfo struct {
	Pid int     `json:"pid"`
	CPU float64 `json:"cpu"`
	RAM float64 `json:"ram"`
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
		m.emit("status", map[string]any{"service": name, "running": running, "procs": procs})
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
		m.emit("status", map[string]any{"service": service, "running": running, "procs": procs})
		return
	}
	a := m.a
	var cmd *exec.Cmd
	switch service {
	case "apache":
		cmd = noWindow(exec.Command(a.httpdBin, "-d", a.httpdRoot))
		cmd.Dir = a.httpdRoot
	case "mariadb":
		dataDir := filepath.Join(a.root, "var", "mysql")
		if _, err := os.Stat(filepath.Join(dataDir, "mysql")); os.IsNotExist(err) {
			os.MkdirAll(dataDir, 0755)
			if out, err := noWindow(exec.Command(a.mysqld, "--initialize-insecure", "--datadir="+dataDir)).CombinedOutput(); err != nil {
				m.emit("log", map[string]any{"service": "mariadb", "line": "initialize: " + string(out)})
			}
		}
		cmd = noWindow(exec.Command(a.mysqld, "--defaults-file="+a.mdbIni))
		cmd.Dir = a.mdbBinDir
	}
	logFile := a.logDir + "/" + service + ".log"
	lw := &lineWriter{emit: m.emit, tag: service, file: logFile}
	cmd.Stdout = lw
	cmd.Stderr = lw
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		m.emit("error", map[string]any{"service": service, "message": err.Error()})
		m.emit("status", map[string]any{"service": service, "running": false, "procs": []procInfo{}})
		return
	}
	s.proc = cmd
	s.running = true
	s.mu.Unlock()
	running, procs := m.status(service)
	m.emit("status", map[string]any{"service": service, "running": running, "procs": procs})
	go func() {
		cmd.Wait()
		s.mu.Lock()
		s.running = false
		s.proc = nil
		s.mu.Unlock()
		running, procs := m.status(service)
		m.emit("status", map[string]any{"service": service, "running": running, "procs": procs})
	}()
}

func (m *svcMgr) stop(service string) {
	s := m.sv[service]
	if s == nil {
		return
	}
	a := m.a
	switch service {
	case "apache":
		if err := noWindow(exec.Command("taskkill", "/F", "/IM", "httpd.exe")).Run(); err != nil {
			log.Printf("[apache] stop: %v", err)
		}
	case "mariadb":
		if err := noWindow(exec.Command(a.mdbAdmin, "-u", "root", "shutdown")).Run(); err != nil {
			log.Printf("[mariadb] admin shutdown: %v", err)
		}
		if err := noWindow(exec.Command("taskkill", "/F", "/IM", "mysqld.exe")).Run(); err != nil {
			log.Printf("[mariadb] stop: %v", err)
		}
	}
	s.mu.Lock()
	s.running = false
	s.proc = nil
	s.mu.Unlock()
	running, procs := m.status(service)
	if running {
		m.emit("error", map[string]any{"service": service, "message": "stop failed: process still running"})
	}
	m.emit("status", map[string]any{"service": service, "running": running, "procs": procs})
}

func (m *svcMgr) startAll() {
	m.start("mariadb")
	m.start("apache")
}

func (m *svcMgr) restartApache() {
	m.stop("apache")
	m.start("apache")
}

func (m *svcMgr) stopAll() {
	m.stop("apache")
	m.stop("mariadb")
}

type lineWriter struct {
	emit func(event string, data any)
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
			w.emit("log", map[string]any{"service": w.tag, "line": line})
			if f, err := os.OpenFile(w.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				io.WriteString(f, line+"\n")
				f.Close()
			}
		}
	}
	return len(p), nil
}

// ---- path ----

func phpDir(root string) string {
	return filepath.Join(root, "bin", "php-8.4.23-Win32-vs17-x64")
}

func mysqlDir(root string) string {
	return filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin")
}

func configDir(root string) string {
	return filepath.Join(root, "config")
}

func pathDirs(root string) []string {
	return []string{configDir(root), phpDir(root), mysqlDir(root)}
}

func phpInUserPath(root string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Path")
	if err != nil {
		return false
	}
	low := strings.ToLower(v)
	for _, d := range pathDirs(root) {
		if !strings.Contains(low, strings.ToLower(d)) {
			return false
		}
	}
	return true
}

func setPhpInUserPath(root string, on bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		log.Printf("[path] open: %v", err)
		return
	}
	defer k.Close()
	cur, _, _ := k.GetStringValue("Path")
	parts := strings.Split(cur, ";")
	var keep []string
	remove := map[string]bool{}
	for _, d := range pathDirs(root) {
		remove[strings.ToLower(d)] = true
	}
	for _, p := range parts {
		if p != "" && !remove[strings.ToLower(p)] {
			keep = append(keep, p)
		}
	}
	if on {
		keep = append(pathDirs(root), keep...)
	}
	if err := k.SetStringValue("Path", strings.Join(keep, ";")); err != nil {
		log.Printf("[path] set: %v", err)
	}
}