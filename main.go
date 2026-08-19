package main

import (
	"embed"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	syswin "golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

var (
	singletonMutex          syswin.Handle
	procFindWindow          = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowW")
	procShowWindow          = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
	procSetForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("SetForegroundWindow")
)

const singletonName = "WERD Panel"

func acquireSingleton() bool {
	name, _ := syswin.UTF16PtrFromString(singletonName)
	mu, err := syswin.CreateMutex(nil, false, name)
	if err == syswin.ERROR_ALREADY_EXISTS {
		return false
	}
	if err != nil {
		return false
	}
	singletonMutex = mu
	return true
}

func findWindow(title string) syswin.Handle {
	t, _ := syswin.UTF16PtrFromString(title)
	r, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(t)))
	return syswin.Handle(r)
}

func notifyAlreadyRunning() {
	hwnd := findWindow(singletonName)
	if hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), 9) // SW_RESTORE
		procSetForegroundWindow.Call(uintptr(hwnd))
	}
	caption, _ := syswin.UTF16PtrFromString("WERD Panel")
	text, _ := syswin.UTF16PtrFromString("WERD Panel sudah berjalan.")
	syswin.MessageBox(syswin.HWND(hwnd), text, caption, 0x40|0x10000) // MB_ICONINFORMATION|MB_SETFOREGROUND
}

func main() {
	if !acquireSingleton() {
		notifyAlreadyRunning()
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:             "WERD Panel",
		Width:             800,
		Height:            600,
		DisableResize:     true,
		MinWidth:          800,
		MaxWidth:          800,
		MinHeight:         600,
		MaxHeight:         600,
		HideWindowOnClose: true,
		Windows: &windows.Options{
			WebviewGpuIsDisabled: true,
			BackdropType:         windows.None,
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
