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
		return voiceprint.Verdict{Why: "nothing was recorded", Reason: voiceprint.ReasonNothingRecorded}
	}

	// Not offered since 1.0.1: nothing measures a voice, and nothing is loaded.
	if !voiceprint.Offered() {
		return voiceprint.Verdict{Why: "telling voices apart is not offered in this version",
			Reason: voiceprint.ReasonWithdrawn}
	}

	if !voiceprint.Installed() {
		// After an upgrade the model is here and the runtime is the previous
		// release's, which is a different fix from never having fetched it.
		if voiceprint.RuntimeStale() {
			return voiceprint.Verdict{Why: "the part that tells voices apart is from an older version " +
				"and has not been updated yet", Reason: voiceprint.ReasonStale}
		}

		return voiceprint.Verdict{Why: "the voiceprint model is not installed", Reason: voiceprint.ReasonNotInstalled}
	}

	match := s.brain.Cfg.VoiceMatch

	if match <= 0 {
		match = 0.5
	}

	return voiceprint.Recognise(s.brain.Root, samples, match)
}

/*
 * refusal is why "answer only me" did not let a turn through: the sentence,
 * and the same as a key and values the page says in the reader's language
 * (i18n.js 'onlyme.*', drawn by onlyme.js). Text alone was English on a
 * Bulgarian page.
 */
type refusal struct {
	Text string
	Key  string
	Vars map[string]any
}

// fields are what a turn's answer carries about the refusal, none when there
// was none: the sentence, and its key and values for the page (onlyme.js).
func (r refusal) fields() map[string]any {
	if r.Text == "" {
		return nil
	}

	return map[string]any{"refused": r.Text, "refused_key": r.Key, "refused_vars": r.Vars}
}

/*
 * onlyMeRefuses says why a turn is not acted on because of "answer only me",
 * or nothing (an empty Text) when that switch lets it through.
 *
 * Somebody else's voice is refused, and so is a voice that could not be
 * recognised at all. The second used to pass: after an upgrade the runtime of
 * the previous release counts as missing, no voice was judged, and everybody
 * was answered while the switch still showed on. A protection that turns off
 * by itself without saying so is worse than none, because it is relied on.
 */
func (s *Server) onlyMeRefuses(whose voiceprint.Verdict) refusal {
	// With no model offered (1.0.1) the switch means nothing, and holding
	// every turn would answer nobody: every voice is answered, as before the
	// feature existed. brain.New switches it off; this covers a settings file
	// edited by hand afterwards.
	if !s.brain.Cfg.OnlyMe || !voiceprint.Offered() {
		return refusal{}
	}

	if !whose.Judged {
		return refusal{
			Text: "answer only me is on, and your voice cannot be recognised now: " + whose.Why,
			Key:  "onlyme.unrecognised",
			Vars: map[string]any{"why": whose.Reason},
		}
	}

	if !whose.Owner {
		match := s.brain.Cfg.VoiceMatch
		if match <= 0 {
			match = 0.5
		}

		return refusal{
			Text: fmt.Sprintf("that is not your voice (%.2f alike, %.2f needed)", whose.Alike, match),
			Key:  "onlyme.notYou",
			Vars: map[string]any{"alike": whose.Alike, "match": match},
		}
	}

	return refusal{}
}

// onlyMeBlocked is why "answer only me" cannot recognise anybody now, as a
// sentence and a Reason code, or "" when it can: for the Privacy page, while
// the switch shows on.
func (s *Server) onlyMeBlocked() (string, string) {
	if !s.brain.Cfg.OnlyMe || !voiceprint.Offered() {
		return "", ""
	}

	switch {
	case voiceprint.RuntimeStale():
		return "the part that tells voices apart is from an older version and has not been updated yet",
			voiceprint.ReasonStale
	case !voiceprint.Installed():
		return "the voiceprint model is not installed", voiceprint.ReasonNotInstalled
	case !voiceprint.Enrolled(s.brain.Root):
		return "no voice has been taught to this brain yet", voiceprint.ReasonNotTaught
	}

	return "", ""
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
		// Not offered since 1.0.1: the page says why and offers only to
		// forget a voice taught before.
		"withdrawn":   !voiceprint.Offered(),
		"installed":   voiceprint.Installed(),
		"downloading": installing.Load() || voiceprint.Downloading(),
		"enrolled":    known != nil && len(known.Print) == voiceprint.Dimensions,
		"only_me":     s.brain.Cfg.OnlyMe && voiceprint.Offered(),
		"cancel_room": s.brain.Cfg.CancelRoom,
		"keep_quiet":  s.brain.Cfg.KeepQuiet,
		"rerouting":   speech.Rerouting(),
		"match":       s.brain.Cfg.VoiceMatch,
		"wanted":      voiceprint.SamplesWanted,
		"megabytes":   voiceprint.AboutMegabytes,
	}

	if blocked, why := s.onlyMeBlocked(); blocked != "" {
		body["only_me_blocked"] = blocked
		body["only_me_blocked_why"] = why
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
	if !voiceprint.Offered() {
		fail(w, http.StatusBadRequest, withdrawnSentence)

		return
	}

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

/*
 * withdrawnSentence is the refusal when somebody asks for "Answer only me"
 * in a release that offers no model to tell voices apart. The page says it
 * in the reader's language ('voice.withdrawn'); this is for the API.
 */
const withdrawnSentence = "Answering only your voice is not offered in this version: " +
	"the model that told voices apart was trained on recordings licensed for research only, " +
	"so it was withdrawn. Every voice that addresses the assistant is answered."

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
	// Nothing to download while no model is offered (1.0.1).
	if !voiceprint.Offered() {
		fail(w, http.StatusBadRequest, withdrawnSentence)

		return
	}

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
		if voiceprint.Offered() && voiceprint.RuntimeStale() {
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
