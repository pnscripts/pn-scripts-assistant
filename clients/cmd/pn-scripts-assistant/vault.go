package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/logs"
	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/brain/profile"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/vault"
	"pn-scripts-assistant/internal/brain/voiceprint"
	"pn-scripts-assistant/internal/brain/window"
)

/*
 * Opening a sealed brain (ADR 0008).
 *
 * Every way into a brain goes through openDB, and openDB goes through here:
 * find the key in this machine's keyring, or give a brain that has none its
 * key; seal whatever is still in the clear; and only then open the database.
 * A brain carried to a machine that has never seen it needs its recovery key,
 * asked for in a window of its own the way setup is — or, from a terminal,
 * with "pn-scripts-assistant unlock".
 */

// askForRecovery is how this run asks for a recovery key, or nil when it
// cannot ask (a one-off command), in which case it says how to unlock.
var askForRecovery func(root paths.Root) (vault.Unlocked, error)

func unlockBrain(root paths.Root) (vault.Unlocked, error) {
	ring := vault.System()

	// A database already sealed with a key and no record of that key is not
	// given a new one: everything in it would become unreadable.
	mayCreate := store.SealedWith(root.DatabasePath()) == ""

	u, err := vault.Unlock(root.Path, ring, mayCreate)

	if errors.Is(err, vault.ErrNeedsRecovery) {
		if askForRecovery == nil {
			return u, fmt.Errorf("%w.\n  Open the app to type it in, or run: %s unlock", err, paths.Name)
		}

		u, err = askForRecovery(root)
	}

	if err != nil {
		return u, err
	}

	vault.Use(u.Key)
	vault.Remember(u)

	return u, nil
}

/*
 * sealTheRest seals the files beside the database that hold something
 * personal, once, in place: the profile, the integrations' credentials, the
 * voiceprint and the secrets in the settings file. Each is written whole and
 * moved into place, so a power cut leaves the old file or the new one.
 */
func sealTheRest(root string, k *vault.Key) error {
	for _, path := range []string{
		profile.Path(root),
		filepath.Join(root, mcp.FolderName, "secrets.json"),
		filepath.Join(voiceprint.Folder(root), voiceprint.KnownFile),
	} {
		if err := vault.SealFileInPlace(path, k); err != nil {
			return fmt.Errorf("sealing %s: %w", filepath.Base(path), err)
		}
	}

	return config.SealFile(config.Path(root))
}

// runUnlock is the terminal way in on a new machine.
func runUnlock(args []string) error {
	fs := flag.NewFlagSet("unlock", flag.ExitOnError)
	fs.Parse(args)

	root, err := paths.FindOrCreate()
	if err != nil {
		return err
	}

	fmt.Printf("\n  This brain (%s) is encrypted. Type its recovery key — 52 letters and\n"+
		"  digits in groups of four — and press Enter:\n\n  > ", root.Path)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')

	u, err := vault.Recover(root.Path, vault.System(), line)
	if err != nil {
		return err
	}

	fmt.Printf("\n  Unlocked. The key is now kept in %s, so it opens here without asking.\n\n", u.Keyring)

	return nil
}

// runReadLog prints this program's log with its sealed lines opened.
func runReadLog(args []string) error {
	db, _, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	raw, err := os.ReadFile(logs.File())
	if err != nil {
		return err
	}

	fmt.Print(vault.OpenLines(vault.Active(), string(raw)))

	return nil
}

/*
 * recoveryWindow asks for the recovery key in a window of its own, on
 * loopback, and waits until it opens the brain or the window is closed.
 */
func recoveryWindow(root paths.Root, serveOnly bool) (vault.Unlocked, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return vault.Unlocked{}, err
	}

	var (
		once   sync.Once
		done   = make(chan struct{})
		result vault.Unlocked
	)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, recoveryPage(root.Path, ""))
	})

	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		u, err := vault.Recover(root.Path, vault.System(), r.PostFormValue("key"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err != nil {
			fmt.Fprint(w, recoveryPage(root.Path, err.Error()))

			return
		}

		result = u

		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><body style="font:16px system-ui;`+
			`background:#0b1220;color:#dbe6f5;padding:40px"><p>Unlocked. The key is now kept in `+
			html.EscapeString(u.Keyring)+`. Opening the brain.</p></body>`)

		once.Do(func() { close(done) })
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	url := "http://" + ln.Addr().String() + "/"

	fmt.Printf("\n  This brain is encrypted and this machine does not have its key yet.\n"+
		"  Type the recovery key at %s\n\n", url)

	if serveOnly || !window.Available() {
		if !serveOnly {
			window.OpenInBrowser(url)
		}

		<-done

		return result, nil
	}

	go func() {
		<-done
		time.Sleep(1500 * time.Millisecond)
		window.Close()
	}()

	if err := window.Open(url, "Unlock this brain", 640, 520); err != nil {
		return vault.Unlocked{}, err
	}

	select {
	case <-done:
		return result, nil
	default:
		return vault.Unlocked{}, errors.New("the window was closed before the brain was unlocked")
	}
}

func recoveryPage(root, problem string) string {
	note := ""
	if problem != "" {
		note = `<p class="problem">` + html.EscapeString(problem) + `</p>`
	}

	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Unlock this brain</title>
<style>
body{font:16px/1.5 system-ui,sans-serif;background:#0b1220;color:#dbe6f5;margin:0;padding:32px}
main{max-width:560px;margin:auto}
h1{font-size:20px;font-weight:600}
textarea{width:100%;box-sizing:border-box;font:15px ui-monospace,monospace;padding:10px;
background:#111a2c;color:#dbe6f5;border:1px solid #2b3b5a;border-radius:6px}
button{margin-top:12px;padding:10px 18px;font:inherit;background:#2e6bd6;color:#fff;border:0;border-radius:6px}
.problem{color:#ffb4a8}.where{color:#8aa0c0;font-size:14px}
</style></head><body><main>
<h1>This brain is encrypted</h1>
<p>Everything it knows is sealed with a key that this computer does not have yet —
which is what keeps it unreadable on a drive that has been lost or copied.
It opens here with the recovery key you were shown when it was first sealed:
52 letters and digits, in groups of four.</p>
<p class="where">` + html.EscapeString(root) + `</p>` + note + `
<form method="post"><textarea name="key" rows="3" autofocus
placeholder="XXXX-XXXX-XXXX-…"></textarea><button type="submit">Unlock</button></form>
<p class="where">Once it opens, this computer keeps the key in its own keyring and will not ask again.</p>
</main></body></html>`
}

// keyringSummary is one line about where the key is kept, for the status.
func keyringSummary() string {
	u := vault.Remembered()

	if strings.TrimSpace(u.Keyring) == "" {
		return "not sealed"
	}

	return "sealed; key in " + u.Keyring
}
