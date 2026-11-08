package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/latest"
	"pn-scripts-assistant/internal/brain/release"
	"pn-scripts-assistant/internal/brain/selfupdate"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * What is out of date, everywhere, in one answer.
 *
 * The pieces were each checked somewhere: the parts list knew whether Ollama
 * had a newer release, the Godot tool asked its own project, the program knew
 * its own commit and told nobody. So "is anything out of date" was a question
 * with four answers in four places and no way to ask it once.
 *
 * One check, and it says how many rather than that there are some — a number
 * is something a person can decide about and "updates available" is a badge
 * they learn to ignore.
 */

// Upkeep is one thing that could be brought up to date.
type Upkeep struct {
	Name string `json:"name"`

	// Have and Latest, both, because "out of date" without the two versions is
	// an assertion rather than a fact.
	Have   string `json:"have,omitempty"`
	Latest string `json:"latest,omitempty"`

	Newer bool `json:"newer"`

	// How it is brought up to date, so the interface can offer the right
	// button rather than guessing from the name.
	How string `json:"how"`

	// Why anything could not be determined. A check that quietly reports "up
	// to date" because it never looked is worse than one that says so.
	Why string `json:"why,omitempty"`

	// For the program itself: how this copy was installed (latest.Channel),
	// so the page can say "through Software Updater" rather than offer a
	// button that would fight the package manager; the release page; and a
	// key for the page's own sentence in either language.
	Channel string `json:"channel,omitempty"`
	Page    string `json:"page,omitempty"`
	WhyKey  string `json:"why_key,omitempty"`
}

/*
 * newestCache is the last answer about the newest release.
 *
 * Kept a day, because the page asks every time it opens and once an hour, and
 * the promise is one request a day. "Check again" asks at once.
 */
type newestCache struct {
	mu  sync.Mutex
	at  time.Time
	rel latest.Release
	err error
}

const newestFor = 24 * time.Hour

func (c *newestCache) get(ctx context.Context, fresh bool) (latest.Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !fresh && !c.at.IsZero() && time.Since(c.at) < newestFor {
		return c.rel, c.err
	}

	rel, err := latest.Newest(ctx, nil)

	// An offline answer is not kept for a day: the network coming back
	// should be noticed the next time the page asks.
	if err == nil || !errors.Is(err, latest.ErrOffline) {
		c.at, c.rel, c.err = time.Now(), rel, err
	}

	return rel, err
}

/*
 * programUpkeep is the program's own row.
 *
 * A released copy is compared in releases, against the newest one published;
 * a copy built by hand keeps the commit comparison selfupdate does.
 */
func (s *Server) programUpkeep(ctx context.Context, fresh bool) Upkeep {
	if release.Version == "" {
		self := selfupdate.Check(ctx, nil)

		program := Upkeep{
			Name:    config.Product,
			Have:    self.Running,
			Latest:  self.Latest,
			Newer:   self.Newer,
			How:     "self",
			Why:     self.Why,
			Channel: string(latest.Source),
		}

		if self.Dirty {
			program.Have += " + your own changes"
		}

		if self.Behind > 0 {
			program.Latest = fmt.Sprintf("%s (%d commits ahead)", self.Latest, self.Behind)
		}

		return program
	}

	channel := latest.How(release.Version)

	program := Upkeep{
		Name:    config.Product,
		Have:    release.Version,
		How:     "self",
		Channel: string(channel),
		Page:    latest.Page,
	}

	// Asked for nothing by itself when its owner said so; the button still
	// asks at once.
	if !s.brain.Cfg.UpdateNotice && !fresh {
		program.Why = "Checking for new versions by itself is off. Check again asks now."
		program.WhyKey = "upd.noticeOff"

		return program
	}

	rel, err := s.newest.get(ctx, fresh)
	if err != nil {
		program.Why = err.Error()
		program.WhyKey = "upd.cannotTell"

		if errors.Is(err, latest.ErrOffline) {
			program.Why = "Could not reach GitHub to see whether a new version is out. It asks again later."
			program.WhyKey = "upd.offline"
		}

		return program
	}

	program.Latest = rel.Version
	program.Page = rel.Page
	program.Newer = latest.Newer(release.Version, rel.Version)

	return program
}

// handleUpkeep checks everything that can be out of date.
func (s *Server) handleUpkeep(w http.ResponseWriter, r *http.Request) {
	/*
	 * Whether it may look things up — which is not the privacy setting.
	 *
	 * Privacy is about where your words go: whether a conversation reaches
	 * somebody else's model. This asks a public server whether a newer version
	 * exists, and carries nothing of anybody's. Gating it on privacy meant
	 * that keeping your conversation on this machine also meant never being
	 * told an update existed, and nobody makes that second decision on
	 * purpose.
	 */
	if !s.brain.Cfg.LookOnline {
		ok(w, map[string]any{
			"items":   []Upkeep{},
			"why_key": "upd.lookOff",
			"why": "Looking things up online is switched off, so I cannot tell " +
				"you whether anything is out of date. It is on the Permissions " +
				"page — and it is a separate switch from privacy, because " +
				"asking whether a newer version exists says nothing about you.",
		})

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	items := []Upkeep{}

	// The program itself.
	items = append(items, s.programUpkeep(ctx, r.URL.Query().Get("fresh") == "1"))

	// The game engine, which publishes releases and says so plainly.
	if engine, installed := godot.Find(); installed {
		item := Upkeep{Name: "Godot", Have: engine.Version, How: "part:Godot (making games)"}

		if latest, err := godot.Latest(ctx, nil); err == nil {
			item.Latest = latest.Version
			item.Newer = godot.Newer(engine.Version, latest.Version)
		} else {
			item.Why = err.Error()
		}

		items = append(items, item)
	}

	/*
	 * And everything the requirements already know about.
	 *
	 * They carry their own version check — the one that put "0.33.3 available"
	 * beside Ollama — so this reads what is already there rather than asking
	 * again in a second way that could disagree with the first.
	 */
	for _, req := range preflight.Requirements() {
		state, detail := req.Check()

		if state != preflight.Outdated {
			continue
		}

		items = append(items, Upkeep{
			Name:   req.Name,
			Have:   detail,
			Newer:  true,
			How:    "part:" + req.Name,
			Latest: "",
		})
	}

	var waiting int

	for _, i := range items {
		if i.Newer {
			waiting++
		}
	}

	ok(w, map[string]any{"items": items, "waiting": waiting, "notice": s.brain.Cfg.UpdateNotice})
}

/*
 * handleSelfUpdate pulls and rebuilds, behind the conversation.
 *
 * A background job because it is a network fetch and a compile, and because
 * the thing being replaced is the program serving this request — a handler
 * that waited for its own replacement would be holding a connection open
 * across the moment its binary changed.
 */
func (s *Server) handleSelfUpdate(w http.ResponseWriter, r *http.Request) {
	if release.Version != "" {
		s.updateReleased(w, r)

		return
	}

	if s.brain.Jobs == nil {
		fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

		return
	}

	_, err := s.brain.Jobs.Start("Updating the assistant", func(ctx context.Context) (string, error) {
		return selfupdate.Update(ctx, func(note string) {
			s.brain.Log.Info("updating the assistant", "step", note)
		})
	})
	if err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	ok(w, map[string]any{"started": "Updating in the background. " +
		"It appears under Activity, and you will need to reopen the program at the end."})
}

/*
 * updateReleased updates a released copy the way it was installed.
 *
 * The Windows installer is the one case that downloads: in the background,
 * because it is a download, and at the end the program closes itself so the
 * installer can replace it — the installer opens the new one again. Every
 * other channel answers at once with what was opened or what to do.
 */
func (s *Server) updateReleased(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	if !s.brain.Cfg.LookOnline {
		fail(w, http.StatusConflict, "Looking things up online is switched off, so I cannot fetch a new version.")

		return
	}

	rel, err := s.newest.get(ctx, false)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())

		return
	}

	if !latest.Newer(release.Version, rel.Version) {
		ok(w, latest.Outcome{Key: "upd.current", Say: "This is already the newest version."})

		return
	}

	channel := latest.How(release.Version)

	if channel != latest.Installer {
		out, err := latest.Apply(ctx, channel, rel, "")
		if err != nil {
			fail(w, http.StatusConflict, err.Error())

			return
		}

		ok(w, out)

		return
	}

	if s.brain.Jobs == nil {
		fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

		return
	}

	dir := filepath.Join(os.TempDir(), "pn-scripts-assistant-update", rel.Version)

	_, err = s.brain.Jobs.Start("Updating the assistant to "+rel.Version, func(ctx context.Context) (string, error) {
		out, err := latest.Apply(ctx, channel, rel, dir)
		if err != nil {
			return "", err
		}

		s.brain.Log.Info("the installer of the new version is running; closing", "version", rel.Version)

		// A moment for the page to hear it, then out of the installer's way.
		go func() {
			time.Sleep(2 * time.Second)

			if s.quit != nil {
				s.quit()
			}
		}()

		return out.Say, nil
	})
	if err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	ok(w, latest.Outcome{
		Started: true,
		Key:     "upd.downloading",
		Say:     fmt.Sprintf("Downloading %s and checking it. The assistant closes when the installer starts and opens again when it is done.", rel.Version),
	})
}

// handleUpdateNotice switches the daily look for a new version on or off.
func (s *Server) handleUpdateNotice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	cfg := s.brain.Cfg
	cfg.UpdateNotice = body.On

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "I could not write it down: "+err.Error())

		return
	}

	s.brain.Cfg = cfg

	s.brain.Log.Info("the daily look for a new version changed", "on", body.On)

	ok(w, map[string]any{"notice": body.On})
}

// so encoding/json stays used if the shapes above change.
var _ = json.Marshal
