//go:build linux && cgo

package window

/*
// x11 as well as gtk: the X error handler below calls into Xlib directly,
// which GTK links but does not export to whoever links GTK.
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1 x11
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <gdk/gdkx.h>
#include <X11/Xlib.h>
#include <stdio.h>
#include <stdlib.h>

// Declared here rather than further down because pnassistant_on_destroy clears it,
// and C reads a file in order.
static WebKitWebView *pnassistant_view = NULL;

// An X error must not kill the process.
//
// GDK installs a handler that aborts, which is a reasonable default for a
// program whose only job is its window and a catastrophic one here. Closing
// the window makes WebKit draw one more frame into a drawable that has just
// gone, X answers BadDrawable, and the abort landed before anything after
// gtk_main() could run — so closing setup half way killed the program on the
// spot, and the rollback that should have removed what it installed never
// started. Whether it died or survived was a race, because X errors arrive
// asynchronously: the same close crashed once and completed the next time.
//
// Reported and survived instead. An X error at teardown is a fact about
// shutdown ordering, not a reason to lose somebody's data — and one during
// normal running is still printed, so it is not hidden either.
//
// Line comments, like everything else in this preamble: it is one Go comment,
// and a nested block comment ends it early — which is exactly what this
// comment did on its first draft.
static int pnassistant_x_error(Display *display, XErrorEvent *event) {
    char text[256];

    XGetErrorText(display, event->error_code, text, sizeof(text));
    fprintf(stderr, "  (window: %s — carrying on)\n", text);

    return 0;
}

// The ids of the callbacks that hold a pointer to the window, so they can be
// taken off it when it goes away. See pnassistant_on_destroy.
static guint pnassistant_poll_id = 0;
static guint pnassistant_maximise_id = 0;
static guint pnassistant_show_id = 0;

static void pnassistant_forget_sources(void) {
    if (pnassistant_poll_id != 0) { g_source_remove(pnassistant_poll_id); pnassistant_poll_id = 0; }
    if (pnassistant_maximise_id != 0) { g_source_remove(pnassistant_maximise_id); pnassistant_maximise_id = 0; }
    if (pnassistant_show_id != 0) { g_source_remove(pnassistant_show_id); pnassistant_show_id = 0; }
}

// Three callbacks are left holding this window: a poll every 200ms, an idle
// that maximises it, and a timeout at 600ms that shows it. None of them were
// taken off when the window was destroyed, so the next tick after a close
// worked on freed memory and the program died with BadDrawable — on the most
// ordinary path there is, somebody pressing the X.
//
// It died before anything after gtk_main() could run, which is why closing
// setup half way through left everything it had installed on the machine: the
// code that removes it again never got to start.
static void pnassistant_on_destroy(GtkWidget *widget, gpointer data) {
    pnassistant_forget_sources();
    pnassistant_view = NULL;
    gtk_main_quit();
}

// Closing the window ends the loop; it does not tear the window down.
//
// Destroying it takes the WebKit view with it, and WebKit is usually drawing
// when somebody presses the X. It then disconnects signal handlers from an
// instance that has already gone, ends a frame clock that is no longer one,
// and finally draws into a window that does not exist — which X answers with
// BadDrawable and GDK turns into an abort.
//
// The program died right there, before anything after gtk_main() could run.
// That is why closing setup half way left everything it had installed sitting
// on the machine: the code that removes it again never reached its first line.
//
// Nothing here needs destroying. The loop is ending because the program is
// either exiting or about to open a different window, and the process tears
// all of this down more safely than GTK manages at this particular moment.
// Returning TRUE stops the default handler, which is the one that destroys.
static gboolean pnassistant_on_delete(GtkWidget *widget, GdkEvent *event, gpointer data) {
    pnassistant_forget_sources();

    // The view first: unmapping it is what stops WebKit compositing into a
    // drawable that is about to stop existing.
    if (pnassistant_view != NULL) {
        gtk_widget_hide(GTK_WIDGET(pnassistant_view));
    }

    gtk_widget_hide(widget);
    gtk_main_quit();

    return TRUE;
}

// Opens a real GTK window containing a WebKit view and blocks until it closes.
// Written here rather than through a binding library because the maintained Go
// bindings still pkg-config against webkit2gtk-4.0, which Ubuntu 24.04 does not
// ship at all — only 4.1 exists. This is the whole of what PN Scripts Assistant needs from a
// desktop toolkit: one window, one web view, one URL.

// What the window is called, kept so that navigating within it does not
// rename it. It used to be reset to a literal product name on every
// navigation, which meant a window titled with whatever its owner had called
// their assistant silently became something else the first time it moved.
static char *pnassistant_title = NULL;

// Navigation has to happen on the GTK main loop, so a pending URL is left here
// and picked up by a timer rather than being loaded from the Go goroutine that
// produced it. Touching GTK from another thread is undefined behaviour.
static char *pnassistant_pending_url = NULL;
static gboolean pnassistant_pending_present = FALSE;
static gboolean pnassistant_pending_quit = FALSE;
static GMutex pnassistant_pending_lock;

// Asked for by a second copy of the program that has just been started and is
// about to exit, so that double-clicking the icon again brings this window
// forward instead of doing nothing visible.
// Asked for when the program has been told to stop.
//
// Without this, a signal cancels everything on the Go side while gtk_main()
// carries on holding the process open — and a process that has stopped serving
// but has not exited still holds the lock on its data root, so the next copy
// will not start either. Measured once at eleven minutes, asleep in a graphics
// wait, refusing to let anything else run.
//
// Line comments, not a block: this whole preamble is one C comment, and a
// nested block comment ends it early and takes the rest of the file with it.
void pnassistant_request_quit(void) {
    g_mutex_lock(&pnassistant_pending_lock);
    pnassistant_pending_quit = TRUE;
    g_mutex_unlock(&pnassistant_pending_lock);
}

void pnassistant_request_present(void) {
    g_mutex_lock(&pnassistant_pending_lock);
    pnassistant_pending_present = TRUE;
    g_mutex_unlock(&pnassistant_pending_lock);
}

void pnassistant_request_navigation(const char *url) {
    g_mutex_lock(&pnassistant_pending_lock);
    g_free(pnassistant_pending_url);
    pnassistant_pending_url = g_strdup(url);
    g_mutex_unlock(&pnassistant_pending_lock);
}

static gboolean pnassistant_poll_navigation(gpointer data) {
    char *url = NULL;
    gboolean present = FALSE;
    gboolean quit = FALSE;

    g_mutex_lock(&pnassistant_pending_lock);
    if (pnassistant_pending_url != NULL) {
        url = pnassistant_pending_url;
        pnassistant_pending_url = NULL;
    }
    present = pnassistant_pending_present;
    pnassistant_pending_present = FALSE;
    quit = pnassistant_pending_quit;
    // Taken, not just read — the same as present above, and for a reason that
    // cost a whole first run.
    //
    // Setup and the brain are two windows in one process: setup opens one,
    // Continue calls request_quit to close it, and the brain then opens
    // another. The flag was never cleared, so the new window's poll timer read
    // a quit left over from a button pressed a second earlier and shut it
    // 200ms after it appeared. The program printed its banner, said which
    // model it was using, and exited without a word — which from outside is
    // "I clicked Continue and it did not start".
    pnassistant_pending_quit = FALSE;
    g_mutex_unlock(&pnassistant_pending_lock);

    if (quit) {
        // The window goes with the loop.
        //
        // gtk_main_quit only stops the loop; the window it was running stays
        // realised and on screen. Setup and the brain are two windows in one
        // process, so handing over left the finished setup sitting there while
        // the brain opened a second window beside it — two windows with the
        // same name, one of them dead, and no way to tell which was which.
        //
        // The view is forgotten first: destroying the window destroys the view
        // inside it, and a navigation arriving in that moment would otherwise
        // be handed a pointer to freed memory.
        pnassistant_view = NULL;
        g_free(pnassistant_title);
        pnassistant_title = NULL;

        // This source is the one running, and returning G_SOURCE_REMOVE below
        // takes it off. Forgetting its id first stops the teardown removing it
        // a second time from underneath GLib.
        pnassistant_poll_id = 0;

        // Hidden, not destroyed — the same reason as pnassistant_on_delete. The
        // window going away is what matters here; destroying it is what
        // crashes. Setup handing over to the brain reaches this line, and it
        // used to leave a dead setup window on screen beside the new one.
        if (pnassistant_view != NULL) {
            gtk_widget_hide(GTK_WIDGET(pnassistant_view));
        }

        if (data != NULL) {
            gtk_widget_hide(GTK_WIDGET(data));
        }

        gtk_main_quit();

        return G_SOURCE_REMOVE;
    }

    if (present) {
        // Deiconify first, then present.
        //
        // gtk_window_present alone does not reliably restore a window that has
        // been minimised: the second copy reported "brought it to the front",
        // exited, and the window stayed exactly where it was. Somebody who
        // minimised PN Scripts Assistant and then pressed its icon got nothing at all, and
        // no way to find out why, because the only explanation went to a
        // terminal they had not opened. That is indistinguishable from the
        // program being broken, and it is what "it will not open" means.
        //
        // show before deiconify because a window that was never mapped cannot
        // be deiconified, and present_with_time carries a timestamp so the
        // window manager treats this as a user action rather than an
        // application stealing focus, which GNOME otherwise answers by only
        // flashing the launcher.
        GtkWindow *win = GTK_WINDOW(data);

        gtk_widget_show_all(GTK_WIDGET(win));
        gtk_window_deiconify(win);
        gtk_window_present_with_time(win, (guint32)(g_get_real_time() / 1000));
    }

    if (url != NULL && pnassistant_view != NULL) {
        webkit_web_view_load_uri(pnassistant_view, url);
        gtk_window_set_title(GTK_WINDOW(data), pnassistant_title);
        g_free(url);
    }

    return G_SOURCE_CONTINUE;
}

// Runs once, after the main loop has mapped the window.
static gboolean pnassistant_maximise_once(gpointer data) {
    GtkWindow *win = GTK_WINDOW(data);

    gtk_window_maximize(win);

    // Its own id, forgotten as it finishes. A one-shot source removes itself
    // by returning this, so the id kept for pnassistant_on_destroy is stale the
    // moment it does — and removing a stale id is a GLib critical every time
    // a window closes normally.
    pnassistant_maximise_id = 0;

    return G_SOURCE_REMOVE;
}

// Make sure the window is actually on the screen, after the desktop has had
// its turn.
//
// Line comments, not a block: this is inside the cgo preamble, which is itself
// one big block comment, so a nested block comment ends the preamble at its
// terminator rather than ending the comment. The file has been broken that way
// before — including by a comment written to warn about it, which contained
// the terminator as an example.
//
// GNOME remembers how an application's window was last left and applies that
// after the window is mapped — later than any idle callback, which is why
// deiconifying from one changed nothing. Once PN Scripts Assistant had been minimised,
// every launch after that opened minimised: the process started, served its
// interface, answered on its port, and put nothing on the screen. From the
// outside that is a program that does not open, and there is no way to tell it
// apart from one that is broken. Starting it again reproduces it exactly,
// because the remembered state is the thing being restored.
//
// Once, and only at startup. A window somebody minimises a minute later is a
// window they wanted minimised, and a program that refuses to stay out of the
// way is worse than one that opens small.
static gboolean pnassistant_show_once(gpointer data) {
    GtkWindow *win = GTK_WINDOW(data);

    gtk_window_deiconify(win);
    gtk_window_present(win);

    pnassistant_show_id = 0;

    return G_SOURCE_REMOVE;
}

static void pnassistant_open_window(const char *url, const char *title, int width, int height, const char *icon_path) {
    if (!gtk_init_check(NULL, NULL)) {
        return;
    }

    // After gtk_init, which installs GDK's own. See pnassistant_x_error.
    XSetErrorHandler(pnassistant_x_error);

    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), title);

    // The icon the switcher and the dock show.
    //
    // The path is worked out in Go, which can look in several places without
    // this file learning about any of them. It used to read APPDIR directly,
    // which only exists inside an AppImage — so every build from source ran
    // with the blank default icon, including the setup window, which is the
    // very first thing anybody sees on a new machine.
    if (icon_path != NULL && icon_path[0] != 0) {
        gtk_window_set_icon_from_file(GTK_WINDOW(window), icon_path, NULL);
    }
    // The screen's size, not a guess at one.
    //
    // Maximising is deferred to an idle callback because asking before the
    // window is mapped is ignored, and until that fires the window is whatever
    // default size was passed in — so opening the program showed a small
    // rectangle of interface in the corner of a large dark window for a second
    // or two, which is the thing the white background was only half of.
    //
    // Starting at the size of the work area means the first frame is already
    // the right shape, and the maximise that follows only formalises it.
    GdkRectangle work_area = {0, 0, width, height};
    GdkDisplay *display = gdk_display_get_default();

    if (display != NULL) {
        GdkMonitor *monitor = gdk_display_get_primary_monitor(display);

        if (monitor == NULL && gdk_display_get_n_monitors(display) > 0) {
            monitor = gdk_display_get_monitor(display, 0);
        }

        if (monitor != NULL) {
            gdk_monitor_get_workarea(monitor, &work_area);
        }
    }

    gtk_window_set_default_size(GTK_WINDOW(window),
        work_area.width > 0 ? work_area.width : width,
        work_area.height > 0 ? work_area.height : height);
    g_signal_connect(window, "destroy", G_CALLBACK(pnassistant_on_destroy), NULL);
    g_signal_connect(window, "delete-event", G_CALLBACK(pnassistant_on_delete), NULL);

    WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new());

    // The console is served from localhost and its own assets only, so nothing
    // here needs to reach the wider web; leaving developer tools available is
    // useful and costs nothing on a single-user desktop app.
    WebKitSettings *settings = webkit_web_view_get_settings(view);
    webkit_settings_set_enable_developer_extras(settings, TRUE);

    // The acceleration policy is deliberately left alone.
    //
    // It was set to ALWAYS on the theory that the page was being composited in
    // software, because no separate GPU process appears in the process list.
    // That theory was wrong: WebKitGTK does this inside the web process, and it
    // already holds five handles on /dev/dri and has EGL, GLES and gbm mapped.
    // The card was in use the whole time.
    //
    // Forcing the policy measurably made things worse — the renderer went from
    // about a fifth of a processor core to nearly half — so the default stands.

    // The dark ground, before there is anything to draw on it.
    //
    // A WebKitWebView paints white until the page has rendered, so opening the
    // program flashed a white rectangle the size of the default window inside
    // an already-maximised dark one, for the second or two it takes to load the
    // console and get the first paint out. On a screen at night that is not a
    // loading state, it is a light going on in your face, and it made a working
    // program look broken.
    //
    // The same colour as the page's own --bg, so the window is one shade from
    // the moment it appears and the interface fades in on top of it rather than
    // replacing something else.
    //
    // Written with line comments, like everything else in this preamble: a
    // block comment here closes the Go comment the whole C file lives in, and
    // the compiler then reads C as Go.
    GdkRGBA ground = {0x04 / 255.0, 0x08 / 255.0, 0x0f / 255.0, 1.0};
    webkit_web_view_set_background_color(view, &ground);

    // And the window behind it, for the instant before the view is mapped and
    // for any edge the view does not cover while it is being resized.
    GtkCssProvider *ground_style = gtk_css_provider_new();
    gtk_css_provider_load_from_data(ground_style,
        "window, .background { background-color: #04080f; }", -1, NULL);
    gtk_style_context_add_provider_for_screen(gdk_screen_get_default(),
        GTK_STYLE_PROVIDER(ground_style), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    g_object_unref(ground_style);

    // No stale request outlives the window that made it. request_quit and
    // request_present can both be called while no window exists — a signal
    // during startup, a second copy arriving early — and either one left set
    // would act on the next window instead of the one it was meant for.
    g_mutex_lock(&pnassistant_pending_lock);
    pnassistant_pending_quit = FALSE;
    pnassistant_pending_present = FALSE;
    g_free(pnassistant_pending_url);
    pnassistant_pending_url = NULL;
    g_mutex_unlock(&pnassistant_pending_lock);

    pnassistant_view = view;
    pnassistant_title = g_strdup(title);
    webkit_web_view_load_uri(view, url);

    // Filling the window, not sitting at its default size inside it.
    //
    // gtk_container_add gives the child the whole container, which is what was
    // wanted, but the view was still being asked for the default size while the
    // maximise request waited for an idle callback. Expanding says the answer
    // is "all of it" whichever order those happen in.
    gtk_widget_set_hexpand(GTK_WIDGET(view), TRUE);
    gtk_widget_set_vexpand(GTK_WIDGET(view), TRUE);

    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

    pnassistant_poll_id = g_timeout_add(200, pnassistant_poll_navigation, window);

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
    pnassistant_maximise_id = g_idle_add(pnassistant_maximise_once, window);

    // Late enough that the desktop has finished restoring its remembered
    // state, which is what would otherwise leave a new window minimised.
    pnassistant_show_id = g_timeout_add(600, pnassistant_show_once, window);

    gtk_main();
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/paths"
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
			C.pnassistant_request_navigation(cNext)
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

	cIcon := C.CString(iconPath())
	defer C.free(unsafe.Pointer(cIcon))

	C.pnassistant_open_window(cURL, cTitle, C.int(width), C.int(height), cIcon)

	return nil
}

// Close asks the window to shut, so the program can exit.
//
// The main loop owns the process: until it returns, nothing else does, and a
// program that has stopped serving but has not exited is worse than one still
// running — it holds its data root and stops the next copy from starting.
func Close() {
	C.pnassistant_request_quit()
}

// Present brings the window to the front.
//
// Called when a second copy of the program is started: rather than opening
// another window onto the same brain, the one already running comes forward and
// the new process exits.
func Present() {
	C.pnassistant_request_present()
}

// Available reports whether a native window can be opened by this build.
func Available() bool { return true }

/*
 * iconPath finds the window icon, or returns "" if there is none.
 *
 * It used to be read straight from APPDIR in C, which meant only an AppImage
 * ever had one: every build from source showed the blank default, including
 * the setup window — the first thing anybody sees on a new machine, and the
 * one most worth looking like it belongs to something.
 *
 * Looked for in the places it actually lives, nearest first. Returning empty
 * rather than failing is deliberate: a missing icon is a cosmetic loss and
 * refusing to open the window over it would not be.
 */
func iconPath() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}

	home, _ := os.UserHomeDir()

	return findIcon(os.Getenv("APPDIR"), exe, home, func(path string) bool {
		info, err := os.Stat(path)

		return err == nil && !info.IsDir()
	})
}

/*
 * findIcon is where the looking happens, given the places rather than reading
 * them from the machine.
 *
 * Separated so it can be checked at all. Asking os.Executable in a test gives
 * the path of the test binary in a temporary directory, so a test of the real
 * function measures the test harness and nothing else — it reported no icon
 * while the program running beside it would have found one.
 */
func findIcon(appdir, exe, home string, exists func(string) bool) string {
	var places []string

	icon := paths.Name + ".png"

	// Inside an AppImage, where it is unpacked beside the binary.
	if appdir != "" {
		places = append(places, filepath.Join(appdir, icon))
	}

	if exe != "" {
		beside := filepath.Dir(exe)

		places = append(places,
			// Beside the binary, as a packaged build lays it out.
			filepath.Join(beside, icon),
			// And in the project's assets, which is where a build from source
			// finds itself: dist/pn-scripts-assistant, assets/pn-scripts-assistant.png.
			filepath.Join(filepath.Dir(beside), "assets", icon),
		)
	}

	// Installed for the desktop, which is where the menu entry points — under
	// the old name too, for a machine that installed it before the rename and
	// has not started since.
	if home != "" {
		for _, name := range []string{icon, paths.LegacyName + ".png"} {
			places = append(places,
				filepath.Join(home, ".local", "share", "icons", name),
				filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps", name),
			)
		}
	}

	for _, path := range places {
		if exists(path) {
			return path
		}
	}

	return ""
}
