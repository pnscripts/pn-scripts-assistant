//go:build linux && cgo

package browse

/*
// A page rendered the way a browser renders it, in a process of its own.
//
// This is the only cgo in the program that is not the window, and it runs in
// a child process on purpose. WebKit and GTK both want the main thread and
// neither forgives being driven from anywhere else, and this package already
// carries the scars of getting that wrong: an X error arriving asynchronously
// aborted the whole program at teardown, and whether it died was a race. A
// separate process means the worst a page can do is kill the process that was
// reading it, and the assistant finds out by getting an error.
//
// webkit2gtk-4.1, the same version the window uses. Two versions of WebKit in
// one binary link but do not work.
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <stdlib.h>
#include <string.h>

extern void pnbrainPageRead(char *text);
extern void pnbrainPageFailed(char *why);

// What to run once the page has settled. innerText rather than the HTML,
// because what is wanted is what a person would read — and the HTML of a
// modern page is mostly not that.
static const char *pnbrain_extract =
    "(function(){"
    "  var t = document.body ? document.body.innerText : '';"
    "  return (document.title ? document.title + '\\n\\n' : '') + t;"
    "})()";

static void pnbrain_got_text(GObject *view, GAsyncResult *result, gpointer data) {
    GError *error = NULL;
    JSCValue *value = webkit_web_view_evaluate_javascript_finish(
        WEBKIT_WEB_VIEW(view), result, &error);

    if (error != NULL) {
        pnbrainPageFailed(g_strdup(error->message));
        g_error_free(error);
        gtk_main_quit();
        return;
    }

    char *text = jsc_value_to_string(value);
    pnbrainPageRead(text);
    gtk_main_quit();
}

// Read the page, whenever we decide it has settled. One function for both
// routes in — the load finishing, and the giving-up timer — because two copies
// of the same call is how one of them stops being fixed.
static gboolean pnbrain_read_now(gpointer data) {
    webkit_web_view_evaluate_javascript(
        WEBKIT_WEB_VIEW(data), pnbrain_extract, -1, NULL, NULL, NULL,
        pnbrain_got_text, NULL);
    return G_SOURCE_REMOVE;
}

static void pnbrain_load_changed(WebKitWebView *view, WebKitLoadEvent event, gpointer data) {
    if (event != WEBKIT_LOAD_FINISHED) {
        return;
    }

    // A moment after the load finishes, not at the instant of it. A page that
    // draws itself with scripts has run them by the time load finishes but has
    // often not put the result in the document yet, and reading too early
    // returns the empty shell that made the page look broken rather than slow.
    g_timeout_add(600, pnbrain_read_now, view);
}

static gboolean pnbrain_load_failed(WebKitWebView *view, WebKitLoadEvent event,
                                    gchar *uri, GError *error, gpointer data) {
    pnbrainPageFailed(g_strdup(error ? error->message : "the page would not load"));
    gtk_main_quit();
    return TRUE;
}

static gboolean pnbrain_gave_up(gpointer data) {
    // A page that never finishes loading is the ordinary case rather than a
    // rare one: an advert that polls forever keeps a load open indefinitely.
    // Whatever has been rendered by the time we stop waiting is the answer.
    return pnbrain_read_now(data);
}

static int pnbrain_render(const char *url, int seconds) {
    if (!gtk_init_check(NULL, NULL)) {
        pnbrainPageFailed(g_strdup("there is no display to render a page on"));
        return 1;
    }

    // Offscreen: nothing is shown, and nothing steals focus from whatever the
    // person is actually doing.
    GtkWidget *offscreen = gtk_offscreen_window_new();
    gtk_window_set_default_size(GTK_WINDOW(offscreen), 1280, 900);

    WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new());

    WebKitSettings *settings = webkit_web_view_get_settings(view);
    webkit_settings_set_enable_javascript(settings, TRUE);
    // Nothing is kept between pages: no cookie jar, no cache, no local
    // storage. A browser that remembered would be a browser that could be
    // logged in as somebody, which is not what this is for.
    webkit_settings_set_enable_html5_database(settings, FALSE);
    webkit_settings_set_enable_html5_local_storage(settings, FALSE);
    webkit_settings_set_enable_page_cache(settings, FALSE);
    webkit_settings_set_enable_developer_extras(settings, FALSE);

    gtk_container_add(GTK_CONTAINER(offscreen), GTK_WIDGET(view));
    gtk_widget_show_all(offscreen);

    g_signal_connect(view, "load-changed", G_CALLBACK(pnbrain_load_changed), NULL);
    g_signal_connect(view, "load-failed", G_CALLBACK(pnbrain_load_failed), NULL);

    g_timeout_add_seconds(seconds, pnbrain_gave_up, view);

    webkit_web_view_load_uri(view, url);

    gtk_main();

    return 0;
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// What the child process read, filled by the callbacks below.
var (
	pageText string
	pageWhy  string
)

//export pnbrainPageRead
func pnbrainPageRead(text *C.char) {
	pageText = C.GoString(text)

	C.free(unsafe.Pointer(text))
}

//export pnbrainPageFailed
func pnbrainPageFailed(why *C.char) {
	pageWhy = C.GoString(why)

	C.free(unsafe.Pointer(why))
}

/*
 * RenderHere loads a page and prints what it says.
 *
 * Only ever called in the child process, by the hidden render subcommand. The
 * parent shells out to it rather than doing this itself, so a page that
 * crashes WebKit crashes something disposable.
 */
func RenderHere(url string, seconds int) error {
	target := C.CString(url)
	defer C.free(unsafe.Pointer(target))

	C.pnbrain_render(target, C.int(seconds))

	if pageWhy != "" && pageText == "" {
		/*
		 * Marked, so the parent can find it among GTK's own chatter.
		 *
		 * An offscreen WebKit prints warnings about drawables on every run —
		 * harmless, but they are the last thing on stderr, and the parent used
		 * to report the last line as the reason. So a page that genuinely
		 * failed came back as "drawable is not a native X11 window", which is
		 * true, unrelated, and no help to anybody.
		 */
		fmt.Fprintln(os.Stderr, Marker+pageWhy)

		return fmt.Errorf("%s", pageWhy)
	}

	// Announced, so the parent can tell a page from whatever else might have
	// answered. See browse.Opening.
	fmt.Fprintln(os.Stdout, Opening)
	fmt.Fprint(os.Stdout, pageText)

	return nil
}

// Possible reports whether this build can render a page at all.
func Possible() bool { return true }
