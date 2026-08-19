package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	gort "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogpu/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	root      string
	logDir    string
	httpdRoot string
	httpdBin  string
	mdbBinDir string
	mysqld    string
	mysql     string
	mdbAdmin  string
	mdbIni    string
	mysqlMu   sync.Mutex
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
		mysql:     filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin", "mysql.exe"),
		mdbAdmin:  filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin", "mysqladmin.exe"),
		mdbIni:    filepath.Join(root, "bin", "mysql-8.4.11-winx64", "my.ini"),
	}
	a.sm = newSvcMgr(a)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.copyConfigs()
	hideFromTaskbar()
	if a.GetAutostart() {
		go a.sm.startAll()
	}
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
	menu.Add("Show Panel", a.show)
	menu.AddSeparator()
	menu.Add("Start All Services", a.sm.startAll)
	menu.Add("Stop All Services", a.sm.stopAll)
	menu.AddSeparator()
	menu.Add("Stop Services and Quit", func() {
		a.sm.stopAll()
		a.quit()
	})
	menu.AddSeparator()
	menu.Add("Quit (services stay running)", a.quit)
	tray.SetTooltip("WERD Panel")
	tray.SetIcon(trayIcon)
	tray.OnClick(a.show)
	tray.SetMenu(menu)
	tray.Show()
	tray.Run()
}

func (a *App) show() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
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

func (a *App) AddProject(path, host string) []Project {
	if path == "" {
		return loadProjects(a.root)
	}
	ps := loadProjects(a.root)
	if host == "" {
		host = projectHost(Project{Path: path})
	}
	ps = append(ps, Project{ID: strconv.FormatInt(time.Now().UnixNano(), 10), Path: path, Host: host})
	saveProjects(a.root, ps)
	writeVhosts(a.root)
	a.sm.restartApache()
	a.ensureHostsEntry(host)
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) UpdateHost(id, host string) []Project {
	ps := loadProjects(a.root)
	for i := range ps {
		if ps[i].ID == id {
			ps[i].Host = host
		}
	}
	saveProjects(a.root, ps)
	writeVhosts(a.root)
	a.sm.restartApache()
	a.ensureHostsEntry(host)
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) RemoveProject(id string) []Project {
	ps := loadProjects(a.root)
	out := ps[:0]
	for _, p := range ps {
		if p.ID == id {
			cert, key := a.certFiles(projectHost(p))
			os.Remove(cert)
			os.Remove(key)
			continue
		}
		out = append(out, p)
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

// ---- settings ----

type settings struct {
	Autostart bool `json:"autostart"`
}

func settingsFile(root string) string {
	os.MkdirAll(filepath.Join(root, "var"), 0755)
	return filepath.Join(root, "var", "settings.json")
}

func loadSettings(root string) settings {
	var s settings
	data, err := os.ReadFile(settingsFile(root))
	if err != nil {
		s.Autostart = true
		return s
	}
	if json.Unmarshal(data, &s) != nil {
		s.Autostart = true
	}
	return s
}

func saveSettings(root string, s settings) {
	b, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(settingsFile(root), b, 0644)
}

func (a *App) GetAutostart() bool { return loadSettings(a.root).Autostart }

func (a *App) SetAutostart(on bool) bool {
	s := loadSettings(a.root)
	s.Autostart = on
	saveSettings(a.root, s)
	return on
}

// ---- databases ----

func (a *App) ListDatabases() []string {
	out, err := noWindow(exec.Command(a.mysql, "-u", "root", "-N", "-e", "SHOW DATABASES")).Output()
	if err != nil {
		return nil
	}
	system := map[string]bool{"information_schema": true, "performance_schema": true, "mysql": true, "sys": true}
	var dbs []string
	for _, l := range strings.Split(string(out), "\n") {
		n := strings.TrimSpace(l)
		if n != "" && !system[n] {
			dbs = append(dbs, n)
		}
	}
	return dbs
}

func (a *App) OpenInExplorer(path string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.root, path)
	}
	noWindow(exec.Command("explorer", "/select,"+path)).Start()
}

// ---- projects ----

type Project struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Host string `json:"host"`
	SSL  bool   `json:"ssl"`
}

func projectHost(p Project) string {
	if p.Host != "" {
		return p.Host
	}
	base := filepath.Base(filepath.Clean(p.Path))
	re := regexp.MustCompile(`[^a-zA-Z0-9.-]+`)
	return strings.ToLower(re.ReplaceAllString(base, "-")) + ".localhost"
}

func projectURL(p Project) string {
	scheme := "http"
	if p.SSL {
		scheme = "https"
	}
	return scheme + "://" + projectHost(p)
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

func writeVhosts(root string) {
	os.MkdirAll(filepath.Join(root, "var"), 0755)
	var b strings.Builder
	for _, p := range loadProjects(root) {
		host := projectHost(p)
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
		if p.SSL {
			cert := filepath.Join(root, "var", "certs", host+".pem")
			key := filepath.Join(root, "var", "certs", host+"-key.pem")
			b.WriteString("<VirtualHost *:443>\n")
			b.WriteString("    ServerName " + host + "\n")
			b.WriteString("    ServerAlias www." + host + "\n")
			b.WriteString("    DocumentRoot \"" + dirSlashed + "\"\n")
			b.WriteString("    SSLEngine on\n")
			b.WriteString("    SSLCertificateFile \"" + filepath.ToSlash(cert) + "\"\n")
			b.WriteString("    SSLCertificateKeyFile \"" + filepath.ToSlash(key) + "\"\n")
			b.WriteString("    <Directory \"" + dirSlashed + "\">\n")
			b.WriteString("        Options Indexes FollowSymLinks\n")
			b.WriteString("        AllowOverride All\n")
			b.WriteString("        Require all granted\n")
			b.WriteString("    </Directory>\n")
			b.WriteString("</VirtualHost>\n")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "var", "vhosts.conf"), []byte(b.String()), 0644); err != nil {
		log.Printf("[vhosts] write: %v", err)
	}
}

// ---- ssl (mkcert) ----

const mkcertURL = "https://github.com/FiloSottile/mkcert/releases/download/v1.4.4/mkcert-v1.4.4-windows-amd64.exe"

const mysqlURL = "https://cdn.mysql.com//Downloads/MySQL-8.4/mysql-8.4.11-winx64.zip"

func (a *App) mkcertBin() string { return filepath.Join(a.root, "var", "tools", "mkcert.exe") }

func (a *App) certDir() string { return filepath.Join(a.root, "var", "certs") }

func (a *App) certFiles(host string) (cert, key string) {
	os.MkdirAll(a.certDir(), 0755)
	cert = filepath.Join(a.certDir(), host+".pem")
	key = filepath.Join(a.certDir(), host+"-key.pem")
	return cert, key
}

func (a *App) ensureMkcert() error {
	bin := a.mkcertBin()
	if _, err := os.Stat(bin); err == nil {
		return nil
	}
	os.MkdirAll(filepath.Dir(bin), 0755)
	resp, err := http.Get(mkcertURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download mkcert: HTTP %d", resp.StatusCode)
	}
	tmp := bin + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, bin)
}

func (a *App) runMkcert(args ...string) error {
	if err := a.ensureMkcert(); err != nil {
		return err
	}
	cmd := noWindow(exec.Command(a.mkcertBin(), args...))
	cmd.Env = append(os.Environ(), "JAVA_HOME=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkcert %v: %v: %s", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *App) CAInstalled() bool {
	bin := a.mkcertBin()
	if _, err := os.Stat(bin); err != nil {
		return false
	}
	script := "if (Get-ChildItem Cert:\\CurrentUser\\Root | Where-Object { $_.Subject -match 'mkcert' }) { exit 0 } else { exit 1 }"
	if err := noWindow(exec.Command("powershell", "-NoProfile", "-Command", script)).Run(); err == nil {
		return true
	}
	out, err := noWindow(exec.Command(bin, "-CAROOT")).Output()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(strings.TrimSpace(string(out)), "rootCA.pem"))
	return err == nil
}

func (a *App) InstallCA() bool {
	if err := a.runMkcert("-install"); err != nil && !a.CAInstalled() {
		a.emit("error", map[string]any{"service": "ssl", "message": err.Error()})
	}
	return a.CAInstalled()
}

func (a *App) UninstallCA() bool {
	if err := a.runMkcert("-uninstall"); err != nil && a.CAInstalled() {
		a.emit("error", map[string]any{"service": "ssl", "message": err.Error()})
	}
	return a.CAInstalled()
}

func (a *App) InstallSSL(id string) []Project {
	ps := loadProjects(a.root)
	var p *Project
	for i := range ps {
		if ps[i].ID == id {
			p = &ps[i]
		}
	}
	if p == nil {
		return loadProjects(a.root)
	}
	if !a.CAInstalled() && !a.InstallCA() {
		return loadProjects(a.root)
	}
	host := projectHost(*p)
	cert, key := a.certFiles(host)
	os.Remove(cert)
	os.Remove(key)
	if err := a.runMkcert("-cert-file", cert, "-key-file", key, host, "www."+host); err != nil {
		a.emit("error", map[string]any{"service": "ssl", "message": err.Error()})
		return loadProjects(a.root)
	}
	p.SSL = true
	saveProjects(a.root, ps)
	writeVhosts(a.root)
	a.sm.restartApache()
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) UninstallSSL(id string) []Project {
	ps := loadProjects(a.root)
	var host string
	for i := range ps {
		if ps[i].ID == id {
			host = projectHost(ps[i])
			ps[i].SSL = false
		}
	}
	if host != "" {
		cert, key := a.certFiles(host)
		os.Remove(cert)
		os.Remove(key)
	}
	saveProjects(a.root, ps)
	writeVhosts(a.root)
	a.sm.restartApache()
	ps = loadProjects(a.root)
	a.emit("projects", map[string]any{"projects": ps})
	return ps
}

func (a *App) ensureHostsEntry(host string) {
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return
	}
	hostsPath := "C:\\Windows\\System32\\drivers\\etc\\hosts"
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		a.emit("error", map[string]any{"service": "ssl", "message": "hosts: " + err.Error()})
		return
	}
	for _, l := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasSuffix(t, " "+host) || strings.HasSuffix(t, " www."+host) {
			return
		}
	}
	entry := "\n127.0.0.1 " + host + " www." + host + "\n"
	if err := os.WriteFile(hostsPath, append(data, []byte(entry)...), 0644); err != nil {
		a.emit("error", map[string]any{"service": "ssl", "message": "hosts (jalankan sebagai admin?): " + err.Error()})
	}
}

// ---- mysql (optional) ----

func (a *App) MySQLInstalled() bool {
	_, err := os.Stat(a.mysqld)
	return err == nil
}

type progressWriter struct {
	emit  func(string, any)
	total int64
	done  int64
	last  time.Time
	mu    sync.Mutex
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.done += int64(len(p))
	now := time.Now()
	if now.Sub(w.last) > 500*time.Millisecond {
		w.last = now
		w.emit("mysql-progress", map[string]any{"done": w.done, "total": w.total})
	}
	w.mu.Unlock()
	return len(p), nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	dest = filepath.Clean(dest)
	for _, f := range r.File {
		p := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(p, dest+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(p)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err = io.Copy(w, rc); err != nil {
			w.Close()
			rc.Close()
			return err
		}
		w.Close()
		rc.Close()
	}
	return nil
}

func (a *App) InstallMySQL() bool {
	a.mysqlMu.Lock()
	defer a.mysqlMu.Unlock()
	if a.MySQLInstalled() {
		return true
	}

	binDir := filepath.Join(a.root, "bin")
	tmp := filepath.Join(a.root, "var", "mysql.zip")
	os.MkdirAll(binDir, 0755)

	resp, err := http.Get(mysqlURL)
	if err != nil {
		a.emit("error", map[string]any{"service": "mariadb", "message": "download mysql: " + err.Error()})
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		a.emit("error", map[string]any{"service": "mariadb", "message": fmt.Sprintf("download mysql: HTTP %d", resp.StatusCode)})
		return false
	}

	f, err := os.Create(tmp)
	if err != nil {
		a.emit("error", map[string]any{"service": "mariadb", "message": err.Error()})
		return false
	}
	pw := &progressWriter{emit: a.emit, total: resp.ContentLength}
	if _, err = io.Copy(f, io.TeeReader(resp.Body, pw)); err != nil {
		f.Close()
		os.Remove(tmp)
		a.emit("error", map[string]any{"service": "mariadb", "message": "download mysql: " + err.Error()})
		return false
	}
	f.Close()

	if err = unzip(tmp, binDir); err != nil {
		os.Remove(tmp)
		a.emit("error", map[string]any{"service": "mariadb", "message": "extract mysql: " + err.Error()})
		return false
	}
	os.Remove(tmp)

	copyConfig(a.root, filepath.Join(a.root, "config", "my.ini"), filepath.Join(a.root, "bin", "mysql-8.4.11-winx64", "my.ini"))

	dataDir := filepath.Join(a.root, "var", "mysql")
	if _, err := os.Stat(filepath.Join(dataDir, "mysql")); os.IsNotExist(err) {
		os.MkdirAll(dataDir, 0755)
		if out, err := noWindow(exec.Command(a.mysqld, "--initialize-insecure", "--datadir="+dataDir)).CombinedOutput(); err != nil {
			a.emit("error", map[string]any{"service": "mariadb", "message": "initialize: " + string(out)})
		}
	}

	a.emit("mysql-progress", map[string]any{"done": 1, "total": 1})
	return a.MySQLInstalled()
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
	copyConfig(a.root, filepath.Join(a.root, "config", "php.ini"), filepath.Join(a.root, "bin", "php-8.4.24-Win32-vs17-x64", "php.ini"))
	if _, err := os.Stat(filepath.Join(a.root, "bin", "mysql-8.4.11-winx64")); err == nil {
		copyConfig(a.root, filepath.Join(a.root, "config", "my.ini"), filepath.Join(a.root, "bin", "mysql-8.4.11-winx64", "my.ini"))
	}
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
		if !a.MySQLInstalled() {
			s.mu.Unlock()
			m.emit("status", map[string]any{"service": service, "running": false, "procs": []procInfo{}})
			return
		}
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
