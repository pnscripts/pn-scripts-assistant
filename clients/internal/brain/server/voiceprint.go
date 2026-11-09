package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/voiceprint"
)

/*
 * whoseVoice asks the model who was speaking, when there is a model and a
 * voice it has been taught.
 *
 * Off by default and free when off: no model installed, or no voice enrolled,
 * and this costs one map lookup and answers "cannot tell". That is the honest
 * third answer and it is deliberately not "somebody else" — a brain that
 * treats "I cannot tell" as "not you" stops listening to its owner the moment
 * a file is missing.
 */
func (s *Server) whoseVoice(samples []float64) voiceprint.Verdict {
	if len(samples) == 0 {
		return voiceprint.Verdict{Why: "nothing was recorded"}
	}

	if !voiceprint.Installed() {
		// After an upgrade the model is here and the runtime is the previous
		// release's, which is a different fix from never having fetched it.
		if voiceprint.RuntimeStale() {
			return voiceprint.Verdict{Why: "the part that tells voices apart is from an older version " +
				"and has not been updated yet"}
		}

		return voiceprint.Verdict{Why: "the voiceprint model is not installed"}
	}

	match := s.brain.Cfg.VoiceMatch

	if match <= 0 {
		match = 0.5
	}

	return voiceprint.Recognise(s.brain.Root, samples, match)
}

/*
 * onlyMeRefuses says why a turn is not acted on because of "answer only me",
 * or "" when that switch lets it through.
 *
 * Somebody else's voice is refused, and so is a voice that could not be
 * recognised at all. The second used to pass: after an upgrade the runtime of
 * the previous release counts as missing, no voice was judged, and everybody
 * was answered while the switch still showed on. A protection that turns off
 * by itself without saying so is worse than none, because it is relied on.
 */
func (s *Server) onlyMeRefuses(whose voiceprint.Verdict) string {
	if !s.brain.Cfg.OnlyMe {
		return ""
	}

	if !whose.Judged {
		return "answer only me is on, and your voice cannot be recognised now: " + whose.Why
	}

	if !whose.Owner {
		match := s.brain.Cfg.VoiceMatch
		if match <= 0 {
			match = 0.5
		}

		return fmt.Sprintf("that is not your voice (%.2f alike, %.2f needed)", whose.Alike, match)
	}

	return ""
}

// onlyMeBlocked is why "answer only me" cannot recognise anybody now, or ""
// when it can: for the Privacy page, while the switch shows on.
func (s *Server) onlyMeBlocked() string {
	if !s.brain.Cfg.OnlyMe {
		return ""
	}

	switch {
	case voiceprint.RuntimeStale():
		return "the part that tells voices apart is from an older version and has not been updated yet"
	case !voiceprint.Installed():
		return "the voiceprint model is not installed"
	case !voiceprint.Enrolled(s.brain.Root):
		return "no voice has been taught to this brain yet"
	}

	return ""
}

/*
 * handleVoiceprint says what the brain knows about its owner's voice.
 *
 * Four separate facts, because they fail separately and the fix differs: the
 * model can be missing, no voice can have been taught, the setting to act on
 * it can be off, and this machine's own music can be drowning the room. The
 * last is not about the voiceprint at all, but it arrives as the same
 * complaint — "it does not know it is me" — and it is the one that is true
 * most often.
 */
func (s *Server) handleVoiceprint(w http.ResponseWriter, r *http.Request) {
	known, _ := voiceprint.Load(s.brain.Root)

	body := map[string]any{
		"installed":   voiceprint.Installed(),
		"downloading": installing.Load() || voiceprint.Downloading(),
		"enrolled":    known != nil && len(known.Print) == voiceprint.Dimensions,
		"only_me":     s.brain.Cfg.OnlyMe,
		"cancel_room": s.brain.Cfg.CancelRoom,
		"keep_quiet":  s.brain.Cfg.KeepQuiet,
		"rerouting":   speech.Rerouting(),
		"match":       s.brain.Cfg.VoiceMatch,
		"wanted":      voiceprint.SamplesWanted,
		"megabytes":   voiceprint.AboutMegabytes,
	}

	if blocked := s.onlyMeBlocked(); blocked != "" {
		body["only_me_blocked"] = blocked
	}

	if known != nil {
		body["samples"] = known.Samples
		body["agreement"] = known.Agreement
	}

	ok(w, body)
}

/*
 * handleTeachVoice listens once and adds what it hears to the voiceprint.
 *
 * Recorded here rather than uploaded from the page: the microphone the brain
 * listens through is the one the print has to describe, and a voice enrolled
 * through a different one describes a different signal chain.
 */
func (s *Server) handleTeachVoice(w http.ResponseWriter, r *http.Request) {
	if !voiceprint.Installed() {
		fail(w, http.StatusBadRequest,
			"The model that tells voices apart is not installed yet.")

		return
	}

	var body struct {
		Device string `json:"device"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

	// Asked for directly — somebody is on the Privacy page teaching it their
	// voice and is waiting for this exact recording.
	heard, err := speech.ListenForTurn(r.Context(), body.Device, true)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	known, err := voiceprint.Learn(s.brain.Root, heard.Samples)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	s.log.Info("learned more of its owner's voice",
		"samples", known.Samples, "agreement", known.Agreement)

	ok(w, map[string]any{
		"samples":   known.Samples,
		"agreement": known.Agreement,
		"wanted":    voiceprint.SamplesWanted,
		"heard":     heard.Text,
		"enough":    known.Samples >= voiceprint.SamplesWanted,
	})
}

// handleForgetVoice removes the voiceprint. One press, and nothing about
// anybody's voice is left on the machine.
func (s *Server) handleForgetVoice(w http.ResponseWriter, r *http.Request) {
	if err := voiceprint.Forget(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	/*
	 * And the setting that depends on it goes with it.
	 *
	 * Leaving "answer only me" on with no voice to compare against would mean
	 * either ignoring everybody or ignoring nobody, and which of those it did
	 * would depend on a detail nobody can see.
	 */
	cfg := s.brain.Cfg
	cfg.OnlyMe = false

	if err := cfg.Save(s.brain.Root); err == nil {
		s.brain.Cfg = cfg
	}

	ok(w, map[string]any{"forgotten": true})
}

// installVoiceprint is voiceprint.Install; a variable so a test can stand in
// for the download.
var installVoiceprint = voiceprint.Install

// installing is true from the press of the button until its download ends.
var installing atomic.Bool

/*
 * handleInstallVoiceprint downloads the model and the runtime.
 *
 * Long — forty megabytes on a home connection — so it runs in the background
 * and the page watches the same progress line everything else here uses.
 *
 * One at a time. A second press, or a press while the refresh at start is
 * fetching the runtime, is told a download is already under way rather than
 * starting another into the same folder (voiceprint keeps the files apart
 * as well; this keeps the network and the page honest).
 *
 * Its progress is background work: Aside and BackgroundDone, so a question
 * asked while it downloads keeps its own step on screen.
 */
func (s *Server) handleInstallVoiceprint(w http.ResponseWriter, r *http.Request) {
	if voiceprint.Installed() {
		ok(w, map[string]any{"installed": true})

		return
	}

	if voiceprint.Downloading() || !installing.CompareAndSwap(false, true) {
		ok(w, map[string]any{"started": false, "downloading": true, "megabytes": voiceprint.AboutMegabytes})

		return
	}

	go func() {
		defer installing.Store(false)

		progress.Aside("learning", "Fetching what tells voices apart")
		defer progress.BackgroundDone()

		if err := installVoiceprint(context.Background(), func(said string) {
			progress.Aside("learning", said)
		}); err != nil {
			s.log.Warn("could not fetch the voiceprint model", "error", err)
			progress.Aside("learning", "Could not fetch it: "+err.Error())
		}
	}()

	ok(w, map[string]any{"started": true, "megabytes": voiceprint.AboutMegabytes})
}

// refreshRuntime is voiceprint.Refresh; a variable so a test can stand in for
// the download.
var refreshRuntime = voiceprint.Refresh

/*
 * refreshVoiceprint replaces a voiceprint runtime left by an older release.
 *
 * Run once when the server starts. Each release of the program speaks to one
 * release of the runtime, and the runtime is a download rather than part of
 * the program, so updating the program alone would leave somebody who taught
 * it their voice with a library it can no longer load. Their choice to fetch
 * it was made once already; this only keeps it matching.
 *
 * Not while looking things up online is switched off: then the page says the
 * part is missing and offers the button, which asks first.
 */
func (s *Server) refreshVoiceprint(ctx context.Context) {
	if !s.brain.Cfg.LookOnline {
		if voiceprint.RuntimeStale() {
			s.log.Info("the voiceprint runtime is from an older release and looking online is off; " +
				"it will be fetched when asked for on the Privacy page")
		}

		return
	}

	started := false

	// Background work: said aside and ended with BackgroundDone, so a
	// question answered while it downloads keeps its step on screen.
	replaced, err := refreshRuntime(ctx, func(said string) {
		started = true

		progress.Aside("learning", said)
	})

	if started {
		defer progress.BackgroundDone()
	}

	if err != nil {
		// The page reads the part as missing and offers the button again.
		s.log.Warn("could not update the voiceprint runtime", "error", err)

		return
	}

	if replaced {
		s.log.Info("updated the voiceprint runtime", "version", voiceprint.RuntimeVersion)
	}
}
