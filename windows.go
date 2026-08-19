//go:build windows

package main

import (
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	procFindWindowW       = user32.NewProc("FindWindowW")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
)

const (
	gwlExStyle     = 0xFFFFFFFFFFFFFFEC
	wsExToolWindow = 0x00000080
)

func noWindow(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func hideFromTaskbar() {
	for i := 0; i < 5; i++ {
		name, _ := syscall.UTF16PtrFromString("WERD Panel")
		h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(name)))
		if h != 0 {
			style, _, _ := procGetWindowLongPtrW.Call(h, uintptr(gwlExStyle))
			procSetWindowLongPtrW.Call(h, uintptr(gwlExStyle), style|wsExToolWindow)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func phpDir(root string) string {
	return filepath.Join(root, "bin", "php-8.4.24-Win32-vs17-x64")
}

func mysqlDir(root string) string {
	return filepath.Join(root, "bin", "mysql-8.4.11-winx64", "bin")
}

func composerDir(root string) string {
	return filepath.Join(root, "bin", "composer-2.10.2")
}

func pathDirs(root string) []string {
	return []string{composerDir(root), phpDir(root), mysqlDir(root)}
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
