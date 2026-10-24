package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The trust layer's pages: what it did and why, and the switch for citing.
 *
 * Both read records this program keeps, never the model's account of itself.
 * See brain/trust.go for how they are written.
 */

/*
 * handleDeeds is "What I did and why": every call, newest first, with the
 * request that led to it, who acted, the model and why that model, what came
 * of it and whether anything left the machine.
 *
 * Filtered on the server rather than the page, so a month of work on a slow
 * machine is not sent to be thrown away.
 */
func (s *Server) handleDeeds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := store.DeedFilter{
		Changes: q.Get("changes") == "1",
		Leaves:  q.Get("leaves") == "1",
		Limit:   200,
	}

	switch q.Get("since") {
	case "day":
		f.Since = time.Now().Add(-24 * time.Hour)
	case "week", "":
		f.Since = time.Now().Add(-7 * 24 * time.Hour)
	case "month":
		f.Since = time.Now().Add(-31 * 24 * time.Hour)
	case "all":
	default:
		fail(w, http.StatusBadRequest, "since is day, week, month or all")

		return
	}

	if c := q.Get("conversation"); c != "" {
		id, err := strconv.ParseInt(c, 10, 64)
		if err != nil || id <= 0 {
			fail(w, http.StatusBadRequest, "That is not a conversation id.")

			return
		}

		f.Conversation = id
	}

	deeds, err := s.brain.DB.Deeds(f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// The counts the page leads with, over what was asked for.
	counts := map[string]int{"all": len(deeds)}

	for _, d := range deeds {
		counts[d.Outcome]++

		if d.Changes {
			counts["changes"]++
		}

		if d.Leaves != "" {
			counts["leaves"]++
		}
	}

	ok(w, map[string]any{"deeds": deeds, "counts": counts, "privacy": s.brain.Cfg.Privacy})
}

/*
 * handleCiteSources switches citing on or off.
 *
 * From the Control page only, like every other setting about what the
 * assistant tells its owner: a conversation cannot ask it to stop saying
 * where its answers came from.
 */
func (s *Server) handleCiteSources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "that could not be read: "+err.Error())

		return
	}

	s.brain.Cfg.CiteSources = body.On

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "that could not be saved: "+err.Error())

		return
	}

	ok(w, map[string]any{"cite_sources": s.brain.Cfg.CiteSources})
}

/*
 * handleUILanguage is which language the pages are shown in: Bulgarian when
 * the assistant has been told its owner speaks Bulgarian, English otherwise.
 * The page asks once and falls back to the browser's own language.
 */
func (s *Server) handleUILanguage(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"language": uiLanguage(s.brain.Cfg.Language)})
}

func uiLanguage(spoken string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(spoken)), "bg") {
		return "bg"
	}

	if strings.TrimSpace(spoken) == "" {
		return ""
	}

	return "en"
}

/*
 * handleOpenSource opens one source the way somebody would: an address in
 * their own browser, a file's folder in their file manager.
 *
 * The native window cannot follow a link itself — it would leave the
 * assistant for the page — so the page asks for it here. Only on a click,
 * only http and https, and a file is shown in its folder rather than opened:
 * a source can be anything a tool read, and "open" on a program would run it.
 */
func (s *Server) handleOpenSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Where string `json:"where"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "that could not be read: "+err.Error())

		return
	}

	target, err := openable(body.Where)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if err := openOutside(target); err != nil {
		fail(w, http.StatusInternalServerError, "it could not be opened: "+err.Error())

		return
	}

	ok(w, map[string]any{"opened": target})
}

// openable is what may be opened for a source: a web address as it is, or
// the folder a file is in.
func openable(where string) (string, error) {
	where = strings.TrimSpace(where)

	if u, err := url.Parse(where); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return u.String(), nil
	}

	if !filepath.IsAbs(where) {
		return "", fmt.Errorf("that is not a web address or a file on this machine")
	}

	info, err := os.Stat(where)
	if err != nil {
		return "", fmt.Errorf("that file is no longer there")
	}

	if !info.IsDir() {
		where = filepath.Dir(where)
	}

	return where, nil
}

/*
 * openOutside hands target to the system's own opener, without a shell.
 *
 * Without one on purpose: an address came from a web page, and on Windows
 * "cmd /c start" reads & and | in it as commands. rundll32's handler takes the
 * address as one argument and nothing else. Replaceable, so a test can check
 * what would be opened without opening anything on the machine running it.
 */
var openOutside = func(target string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() { _ = cmd.Wait() }()

	return nil
}
