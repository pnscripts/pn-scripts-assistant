package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/brain/storage"
	"sort"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/desktop"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/preflight"
	"pn-scripts-assistant/internal/stage"
	"pn-scripts-assistant/internal/starter"
)

// setupServer is a small HTTP server the desktop app runs itself, so first-run
// setup happens inside the application window rather than in a terminal.
//
// It has to exist separately from the brain because of an ordering problem: the
// brain cannot serve its own setup page on a machine that is missing the things
// the brain needs to start. So the app carries just enough of a web server to
// explain the situation and fix it, then hands the window over to the brain.
type Server struct {
	mu       sync.Mutex
	log      bytes.Buffer
	busy     bool
	envPath  string
	listener net.Listener

	// root is where the brain will be kept, once somebody has chosen. Empty
	// until then, which means wherever it would have gone anyway.
	root string

	// finished records that somebody pressed Continue, as opposed to closing
	// the window. See Finished.
	finished bool

	/*
	 * welcome is the opening written for this machine, and writingWelcome
	 * that somebody is writing it. See welcome.go.
	 */
	welcome        string
	writingWelcome bool

	// voice is which of robot/man/woman was picked, so the page can show
	// which one is chosen after a reload. The setting itself is written to
	// the settings file the moment it is chosen; this is only the mark.
	voice string

	/*
	 * How the last apply ended, because the requirement counts cannot say.
	 *
	 * "Everything is ready" is computed from how many blocking requirements
	 * remain, and an apply that dies on an optional one leaves that count at
	 * zero — so the page announced everything was ready directly underneath
	 * the words "Failed: exit status 1". Being technically about a different
	 * question is no defence when the two sit one above the other.
	 */
	applyFailed bool

	/*
	 * Where the install has got to, said as a person would say it.
	 *
	 * The log was the only answer to "how much longer": several thousand lines
	 * of cmake output, in which the honest answer is present and unreadable.
	 * These three are what somebody actually wants — which piece, how many
	 * pieces, and how far into this one — and they are kept here rather than
	 * parsed out of the log by the page, because the page would then be
	 * guessing at the meaning of somebody else's build output.
	 */
	stepNow   int
	stepTotal int
	stepName  string

	/*
	 * The raw name of what is installing, as the page spells it.
	 *
	 * stepName beside it is the readable version — "Downloading qwen2.5-coder:7b"
	 * — which is what somebody reads and useless for matching. The page needs
	 * to put the progress inside the card of the thing being installed, and
	 * for that it has to be able to say which card that is.
	 */
	stepTarget string

	// What this run put on the machine, so closing setup half way can take it
	// back off. See rollback.go.
	added []added

	/*
	 * The readiness ticks, cached because two of them are slow.
	 *
	 * Its own lock rather than the one above: the slow probes take minutes on
	 * a processor, and holding the server's lock for that would stall every
	 * two-second poll behind them. It deliberately does not set s.busy either
	 * — RollBack waits on that before removing anything, so a probe running
	 * when somebody closes the window would make the close hang.
	 */
	ready readiness
}

func New(envPath string) (*Server, error) {
	// Port 0: the OS picks a free one. Hardcoding a port would collide with
	// whatever else the user happens to be running.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	return &Server{listener: l, envPath: envPath}, nil
}

func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

func (s *Server) Serve(onReady func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, setupPage)
	})

	// The world the steps stand in, shared with the program's own page.
	mux.Handle(stage.Prefix, stage.Handler())

	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, s.state())
	})

	mux.HandleFunc("/install", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		go s.install(name)
		s.writeJSON(w, map[string]any{"started": true})
	})

	// Which of the installed models should answer.
	mux.HandleFunc("/chosen-model", s.handleChosenModel)

	// Whether it actually works, rather than whether it is installed.
	mux.HandleFunc("/ready", s.handleReady)

	// The opening, written for this machine when there is a model to write
	// it. See welcome.go.
	mux.HandleFunc("/welcome", s.handleWelcome)

	// Who it is: a name to call it, and the language it should expect to hear.
	mux.HandleFunc("/identity", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.saveIdentity(w, r)

			return
		}

		s.writeJSON(w, s.identity())
	})

	// Every model there is, for somebody who wants more than the eight.
	mux.HandleFunc("/library", s.handleLibrary)

	mux.HandleFunc("/choose-model", func(w http.ResponseWriter, r *http.Request) {
		/*
		 * Which model, when one is named.
		 *
		 * An empty body still means the recommended one, so the page works
		 * unchanged and an older client is not broken by this.
		 */
		var body struct {
			Model string `json:"model"`
		}

		json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)

		go s.install(modelRequest(body.Model))
		s.writeJSON(w, map[string]any{"started": true})
	})

	/*
	 * Where the brain should be kept.
	 *
	 * Recorded rather than acted on: nothing is created until setup finishes,
	 * so choosing a drive and then changing your mind leaves nothing behind
	 * on the first one.
	 */
	/*
	 * Do everything that was chosen, in one go.
	 *
	 * Setup collects decisions and this carries them out, rather than each
	 * button acting the moment it is pressed. Somebody picking a drive, a
	 * model and two optional pieces should be able to change their mind about
	 * any of them without having already downloaded five gigabytes of the
	 * first answer.
	 *
	 * In the order given, and it stops at the first failure: installing a
	 * model before Ollama exists produces a confusing error about a command
	 * not being found, where stopping produces the real one.
	 */
	mux.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Steps []string `json:"steps"`
		}

		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		go s.applyAll(body.Steps)
		s.writeJSON(w, map[string]any{"started": true})
	})

	/*
	 * A folder, not only a drive.
	 *
	 * The drive buttons offer one folder per drive and nothing else, which is
	 * the right default and the wrong only option — somebody who keeps their
	 * work under a particular directory has a place they want this to go, and
	 * "pick one of two" does not let them say so.
	 *
	 * Checked before it is accepted, because the failure otherwise arrives
	 * much later: setup finishes, applies, and the first write fails on a path
	 * nobody could create.
	 */
	mux.HandleFunc("/choose-folder", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}

		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		path := strings.TrimSpace(body.Path)

		if path == "" {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Give a folder."})

			return
		}

		// A relative path here would be resolved against wherever the program
		// happens to be running from, which is not a place anybody meant.
		if !filepath.IsAbs(path) {
			s.writeJSON(w, map[string]any{
				"ok":    false,
				"error": "Give the full path, starting with /.",
			})

			return
		}

		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}

		path = filepath.Clean(path)

		if !canCreate(path) {
			s.writeJSON(w, map[string]any{
				"ok": false,
				"error": "Cannot write to " + path +
					". Pick somewhere inside your own folders, or choose a drive above.",
			})

			return
		}

		s.mu.Lock()
		s.root = path
		s.mu.Unlock()

		s.writeJSON(w, map[string]any{"ok": true, "path": path})
	})

	mux.HandleFunc("/choose-drive", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}

		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		if err := s.chooseRoot(body.Path); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true, "path": s.chosenRoot()})
	})

	mux.HandleFunc("/api-key", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`

			// Which service the key belongs to. Empty means Anthropic, which
			// is what the page sent before there was anywhere else to send.
			Provider string `json:"provider"`

			// Where it lives, for a server this program cannot know the
			// address of — LM Studio, vLLM, another machine in the house.
			BaseURL string `json:"base_url"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		if err := s.saveProvider(body.Provider, strings.TrimSpace(body.Key),
			strings.TrimSpace(body.BaseURL)); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	// The starter answers questions while the real brain is still downloading.
	// That gap is exactly when someone has the most questions about what is
	// being installed on their machine, and the least reason to trust it.
	mux.HandleFunc("/ask", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Question string `json:"question"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		reply := starter.Ask(body.Question)

		s.writeJSON(w, map[string]any{
			"answer":   reply.Answer,
			"matched":  reply.Matched,
			"followup": reply.Followup,
		})
	})

	mux.HandleFunc("/suggestions", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]any{"suggestions": starter.Suggestions()})
	})

	/*
	 * The choices about what it asks before reading.
	 *
	 * Written into the brain's own folder, so they are already in force the
	 * first time it opens rather than being a thing to go and set afterwards.
	 */
	mux.HandleFunc("/protection", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Off   *[]string `json:"off"`
			Yours *[]string `json:"yours"`
		}

		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

		root := s.chosenRoot()
		chosen := protect.Load(root)

		if body.Off != nil {
			chosen.Off = *body.Off
		}

		if body.Yours != nil {
			chosen.Extra = *body.Yours
		}

		if err := protect.Save(root, chosen); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	/*
	 * The one switch: how much it asks, and how much leaves.
	 *
	 * The same three answers the program offers under Permissions, asked
	 * here because it is the question every later approval prompt is the
	 * consequence of. Somebody who wanted it never to stop should not first
	 * have to meet a stream of questions to discover that it can.
	 */
	mux.HandleFunc("/freedom", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Level string `json:"level"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		level, err := s.saveFreedom(body.Level)
		if err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true, "freedom": string(level)})
	})

	/*
	 * How it will sound, before setup closes.
	 *
	 * On the last step rather than beside the other choices, because the
	 * voices do not exist until the install has run — a voice picker offered
	 * three buttons of which two could not play would be asking somebody to
	 * choose between things they cannot hear.
	 */
	mux.HandleFunc("/voices", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		chosen := s.voice
		s.mu.Unlock()

		s.writeJSON(w, map[string]any{"voices": voiceChoices(), "chosen": chosen})
	})

	mux.HandleFunc("/hear", func(w http.ResponseWriter, r *http.Request) {
		if err := s.hear(r.URL.Query().Get("kind")); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/voice", func(w http.ResponseWriter, r *http.Request) {
		if err := s.chooseVoice(r.URL.Query().Get("kind")); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.finished = true
		s.mu.Unlock()

		s.writeJSON(w, map[string]any{"ok": true})
		go onReady()
	})

	_ = http.Serve(s.listener, mux)
}

/*
 * Finished reports whether setup was seen through, rather than merely closed.
 *
 * Closing the window and pressing Continue both end the setup server, and for
 * a long time they were the same thing to everything downstream: the brain
 * started either way. Which meant shutting the window on a machine with no
 * Ollama started a brain that could not answer, with no clue why — the one
 * failure that looks exactly like the program being broken.
 */
func (s *Server) Finished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.finished
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type requirementView struct {
	Name        string `json:"name"`
	Why         string `json:"why"`
	Consequence string `json:"consequence"`
	State       string `json:"state"`
	Detail      string `json:"detail"`
	Optional    bool   `json:"optional"`
	Installable bool   `json:"installable"`
	ManualHint  string `json:"manual_hint"`

	// Where it is now, or where it will go. See Requirement.Where.
	Where string `json:"where"`

	/*
	 * What it costs and whether there is room for it.
	 *
	 * Both were known here and said nowhere: the page offered to download nine
	 * gigabytes onto a disk whose free space it had already measured. Free is
	 * for the filesystem this particular piece lands on, which is not one
	 * number for the page — the models go to ollama's own directory whatever
	 * drive the brain was put on.
	 */
	Size   string `json:"size,omitempty"`
	FreeGB int    `json:"free_gb,omitempty"`

	/*
	 * NeedsRoot marks an install that will raise a password prompt.
	 *
	 * Worth saying on the card, and worth knowing when several are installed
	 * in one press: a run stops at the first failure, and a dismissed password
	 * box is a failure. Those go last, so cancelling one does not abandon
	 * everything queued behind it.
	 */
	NeedsRoot bool `json:"needs_password,omitempty"`
}

func (s *Server) state() map[string]any {
	results := preflight.Check()
	views := make([]requirementView, 0, len(results))

	for _, r := range results {
		free := 0

		if where := r.Requirement.Location(); where != "" {
			if _, usable, err := storage.SpaceOn(where); err == nil {
				free = int(usable / (1 << 30))
			}
		}

		views = append(views, requirementView{
			NeedsRoot:   r.Requirement.NeedsRoot,
			Size:        r.Requirement.Size,
			FreeGB:      free,
			Name:        r.Requirement.Name,
			Why:         r.Requirement.Why,
			Consequence: r.Requirement.Consequence,
			State:       r.State.Label(),
			Detail:      r.Detail,
			Optional:    r.Requirement.Optional,
			Installable: r.Requirement.Installable(),
			ManualHint:  r.Requirement.ManualHint,
			Where:       r.Requirement.Location(),
		})
	}

	// Whether anything is configured that can answer without a local model.
	// Loaded rather than inferred from s.keysHeld so a custom server with no
	// key counts too.
	paidBrain := false
	chosenModel := ""
	freedom := string(permits.AskEveryTime)

	if cfg, err := config.LoadFrom(s.envPath); err == nil {
		paidBrain = cfg.HasPaidProvider()
		chosenModel = cfg.OllamaModel

		if asking := permits.Freedom(cfg.Asking()); permits.Known(asking) {
			freedom = string(asking)
		}
	}

	hw := preflight.DetectHardware()
	model := preflight.RecommendModel(hw)

	s.mu.Lock()
	logText, busy, failed := s.log.String(), s.busy, s.applyFailed
	stepNow, stepTotal, stepName := s.stepNow, s.stepTotal, s.stepName
	stepTarget := s.stepTarget
	s.mu.Unlock()

	/*
	 * What the brain will stop and ask about before reading.
	 *
	 * Offered here rather than announced later, because it is a decision about
	 * somebody's own files and the moment to make it is while they are setting
	 * the thing up. Every rule starts on, so skipping the step gives the
	 * protective answer.
	 */
	chosen := protect.Load(s.chosenRoot())
	rules := make([]map[string]any, 0, len(protect.BuiltIn))

	for _, rule := range protect.BuiltIn {
		rules = append(rules, map[string]any{
			"id": rule.ID, "what": rule.What, "why": rule.Why, "fixed": rule.Fixed,
		})
	}

	return map[string]any{
		"requirements": views,

		// The one switch, as the file has it. See /freedom.
		"freedom": freedom,

		"protection": map[string]any{
			"rules": rules,
			"off":   chosen.Off,
			"yours": chosen.Extra,
		},
		// Counted against the chosen path: the pieces that only run a local
		// model are not missing from somebody using a paid service, they are
		// irrelevant to them. See preflight.BlockingFor.
		"blocking": preflight.BlockingCountFor(results, paidBrain),
		"busy":     busy,

		// Which piece, of how many, and how far into it. See the fields.
		"step_now":   stepNow,
		"step_total": stepTotal,
		"step_name":  stepName,

		// Which card the progress belongs in. See stepTarget.
		"step_target":  stepTarget,
		"percent":      percentOf(logText),
		"apply_failed": failed,
		"log":          logText,
		"hardware": map[string]any{
			"cores":     hw.CPUCores,
			"ram_gb":    hw.RAMGB,
			"gpu":       hw.GPUName,
			"has_gpu":   hw.HasGPU,
			"can_local": preflight.CanRunLocalModels(hw),
		},
		"recommended_model": map[string]any{
			"model": model.Model,
			"size":  model.SizeNote,
			"speed": model.SpeedNote,
		},

		/*
		 * And the alternatives, because a single suggestion makes a trade on
		 * somebody's behalf that they may not want.
		 *
		 * The recommendation is still marked. What changes is that a person
		 * who would rather wait two seconds than get the better answer can now
		 * see that the option exists, which they could not before.
		 */
		"model_options": modelOptions(hw),
		"has_api_key":   s.hasAPIKey(),

		// Which companies already have one, so the page can show them and
		// offer the rest. Names only — never the keys themselves.
		"api_keys": s.keysHeld(),

		// And every company there is, from the one list. The picker used to
		// carry its own copy of three of them.
		"services": serviceViews(),

		/*
		 * And where the brain should live.
		 *
		 * Asked here because this is the moment it is cheap. Moving it later
		 * copies everything it has learned and deletes the original, which is
		 * a long operation on a drive that could be unplugged half way
		 * through — and somebody who keeps their work on a second disk would
		 * have wanted it there from the start rather than after.
		 */
		"drives":       s.drives(),
		"chosen_model": chosenModel,
		"chosen_drive": s.chosenRoot(),

		// Where the models land, which is not the folder chosen above — see
		// the overview on the last step.
		"model_dir": preflight.ModelDir(),

		/*
		 * And whether there is any way to start this again afterwards.
		 *
		 * Setup left the machine able to run PN Scripts Assistant and gave nobody a way to
		 * do it: no menu entry, so the only route back was the file it was
		 * launched from, or a terminal. Reported here so the last step can
		 * offer it like everything else rather than doing it silently.
		 */
		"menu_installed": desktop.Installed(),
		"menu_path":      menuPath(),
	}
}

func (s *Server) install(name string) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()

		return
	}

	s.busy = true
	s.log.Reset()

	/*
	 * Which piece, so the page can put the progress inside its card.
	 *
	 * Set here as well as in applyAll, and it matters more here: every install
	 * goes through this path now that each step installs its own things, so
	 * without it the bar had no idea what it belonged to and fell back to the
	 * foot of the page for everything.
	 */
	s.stepName, s.stepTarget = readableStep(name), name
	s.stepNow, s.stepTotal = 1, 1
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.busy = false
		s.stepNow, s.stepTotal, s.stepName = 0, 0, ""
		s.stepTarget = ""
		s.mu.Unlock()
	}()

	/*
	 * The same installer applyAll uses, not a second one beside it.
	 *
	 * This used to have its own copy — find the requirement, install it,
	 * report a failure — which was harmless while it was the minor path and
	 * became a real hole when each step started installing its own things and
	 * this became the only path. runOne records what it installs so that
	 * closing setup half way can take it off again; this did not, so the
	 * rollback quietly had nothing to undo.
	 */
	w := &syncWriter{s: s}

	/*
	 * A model needs somewhere to run, and that is not somebody's problem.
	 *
	 * Choosing how it thinks now comes before the list of pieces, which is the
	 * right order for the decision and the wrong one for the dependency: the
	 * model cannot be pulled until Ollama exists. The alternative was to put
	 * the demand back — "install Ollama before you may choose a model" — which
	 * is the ordering this whole change exists to remove.
	 *
	 * So it is fetched first, silently in the sense that nobody had to ask for
	 * it, and loudly in the sense that the progress says what it is doing.
	 * Ollama still appears on the pieces step, by then already ticked, because
	 * "what is on my machine" deserves a complete answer even when the person
	 * never had to think about one of the entries.
	 */
	_, isModel := strings.CutPrefix(name, modelPrefix)
	_, haveOllama := exec.LookPath("ollama")

	if isModel && haveOllama != nil {
		fmt.Fprintln(w, "This needs Ollama to run models. Fetching that first.")

		s.mu.Lock()
		s.stepName, s.stepTarget = "Installing Ollama", "Ollama"
		s.stepNow, s.stepTotal = 1, 2
		s.mu.Unlock()

		// Through runOne like everything else, so closing setup half way can
		// still take it back off again.
		if !s.runOne("Ollama", w) {
			fmt.Fprintln(w, "\nOllama could not be installed, so the model was not fetched.")

			return
		}

		s.mu.Lock()
		s.stepName, s.stepTarget = readableStep(name), name
		s.stepNow, s.stepTotal = 2, 2
		s.mu.Unlock()
	}

	s.runOne(name, w)
}

// saveAPIKey writes the key into the brain's settings file. It is never logged or echoed back: the
// setup log is displayed in the window, and a key that appears there would be
// a key shown to anyone looking over the user's shoulder.
/*
 * saveProvider records what is needed to reach one company.
 *
 * A key for most of them, an address for the one that has none of its own, and
 * for a self-hosted server neither is optional in the same way: it needs the
 * address and usually no key at all. So "no key given" stopped being the only
 * failure worth naming.
 */
func (s *Server) saveProvider(provider, key, baseURL string) error {
	/*
	 * No company named means Anthropic.
	 *
	 * What the page sent before there was anywhere else to send, and what an
	 * older one still sends. Rejecting it broke saving a key at all from any
	 * caller that had not been updated — including this package's own tests,
	 * which is how it was caught.
	 */
	if strings.TrimSpace(provider) == "" {
		provider = "anthropic"
	}

	svc, known := llm.ServiceByID(provider)
	if !known {
		return fmt.Errorf("I do not know a service called %q", provider)
	}

	if baseURL != "" {
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return fmt.Errorf("that address needs to start with http:// or https://")
		}

		if err := s.writeSetting(llm.BaseURLSetting(svc.ID), baseURL, 0o600); err != nil {
			return err
		}
	}

	if !svc.NeedsKey && key == "" {
		if baseURL == "" {
			return fmt.Errorf("give the address of the server")
		}

		return nil
	}

	return s.saveAPIKey(provider, key)
}

func (s *Server) saveAPIKey(provider, key string) error {
	setting := llm.KeySetting(provider)

	if key == "" {
		return fmt.Errorf("no key given")
	}

	if err := llm.CheckKey(provider, key); err != nil {
		return err
	}

	// 0600: this file now holds a credential.
	return s.writeSetting(setting, key, 0o600)
}

/*
 * keysHeld lists the companies a key has been saved for.
 *
 * Names only, never the keys: this goes to the page, and a key that reaches an
 * interface is a key in a place it was not put. Setup can hold one for each
 * company at once — they are separate settings and always have been — and this
 * is what lets the page say which are done and offer the rest.
 */
func (s *Server) keysHeld() []string {
	held := []string{}

	data, err := os.ReadFile(s.envPath)
	if err != nil {
		return held
	}

	lines := strings.Split(string(data), "\n")

	for _, svc := range llm.Services() {
		setting := llm.KeySetting(svc.ID) + "="

		for _, line := range lines {
			if !strings.HasPrefix(line, setting) {
				continue
			}

			if strings.TrimSpace(strings.TrimPrefix(line, setting)) != "" {
				held = append(held, svc.ID)
			}

			break
		}
	}

	return held
}

/*
 * serviceViews is the company list as the page needs it.
 *
 * Base URLs and default models are left out: neither is a decision anybody
 * makes on this screen, and the address of a company's API is not information,
 * it is trivia. The custom entry is the exception and says so in its note.
 */
func serviceViews() []map[string]any {
	out := []map[string]any{}

	for _, svc := range llm.Services() {
		out = append(out, map[string]any{
			"id":        svc.ID,
			"name":      svc.Name,
			"where":     svc.Where,
			"note":      svc.Note,
			"prefix":    svc.Prefix,
			"needs_key": svc.NeedsKey,
			"custom":    svc.ID == "custom",
		})
	}

	return out
}

/*
 * writeSetting puts one name=value into the settings file.
 *
 * Line by line rather than by rewriting the file from a parsed structure: the
 * file is full of comments explaining what each setting does, and a rewrite
 * would throw away the explanation along with the formatting. Somebody who
 * opens it after setup should find the same document they would have found
 * before, with one line changed.
 *
 * On a first run the file does not exist yet, which is exactly when somebody
 * is most likely to be choosing something here. A missing file means "no
 * settings", not a failure.
 */
func (s *Server) writeSetting(name, value string, mode os.FileMode) error {
	return s.writeSettings(mode, [2]string{name, value})
}

// writeSettings is writeSetting for settings that must change together, in
// one write, so the file is never read with one of them changed and not the
// other.
func (s *Server) writeSettings(mode os.FileMode, settings ...[2]string) error {
	data, err := os.ReadFile(s.envPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read %s: %w", s.envPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(s.envPath), 0o755); err != nil {
		return fmt.Errorf("could not create the settings folder: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	for _, setting := range settings {
		name := setting[0]

		/*
		 * One line, whatever was typed.
		 *
		 * This file is name=value a line at a time, and every caller so far
		 * has passed a key, a URL or a voice id — none of which can contain a
		 * newline. A name can: it is the first free-text value a person gets
		 * to type, and "Bob\nANTHROPIC_API_KEY=..." would write a second
		 * setting nobody asked for. Flattened here rather than at the caller,
		 * so the next free-text setting is safe without anybody remembering.
		 */
		value := strings.NewReplacer("\r", " ", "\n", " ").Replace(setting[1])

		replaced := false

		for i, line := range lines {
			if strings.HasPrefix(line, name+"=") {
				lines[i] = name + "=" + value
				replaced = true

				break
			}
		}

		if !replaced {
			lines = append(lines, name+"="+value)
		}
	}

	return os.WriteFile(s.envPath, []byte(strings.Join(lines, "\n")), mode)
}

/*
 * hasAPIKey reports whether anything is configured that can answer.
 *
 * One rule, in config, asked from here. It used to name three companies while
 * the picker offered thirteen — so a Groq key saved perfectly well, the card
 * said "key saved", and this returned false. Survivable while it shut one
 * step; not survivable once the wizard asks how it should think as its first
 * question, because the gate on that step reads this.
 *
 * Then it briefly counted a bare base URL, which is worse in the other
 * direction: ANTHROPIC_BASE_URL is an ordinary thing to have exported, and a
 * machine with nothing installed reported a brain it did not have. Both
 * mistakes are avoided by asking config, which is where the question belongs
 * and where the running program asks it too.
 */
func (s *Server) hasAPIKey() bool {
	cfg, err := config.LoadFrom(s.envPath)
	if err != nil {
		return len(s.keysHeld()) > 0
	}

	return cfg.HasPaidProvider()
}

// syncWriter funnels install output into the buffer the window polls.
type syncWriter struct {
	s *Server
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()

	return w.s.log.Write(p)
}

// modelOptions renders the models worth offering this machine.
func modelOptions(hw preflight.Hardware) []map[string]any {
	options := preflight.ModelOptions(hw)
	here := modelsHere()

	out := make([]map[string]any, 0, len(options))

	for _, o := range options {
		out = append(out, map[string]any{
			"model":       o.Model,
			"size":        o.SizeNote,
			"speed":       o.SpeedNote,
			"label":       o.Label,
			"recommended": o.Recommended,
			"fits":        o.Fits,
			"needs_gb":    o.NeedsGB,

			// So the row can say "installed" instead of offering to fetch
			// something that is already here.
			"installed": here[o.Model] || here[o.Model+":latest"],
		})
	}

	return out
}

/*
 * modelPrefix marks an install request as being for a model rather than for
 * one of the machine's requirements.
 *
 * A prefix rather than the old "__model__" sentinel, because the name of the
 * model now travels with it and a sentinel has nowhere to put one.
 */
const modelPrefix = "model:"

func modelRequest(model string) string { return modelPrefix + strings.TrimSpace(model) }

// DataFolder is the directory the brain keeps itself in on a chosen drive.
//
// Named rather than assembled at each call site, because paths.Find looks for
// exactly this name when working out where an existing brain lives — and the
// two spellings drifting apart would mean a brain that is created in one place
// and looked for in another.
// The name lives in paths, beside the code that looks for it and beside the
// old name it still recognises, so a rename cannot leave a brain created in
// one place and looked for in another.
const DataFolder = paths.DataFolder

/*
 * drives lists where the brain could be kept.
 *
 * Only mounted filesystems with room to grow, which is what storage.Drives
 * already decides: a drive that is plugged in but not mounted cannot be
 * written to, and a 512MB boot partition would accept the first write and fail
 * on the hundredth.
 */
func (s *Server) drives() []map[string]any {
	/*
	 * Asked without telling it what is chosen, so the order never moves.
	 *
	 * storage.Drives sorts the current drive first, which is right inside the
	 * running program — it is showing where the brain is. Here it is offering
	 * a choice, and a list that re-sorts when you pick from it makes the
	 * buttons appear to swap places under the cursor. Nothing had moved except
	 * which one was current.
	 */
	found, err := storage.Drives("")
	if err != nil {
		return nil
	}

	/*
	 * The system disk first, then the rest by room.
	 *
	 * A fixed order, and one that puts the answer most people want at the top
	 * — the drive that is always there — while still showing that a bigger one
	 * exists underneath it.
	 */
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].MountPoint == "/") != (found[j].MountPoint == "/") {
			return found[i].MountPoint == "/"
		}

		return found[i].FreeBytes > found[j].FreeBytes
	})

	out := make([]map[string]any, 0, len(found))

	for _, d := range found {
		where := brainFolderOn(d.MountPoint)

		/*
		 * Judged on the folder the brain would use, not on the mount point.
		 *
		 * The root filesystem is not writable by an ordinary user, so testing
		 * the mount point dropped "this computer" from the list entirely and
		 * left a single option — which is not a choice. The brain does not
		 * live at /; it lives in a home directory on that filesystem, and that
		 * is writable.
		 */
		if !canCreate(where) {
			continue
		}

		out = append(out, map[string]any{
			"path":       where,
			"mount":      d.MountPoint,
			"free_gb":    d.FreeBytes / (1 << 30),
			"total_gb":   d.TotalBytes / (1 << 30),
			"removable":  d.Removable,
			"filesystem": d.Filesystem,
		})
	}

	return out
}

/*
 * brainFolderOn is where the brain would keep itself on a given filesystem.
 *
 * Inside the home directory when that is on this filesystem, because that is
 * the one place an ordinary user can always write and because a folder at the
 * top of the system disk is not somewhere anybody expects their things. On any
 * other drive it is a named folder at the top, which is what somebody plugging
 * that drive into another machine would go looking for.
 */
func brainFolderOn(mount string) string {
	home, err := os.UserHomeDir()
	if err == nil && onSameFilesystem(home, mount) {
		return paths.HomeRootHere()
	}

	return filepath.Join(mount, DataFolder)
}

// onSameFilesystem reports whether a path sits under a mount point.
//
// By path rather than by device id, because the question here is which of the
// offered mounts a directory belongs to, and the longest matching prefix is
// exactly that.
func onSameFilesystem(path, mount string) bool {
	if mount == "/" {
		return !strings.HasPrefix(path, "/media/") &&
			!strings.HasPrefix(path, "/mnt/") &&
			!strings.HasPrefix(path, "/run/media/")
	}

	return strings.HasPrefix(path, strings.TrimSuffix(mount, "/")+"/")
}

/*
 * canCreate reports whether a folder could be made and written at this path.
 *
 * Tried rather than inspected. A drive can be mounted, report every permission
 * correctly, and still refuse: a read-only mount, a full disk, a filesystem
 * that will not take the ownership this needs. Anything left behind is removed,
 * including a directory that was created only to find out.
 */
func canCreate(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return canWriteInto(path)
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return false
	}

	ok := canWriteInto(path)

	// Only the folder just created, and only if it is still empty.
	os.Remove(path)

	return ok
}

func canWriteInto(dir string) bool {
	probe := filepath.Join(dir, ".pn-scripts-assistant-write-test")

	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return false
	}

	os.Remove(probe)

	return true
}

// chosenRoot is where the brain will live, as things stand.
func (s *Server) chosenRoot() string {
	s.mu.Lock()
	chosen := s.root
	s.mu.Unlock()

	if chosen != "" {
		return chosen
	}

	if r, err := paths.Find(); err == nil {
		return r.Path
	}

	return paths.HomeRootHere()
}

/*
 * chooseRoot records where the brain is to be kept, having checked it can be.
 *
 * Checked by writing rather than by inspecting permissions: a drive can be
 * mounted, look writable in every property the system reports, and still
 * refuse — a read-only mount, a full disk, a filesystem that does not allow
 * the ownership this needs. Finding that out now costs a file; finding it out
 * after setup costs somebody their first impression of the program.
 */
func (s *Server) chooseRoot(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("no folder was given")
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", path, err)
	}

	probe := filepath.Join(path, ".pn-scripts-assistant-write-test")

	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("%s cannot be written to: %w", path, err)
	}

	os.Remove(probe)

	s.mu.Lock()
	s.root = path
	s.mu.Unlock()

	return nil
}

// ChosenRoot is where setup was told to keep the brain, or empty for wherever
// it would have gone. Read once setup finishes, so the brain starts in the
// place somebody actually picked.
func (s *Server) ChosenRoot() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.root
}

/*
 * applyAll carries out everything setup was asked for, in order.
 *
 * One goroutine rather than several, because these compete for the same
 * network and the same disk: two model downloads at once on a domestic
 * connection finish later than the same two in sequence, and the log they
 * write into would interleave into something nobody could read.
 */
func (s *Server) applyAll(steps []string) {
	s.mu.Lock()

	if s.busy {
		s.mu.Unlock()

		return
	}

	s.busy = true
	s.log.Reset()
	s.applyFailed = false
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	w := &syncWriter{s: s}

	for i, name := range steps {
		s.mu.Lock()
		s.stepNow, s.stepTotal, s.stepName = i+1, len(steps), readableStep(name)
		s.stepTarget = name
		s.mu.Unlock()

		fmt.Fprintf(w, "\n[%d of %d] %s\n", i+1, len(steps), readableStep(name))

		if !s.runOne(name, w) {
			fmt.Fprintf(w, "\nStopped at step %d of %d. Nothing after this was attempted.\n",
				i+1, len(steps))

			s.mu.Lock()
			s.applyFailed = true
			s.mu.Unlock()

			return
		}
	}

	fmt.Fprint(w, "\nAll done.\n")
}

// readableStep names a step the way somebody would say it.
func readableStep(name string) string {
	// "Installing menu:" is an identifier escaping into the one place somebody
	// is watching to understand what is happening to their machine.
	if name == menuStep {
		return "Adding to the applications menu"
	}

	if model, ok := strings.CutPrefix(name, modelPrefix); ok {
		if model == "" {
			return "Downloading the language model"
		}

		return "Downloading " + model
	}

	return "Installing " + name
}

// runOne does a single step and reports whether it worked.
// menuPath is where the menu entry goes, for the overview to name.
func menuPath() string {
	if entry := desktop.SystemWide(); entry != "" {
		return entry
	}

	entry, _ := desktop.Where()

	return entry
}

// menuStep is the plan entry that puts PN Scripts Assistant in the applications menu.
const menuStep = "menu:"

func (s *Server) runOne(name string, w io.Writer) bool {
	if name == menuStep {
		entry, err := desktop.Install(config.Product)
		if err != nil {
			fmt.Fprintf(w, "\nFailed: %v\n", err)

			return false
		}

		fmt.Fprintf(w, "Added to the applications menu: %s\n", entry)
		s.note(added{kind: "menu"})

		return true
	}

	if model, ok := strings.CutPrefix(name, modelPrefix); ok {
		if model == "" {
			model = preflight.RecommendModel(preflight.DetectHardware()).Model
		}

		req := preflight.Requirement{
			Name:       "model",
			InstallCmd: func() []string { return []string{"ollama", "pull", model} },
		}

		// Asked before, because "was it here already" cannot be answered
		// afterwards — and pulling a model somebody already had must not
		// record it as this run's to delete.
		wasHere := modelPresent(model)

		if err := preflight.Install(req, w); err != nil {
			fmt.Fprintf(w, "\nFailed: %v\n", err)

			return false
		}

		if !wasHere {
			s.note(added{kind: "model", name: model})
		}

		// So a machine with one model does not leave the program guessing
		// which one to use. See noteFirstModel.
		s.noteFirstModel(model)

		return true
	}

	for _, r := range preflight.Check() {
		if r.Requirement.Name != name {
			continue
		}

		// r.State is what it was before this ran: preflight.Check was called
		// at the top of the loop. An update to something already installed
		// records nothing — undoing an update by deleting the thing would
		// leave somebody worse off than never having run setup.
		wasMissing := r.State != preflight.OK

		if err := preflight.Install(r.Requirement, w); err != nil {
			fmt.Fprintf(w, "\nFailed: %v\n", err)

			return false
		}

		if wasMissing {
			s.note(added{kind: "part", name: r.Requirement.Name})
		}

		return true
	}

	fmt.Fprintf(w, "\nNothing here is called %q.\n", name)

	return false
}

/*
 * modelsHere is what ollama already holds, as a set.
 *
 * Read once per state rather than once per row: ollama list runs a process,
 * and the page polls every two seconds.
 */
func modelsHere() map[string]bool {
	out := map[string]bool{}

	listed, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return out
	}

	for i, line := range strings.Split(string(listed), "\n") {
		// The first line is the header.
		if i == 0 {
			continue
		}

		name, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if name == "" {
			continue
		}

		out[name] = true
		out[strings.TrimSuffix(name, ":latest")] = true
	}

	return out
}
