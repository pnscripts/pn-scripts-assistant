//go:build windows && amd64

package window

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pn-scripts-assistant/internal/brain/paths"
)

/*
 * The window on Windows: a plain Win32 window holding a WebView2.
 *
 * Until 0.9 the Windows build opened the system browser, which made the
 * assistant a tab among tabs — closed by accident, lost behind others, and
 * showing an address bar with a port number in it. WebView2 is the engine
 * Windows itself ships for exactly this (Edge's, kept up to date by Windows
 * Update, present on Windows 11 and on almost every Windows 10).
 *
 * Without cgo, like the rest of the Windows build: user32 and the WebView2
 * COM interfaces are reached through syscall, the vtable indices taken from
 * WebView2.h of the SDK whose loader ships beside the program (see
 * scripts/build-packages.sh, which downloads it from NuGet and checks its
 * SHA-256). Only documented entry points are used: the loader's
 * CreateCoreWebView2EnvironmentWithOptions and
 * GetAvailableCoreWebView2BrowserVersionString.
 *
 * Whatever is missing — the loader, the runtime, a window station — the
 * program falls back to the browser exactly as before. A window that cannot
 * open is a cosmetic loss; an assistant that cannot be reached is not.
 *
 * amd64 only: the release builds amd64 for Windows, and put_Bounds takes a
 * RECT by value, which the amd64 calling convention passes by pointer and
 * others do not.
 */

// LoaderName is the WebView2 loader the packages ship beside the program.
const LoaderName = "WebView2Loader.dll"

const (
	wmDestroy  = 0x0002
	wmSize     = 0x0005
	wmClose    = 0x0010
	wmNavigate = 0x8000 + 1 // WM_APP + 1
	wmPresent  = 0x8000 + 2

	wsOverlappedWindow = 0x00CF0000
	cwUseDefault       = 0x80000000
	swShow             = 5
	swRestore          = 9
	idcArrow           = 32512
	colorWindow        = 5
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procIsIconic             = user32.NewProc("IsIconic")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procSetDpiAwarenessCtx   = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procCoInitializeEx       = windows.NewLazySystemDLL("ole32.dll").NewProc("CoInitializeEx")
	procCreateEnvironment    *windows.LazyProc
	procAvailableVersion     *windows.LazyProc
	errNoLoader              = errors.New("the WebView2 loader is not beside the program")
	errNoRuntime             = errors.New("the WebView2 runtime is not installed")
	errClosedBeforeReady     = errors.New("the window closed before its page could be shown")
	className                = windows.StringToUTF16Ptr("PNScriptsAssistantWindow")
	wndProcCallback          = windows.NewCallback(wndProc)
	setupOnce, availableOnce sync.Once
	setupErr, availableErr   error
)

func init() {
	// Win32 windows belong to the thread that made them, and the message loop
	// must run there too. Locked in init so it is the main thread, as on Linux.
	runtime.LockOSThread()
}

/*
 * The COM side.
 *
 * object is any WebView2 interface: a pointer to a pointer to its vtable.
 * call invokes the method at an index of that vtable with the object itself
 * as the first argument, which is what a COM method is.
 */
type object struct{ vtbl *[64]uintptr }

func (o *object) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[method], all...)

	return r
}

func (o *object) release() {
	if o != nil {
		o.call(2)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

// Vtable indices, from WebView2.h (SDK 1.0.4258.31).
const (
	envCreateController = 3

	ctlPutIsVisible = 4
	ctlPutBounds    = 6
	ctlClose        = 24
	ctlGetWebView   = 25

	wvGetSettings             = 3
	wvNavigate                = 5
	wvAddNavigationCompleted  = 15
	wvAddNewWindowRequested   = 44
	setPutIsStatusBarEnabled  = 10
	navGetIsSuccess           = 3
	newWindowGetURI           = 3
	newWindowPutHandled       = 6
	completedHandlerInterface = 4 // QueryInterface, AddRef, Release, Invoke
)

/*
 * handler is a COM object implemented here: WebView2 calls Invoke on it when
 * something it was asked for is ready, or when an event happens.
 *
 * Every handler has the same four methods, so one vtable serves all of them;
 * which handler is meant is the "this" pointer. The arguments of Invoke are
 * (HRESULT, result) for a completion and (sender, args) for an event — both
 * fit invoke(a, b).
 *
 * Handlers are kept in a package list so the collector never frees one that
 * only the WebView2 side still points at.
 */
type handler struct {
	vtbl   *[completedHandlerInterface]uintptr
	invoke func(a uintptr, b *object) uintptr
}

var (
	handlerVtbl = [completedHandlerInterface]uintptr{
		windows.NewCallback(func(this *handler, _ uintptr, out **handler) uintptr {
			*out = this

			return 0
		}),
		windows.NewCallback(func(*handler) uintptr { return 1 }),
		windows.NewCallback(func(*handler) uintptr { return 1 }),
		windows.NewCallback(func(this *handler, a uintptr, b *object) uintptr {
			return this.invoke(a, b)
		}),
	}

	keptMu sync.Mutex
	kept   []*handler
)

func newHandler(invoke func(a uintptr, b *object) uintptr) *handler {
	h := &handler{vtbl: &handlerVtbl, invoke: invoke}

	keptMu.Lock()
	kept = append(kept, h)
	keptMu.Unlock()

	return h
}

/*
 * The one window that is open, if any.
 *
 * hwnd is read by Close, Present and navigation from other goroutines, which
 * only ever post a message to it; everything else here is touched only on the
 * window's own thread, inside the message loop.
 */
var (
	hwnd       atomic.Uintptr
	generation atomic.Int64

	pendingMu  sync.Mutex
	pendingURL string

	current struct {
		env, controller, webview *object
		url                      string
		failure                  error
	}

	// loaded is told whether each navigation succeeded; set by Check.
	loaded func(ok bool)
)

// Available reports whether a native window can be opened here: the loader
// ships beside the program and the WebView2 runtime is installed.
func Available() bool {
	availableOnce.Do(func() { _, availableErr = runtimeVersion() })

	return availableErr == nil
}

// runtimeVersion is the installed WebView2 runtime's version.
func runtimeVersion() (string, error) {
	if err := loadLoader(); err != nil {
		return "", err
	}

	var version *uint16

	hr, _, _ := procAvailableVersion.Call(0, uintptr(unsafe.Pointer(&version)))
	if failed(hr) || version == nil {
		return "", errNoRuntime
	}

	defer windows.CoTaskMemFree(unsafe.Pointer(version))

	return windows.UTF16PtrToString(version), nil
}

var loaderOnce sync.Once
var loaderErr error

/*
 * loadLoader loads the loader by its full path beside the program, never by
 * name: a bare name is looked for in the current directory too, and a DLL
 * planted in a Downloads folder would then run inside the assistant.
 */
func loadLoader() error {
	loaderOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			loaderErr = errNoLoader

			return
		}

		path := filepath.Join(filepath.Dir(exe), LoaderName)
		if _, err := os.Stat(path); err != nil {
			loaderErr = errNoLoader

			return
		}

		dll := windows.NewLazyDLL(path)
		if err := dll.Load(); err != nil {
			loaderErr = fmt.Errorf("the WebView2 loader would not load: %w", err)

			return
		}

		procCreateEnvironment = dll.NewProc("CreateCoreWebView2EnvironmentWithOptions")
		procAvailableVersion = dll.NewProc("GetAvailableCoreWebView2BrowserVersionString")

		if procCreateEnvironment.Find() != nil || procAvailableVersion.Find() != nil {
			loaderErr = errors.New("the WebView2 loader beside the program is not one this build knows")
		}
	})

	return loaderErr
}

// setup does once what every window needs: COM, sharp text on a scaled
// screen, and the window class.
func setup() error {
	setupOnce.Do(func() {
		if err := loadLoader(); err != nil {
			setupErr = err

			return
		}

		// S_FALSE (already initialised on this thread) is success too.
		if hr, _, _ := procCoInitializeEx.Call(0, windows.COINIT_APARTMENTTHREADED); failed(hr) {
			setupErr = fmt.Errorf("COM would not start (0x%08x)", uint32(hr))

			return
		}

		// Per-monitor DPI awareness (v2), where Windows has it: otherwise
		// a scaled screen shows the whole window as a blurred bitmap.
		if procSetDpiAwarenessCtx.Find() == nil {
			procSetDpiAwarenessCtx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		}

		instance, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)

		class := wndClassEx{
			style:      0x0003, // CS_HREDRAW | CS_VREDRAW
			wndProc:    wndProcCallback,
			instance:   instance,
			cursor:     cursor,
			background: colorWindow + 1,
			className:  className,
			icon:       loadIcon(),
		}
		class.size = uint32(unsafe.Sizeof(class))
		class.smallIcon = class.icon

		if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			setupErr = fmt.Errorf("the window class could not be registered: %v", err)
		}
	})

	return setupErr
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	smallIcon  uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

type rect struct{ left, top, right, bottom int32 }

// loadIcon reads the program's icon from beside it, when it ships one. A
// missing icon leaves Windows' default, which is a cosmetic loss only.
func loadIcon() uintptr {
	path := iconPath()
	if path == "" {
		return 0
	}

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}

	// IMAGE_ICON, LR_LOADFROMFILE | LR_DEFAULTSIZE
	icon, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(name)), 1, 0, 0, 0x0010|0x0040)

	return icon
}

// iconPath is pn-scripts-assistant.ico beside the program, or "".
func iconPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	path := filepath.Join(filepath.Dir(exe), paths.Name+".ico")
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	return path
}

/*
 * dataFolder is where WebView2 keeps the page's own storage (the chosen
 * language, a collapsed panel). Never beside an installed program, which may
 * not be writable; beside a portable one, so the stick carries it.
 */
func dataFolder() string {
	if root, ok := paths.PortableRoot(); ok {
		return filepath.Join(filepath.Dir(root), "PN-SCRIPTS-ASSISTANT-WEBVIEW")
	}

	if local, err := os.UserCacheDir(); err == nil {
		return filepath.Join(local, paths.Name, "webview") // %LOCALAPPDATA%
	}

	return filepath.Join(paths.MachineFolder(), "webview")
}

// OpenWithNavigation opens the window and, when a URL arrives on the channel,
// navigates the same window to it.
func OpenWithNavigation(url, title string, width, height int, navigate <-chan string) error {
	go func() {
		for next := range navigate {
			pendingMu.Lock()
			pendingURL = next
			pendingMu.Unlock()

			if h := hwnd.Load(); h != 0 {
				procPostMessageW.Call(h, wmNavigate, 0, 0)
			}
		}
	}()

	return Open(url, title, width, height)
}

/*
 * Open shows url in a window and blocks until the window closes.
 *
 * If the window cannot be made, or WebView2 refuses, the page opens in the
 * system browser instead and Open blocks until Close is called — the same
 * contract, so every caller works unchanged on a machine without WebView2.
 */
func Open(url, title string, width, height int) error {
	if err := openWindow(url, title, width, height); err != nil {
		return fallBack(url)
	}

	return nil
}

var fallbackClosed = make(chan struct{}, 1)

func fallBack(url string) error {
	// A Close left over from an earlier window must not end this one.
	select {
	case <-fallbackClosed:
	default:
	}

	if !OpenInBrowser(url) {
		Alert("PN Scripts Assistant",
			"The window could not be opened, and neither could the browser.\n\n"+
				"Open this address yourself:\n"+url)
	}

	falling.Store(true)
	defer falling.Store(false)

	<-fallbackClosed

	return nil
}

var falling atomic.Bool

func openWindow(url, title string, width, height int) error {
	if err := setup(); err != nil {
		return err
	}

	if !Available() {
		return availableErr
	}

	scale := 1.0
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi > 0 {
			scale = float64(dpi) / 96
		}
	}

	titleW, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}

	instance, _, _ := procGetModuleHandleW.Call(0)

	h, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		wsOverlappedWindow,
		cwUseDefault, cwUseDefault,
		uintptr(int32(float64(width)*scale)), uintptr(int32(float64(height)*scale)),
		0, 0, instance, 0)
	if h == 0 {
		return fmt.Errorf("the window could not be created: %v", callErr)
	}

	gen := generation.Add(1)

	current.env, current.controller, current.webview = nil, nil, nil
	current.url, current.failure = url, nil

	hwnd.Store(h)

	procShowWindow.Call(h, swShow)
	procUpdateWindow.Call(h)
	procSetForegroundWindow.Call(h)

	folder, err := windows.UTF16PtrFromString(dataFolder())
	if err != nil {
		procDestroyWindow.Call(h)
	} else if hr, _, _ := procCreateEnvironment.Call(0, uintptr(unsafe.Pointer(folder)), 0,
		uintptr(unsafe.Pointer(environmentReady(gen, h)))); failed(hr) {
		current.failure = fmt.Errorf("WebView2 refused to start (0x%08x)", uint32(hr))
		procDestroyWindow.Call(h)
	}

	var m msg

	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	hwnd.Store(0)
	runtime.KeepAlive(folder)

	// A window closed while WebView2 was still starting was closed by the
	// person, which is a closed window, not a failure.
	return current.failure
}

// environmentReady is called once WebView2's environment exists; it asks for
// the view inside the window.
func environmentReady(gen int64, h uintptr) *handler {
	return newHandler(func(hr uintptr, env *object) uintptr {
		if gen != generation.Load() || hwnd.Load() != h {
			return 0
		}

		if failed(hr) || env == nil {
			current.failure = fmt.Errorf("WebView2 could not start (0x%08x)", uint32(hr))
			procPostMessageW.Call(h, wmClose, 0, 0)

			return 0
		}

		env.call(1) // AddRef: kept until the window closes
		current.env = env

		if hr := env.call(envCreateController, h, uintptr(unsafe.Pointer(controllerReady(gen, h)))); failed(hr) {
			current.failure = fmt.Errorf("WebView2 could not make a view (0x%08x)", uint32(hr))
			procPostMessageW.Call(h, wmClose, 0, 0)
		}

		return 0
	})
}

// controllerReady fits the view to the window and opens the page.
func controllerReady(gen int64, h uintptr) *handler {
	return newHandler(func(hr uintptr, controller *object) uintptr {
		if gen != generation.Load() || hwnd.Load() != h {
			return 0
		}

		if failed(hr) || controller == nil {
			current.failure = fmt.Errorf("WebView2 could not make a view (0x%08x)", uint32(hr))
			procPostMessageW.Call(h, wmClose, 0, 0)

			return 0
		}

		controller.call(1)
		current.controller = controller

		var webview *object
		if hr := controller.call(ctlGetWebView, uintptr(unsafe.Pointer(&webview))); failed(hr) || webview == nil {
			current.failure = errClosedBeforeReady
			procPostMessageW.Call(h, wmClose, 0, 0)

			return 0
		}

		current.webview = webview

		var settings *object
		if hr := webview.call(wvGetSettings, uintptr(unsafe.Pointer(&settings))); !failed(hr) && settings != nil {
			// The status bar shows link addresses with the port in them;
			// an application has no use for it.
			settings.call(setPutIsStatusBarEnabled, 0)
			settings.release()
		}

		var token int64

		webview.call(wvAddNewWindowRequested, uintptr(unsafe.Pointer(newWindowRequested())), uintptr(unsafe.Pointer(&token)))
		webview.call(wvAddNavigationCompleted, uintptr(unsafe.Pointer(navigationCompleted())), uintptr(unsafe.Pointer(&token)))

		fit(h)
		controller.call(ctlPutIsVisible, 1)
		navigate(current.url)

		return 0
	})
}

/*
 * A link meant for a new window opens in the system browser.
 *
 * The assistant's pages open sources, documents and downloads that way, and
 * WebView2 would otherwise make a second, bare window of its own for each one
 * — with no address bar, no tabs and no way back to the browser somebody
 * actually uses. Only http and https are handed over (OpenInBrowser refuses
 * anything else); the rest are simply not opened.
 */
func newWindowRequested() *handler {
	return newHandler(func(_ uintptr, args *object) uintptr {
		if args == nil {
			return 0
		}

		var uri *uint16
		if hr := args.call(newWindowGetURI, uintptr(unsafe.Pointer(&uri))); !failed(hr) && uri != nil {
			OpenInBrowser(windows.UTF16PtrToString(uri))
			windows.CoTaskMemFree(unsafe.Pointer(uri))
		}

		args.call(newWindowPutHandled, 1)

		return 0
	})
}

func navigationCompleted() *handler {
	return newHandler(func(_ uintptr, args *object) uintptr {
		if loaded == nil || args == nil {
			return 0
		}

		var ok int32

		args.call(navGetIsSuccess, uintptr(unsafe.Pointer(&ok)))
		loaded(ok != 0)

		return 0
	})
}

func navigate(url string) {
	if current.webview == nil || url == "" {
		return
	}

	u, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return
	}

	current.webview.call(wvNavigate, uintptr(unsafe.Pointer(u)))
	runtime.KeepAlive(u)
}

// fit makes the view fill the window's client area.
func fit(h uintptr) {
	if current.controller == nil {
		return
	}

	var r rect

	procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	current.controller.call(ctlPutBounds, uintptr(unsafe.Pointer(&r)))
}

func wndProc(h, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmSize:
		fit(h)

		return 0
	case wmNavigate:
		pendingMu.Lock()
		next := pendingURL
		pendingURL = ""
		pendingMu.Unlock()

		if next != "" {
			current.url = next
			navigate(next)
		}

		return 0
	case wmPresent:
		if iconic, _, _ := procIsIconic.Call(h); iconic != 0 {
			procShowWindow.Call(h, swRestore)
		}

		procSetForegroundWindow.Call(h)

		return 0
	case wmDestroy:
		if current.controller != nil {
			current.controller.call(ctlClose)
		}

		current.webview.release()
		current.controller.release()
		current.env.release()

		procPostQuitMessage.Call(0)

		return 0
	}

	r, _, _ := procDefWindowProcW.Call(h, message, wParam, lParam)

	return r
}

// Close asks the window to shut, so the program can exit. With no window but
// a page in the browser instead, it ends the wait that stands for one.
func Close() {
	if h := hwnd.Load(); h != 0 {
		procPostMessageW.Call(h, wmClose, 0, 0)

		return
	}

	if falling.Load() {
		select {
		case fallbackClosed <- struct{}{}:
		default:
		}
	}
}

// Present brings the window to the front, for a second copy of the program
// that found this one already running.
func Present() {
	if h := hwnd.Load(); h != 0 {
		procPostMessageW.Call(h, wmPresent, 0, 0)
	}
}

/*
 * Check opens url in a window and reports whether the page loaded within the
 * time given, then closes it. It is what `pn-scripts-assistant window-check`
 * runs, so the release can prove on a real Windows that the window works
 * rather than that it compiles.
 *
 * It goes through what the program does with a window: the first page loads,
 * the same window is sent on to a second address (setup handing over to the
 * assistant), it is brought to the front (a second copy starting), and Close
 * from another goroutine ends it (the program being told to stop).
 */
func Check(url string, within time.Duration) error {
	version, err := runtimeVersion()
	if err != nil {
		return err
	}

	second := url + "?then"
	result := make(chan error, 1)
	pages := 0

	finish := func(err error) {
		select {
		case result <- err:
		default:
		}

		go Close()
	}

	loaded = func(ok bool) {
		pages++

		switch {
		case !ok:
			finish(fmt.Errorf("WebView2 %s opened the window but page %d did not load", version, pages))
		case pages == 1:
			pendingMu.Lock()
			pendingURL = second
			pendingMu.Unlock()

			procPostMessageW.Call(hwnd.Load(), wmNavigate, 0, 0)
			Present()
		default:
			finish(nil)
		}
	}
	defer func() { loaded = nil }()

	timer := time.AfterFunc(within, Close)
	defer timer.Stop()

	if err := openWindow(url, "PN Scripts Assistant — window check", 640, 480); err != nil {
		return err
	}

	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("WebView2 %s: %d of 2 pages loaded within %s", version, pages, within)
	}
}

// Engine names what draws the window, for the check's report.
func Engine() string {
	version, err := runtimeVersion()
	if err != nil {
		return "none: " + err.Error()
	}

	return "WebView2 " + version
}
