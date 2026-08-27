//go:build linux && cgo

package window

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <stdlib.h>

static void pnbrain_on_destroy(GtkWidget *widget, gpointer data) {
    gtk_main_quit();
}

// Opens a real GTK window containing a WebKit view and blocks until it closes.
// Written here rather than through a binding library because the maintained Go
// bindings still pkg-config against webkit2gtk-4.0, which Ubuntu 24.04 does not
// ship at all — only 4.1 exists. This is the whole of what PN Brain needs from a
// desktop toolkit: one window, one web view, one URL.
static WebKitWebView *pnbrain_view = NULL;

// Navigation has to happen on the GTK main loop, so a pending URL is left here
// and picked up by a timer rather than being loaded from the Go goroutine that
// produced it. Touching GTK from another thread is undefined behaviour.
static char *pnbrain_pending_url = NULL;
static GMutex pnbrain_pending_lock;

void pnbrain_request_navigation(const char *url) {
    g_mutex_lock(&pnbrain_pending_lock);
    g_free(pnbrain_pending_url);
    pnbrain_pending_url = g_strdup(url);
    g_mutex_unlock(&pnbrain_pending_lock);
}

static gboolean pnbrain_poll_navigation(gpointer data) {
    char *url = NULL;

    g_mutex_lock(&pnbrain_pending_lock);
    if (pnbrain_pending_url != NULL) {
        url = pnbrain_pending_url;
        pnbrain_pending_url = NULL;
    }
    g_mutex_unlock(&pnbrain_pending_lock);

    if (url != NULL && pnbrain_view != NULL) {
        webkit_web_view_load_uri(pnbrain_view, url);
        gtk_window_set_title(GTK_WINDOW(data), "PN Brain");
        g_free(url);
    }

    return G_SOURCE_CONTINUE;
}

// Runs once, after the main loop has mapped the window.
static gboolean pnbrain_maximise_once(gpointer data) {
    gtk_window_maximize(GTK_WINDOW(data));

    return G_SOURCE_REMOVE;
}

static void pnbrain_open_window(const char *url, const char *title, int width, int height) {
    if (!gtk_init_check(NULL, NULL)) {
        return;
    }

    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), title);
    gtk_window_set_default_size(GTK_WINDOW(window), width, height);
    g_signal_connect(window, "destroy", G_CALLBACK(pnbrain_on_destroy), NULL);

    WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new());

    // The console is served from localhost and its own assets only, so nothing
    // here needs to reach the wider web; leaving developer tools available is
    // useful and costs nothing on a single-user desktop app.
    WebKitSettings *settings = webkit_web_view_get_settings(view);
    webkit_settings_set_enable_developer_extras(settings, TRUE);

    pnbrain_view = view;
    webkit_web_view_load_uri(view, url);
    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

    g_timeout_add(200, pnbrain_poll_navigation, window);

    // Maximised, not fullscreen. The brain map wants room and the rail is a
    // column of readouts, so a small window wastes both — but fullscreen takes
    // the title bar and the way out with it, which is a different promise than
    // somebody double-clicking an icon is making.
    gtk_widget_show_all(window);

    // Maximised once the window actually exists.
    //
    // Asking before it is mapped is ignored — mutter kept the default size
    // twice over, once before show_all and once immediately after, because the
    // window had been shown but not yet mapped. An idle callback runs after the
    // main loop has done that, which is the first moment the request means
    // anything.
    //
    // Maximised rather than fullscreen: the map wants room and the rails are
    // columns of readouts, so a small window wastes both — but fullscreen takes
    // the title bar and the way out with it, which is more than double-clicking
    // an icon asks for.
    g_idle_add(pnbrain_maximise_once, window);
    gtk_main();
}
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func init() {
	// GTK must be driven from the thread it was initialised on, and Go is free
	// to move goroutines between threads unless told otherwise.
	runtime.LockOSThread()
}

// OpenWithNavigation opens the window and, when a URL arrives on the channel,
// navigates the same window to it — so first-run setup hands over to the brain
// in place rather than closing one window and opening another.
func OpenWithNavigation(url, title string, width, height int, navigate <-chan string) error {
	go func() {
		for next := range navigate {
			cNext := C.CString(next)
			C.pnbrain_request_navigation(cNext)
			C.free(unsafe.Pointer(cNext))
		}
	}()

	return Open(url, title, width, height)
}

func Open(url, title string, width, height int) error {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))

	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	C.pnbrain_open_window(cURL, cTitle, C.int(width), C.int(height))

	return nil
}

// Available reports whether a native window can be opened by this build.
func Available() bool { return true }
