//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <stdlib.h>

static void pn-brain_on_destroy(GtkWidget *widget, gpointer data) {
    gtk_main_quit();
}

// Opens a real GTK window containing a WebKit view and blocks until it closes.
// Written here rather than through a binding library because the maintained Go
// bindings still pkg-config against webkit2gtk-4.0, which Ubuntu 24.04 does not
// ship at all — only 4.1 exists. This is the whole of what PN Brain needs from a
// desktop toolkit: one window, one web view, one URL.
static WebKitWebView *pn-brain_view = NULL;

// Navigation has to happen on the GTK main loop, so a pending URL is left here
// and picked up by a timer rather than being loaded from the Go goroutine that
// produced it. Touching GTK from another thread is undefined behaviour.
static char *pn-brain_pending_url = NULL;
static GMutex pn-brain_pending_lock;

void pn-brain_request_navigation(const char *url) {
    g_mutex_lock(&pn-brain_pending_lock);
    g_free(pn-brain_pending_url);
    pn-brain_pending_url = g_strdup(url);
    g_mutex_unlock(&pn-brain_pending_lock);
}

static gboolean pn-brain_poll_navigation(gpointer data) {
    char *url = NULL;

    g_mutex_lock(&pn-brain_pending_lock);
    if (pn-brain_pending_url != NULL) {
        url = pn-brain_pending_url;
        pn-brain_pending_url = NULL;
    }
    g_mutex_unlock(&pn-brain_pending_lock);

    if (url != NULL && pn-brain_view != NULL) {
        webkit_web_view_load_uri(pn-brain_view, url);
        gtk_window_set_title(GTK_WINDOW(data), "PN Brain");
        g_free(url);
    }

    return G_SOURCE_CONTINUE;
}

static void pn-brain_open_window(const char *url, const char *title, int width, int height) {
    if (!gtk_init_check(NULL, NULL)) {
        return;
    }

    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), title);
    gtk_window_set_default_size(GTK_WINDOW(window), width, height);
    g_signal_connect(window, "destroy", G_CALLBACK(pn-brain_on_destroy), NULL);

    WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new());

    // The console is served from localhost and its own assets only, so nothing
    // here needs to reach the wider web; leaving developer tools available is
    // useful and costs nothing on a single-user desktop app.
    WebKitSettings *settings = webkit_web_view_get_settings(view);
    webkit_settings_set_enable_developer_extras(settings, TRUE);

    pn-brain_view = view;
    webkit_web_view_load_uri(view, url);
    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

    g_timeout_add(200, pn-brain_poll_navigation, window);

    gtk_widget_show_all(window);
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

// openWindowWithNavigation opens the window and, when a URL arrives on the
// channel, navigates the same window to it — so first-run setup hands over to
// the brain in place rather than closing and reopening.
func openWindowWithNavigation(url, title string, width, height int, navigate <-chan string) error {
	go func() {
		for next := range navigate {
			cNext := C.CString(next)
			C.pn - brain_request_navigation(cNext)
			C.free(unsafe.Pointer(cNext))
		}
	}()

	return openWindow(url, title, width, height)
}

func openWindow(url, title string, width, height int) error {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))

	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	C.pn - brain_open_window(cURL, cTitle, C.int(width), C.int(height))

	return nil
}
