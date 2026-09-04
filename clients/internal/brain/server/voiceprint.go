package server

import (
	"context"
	"encoding/json"
	"net/http"

	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/voiceprint"
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
		return voiceprint.Verdict{Why: "the voiceprint model is not installed"}
	}

	match := s.brain.Cfg.VoiceMatch

	if match <= 0 {
		match = 0.5
	}

	return voiceprint.Recognise(s.brain.Root, samples, match)
}

/*
 * handleVoiceprint says what the brain knows about its owner's voice.
 *
 * Three separate facts, because they fail separately and the fix differs: the
 * model can be missing, no voice can have been taught, and the setting to act
 * on it can be off.
 */
func (s *Server) handleVoiceprint(w http.ResponseWriter, r *http.Request) {
	known, _ := voiceprint.Load(s.brain.Root)

	body := map[string]any{
		"installed": voiceprint.Installed(),
		"enrolled":  known != nil && len(known.Print) == voiceprint.Dimensions,
		"only_me":   s.brain.Cfg.OnlyMe,
		"match":     s.brain.Cfg.VoiceMatch,
		"wanted":    voiceprint.SamplesWanted,
		"megabytes": voiceprint.AboutMegabytes,
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

	heard, err := speech.ListenForTurn(r.Context(), body.Device)
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
 * handleInstallVoiceprint downloads the model and the runtime.
 *
 * Long — forty megabytes on a home connection — so it runs in the background
 * and the page watches the same progress line everything else here uses.
 */
func (s *Server) handleInstallVoiceprint(w http.ResponseWriter, r *http.Request) {
	if voiceprint.Installed() {
		ok(w, map[string]any{"installed": true})

		return
	}

	go func() {
		progress.SetBackground("learning", "Fetching what tells voices apart")
		defer progress.Done()

		if err := voiceprint.Install(context.Background(), func(said string) {
			progress.SetBackground("learning", said)
		}); err != nil {
			s.log.Warn("could not fetch the voiceprint model", "error", err)
			progress.SetBackground("learning", "Could not fetch it: "+err.Error())
		}
	}()

	ok(w, map[string]any{"started": true, "megabytes": voiceprint.AboutMegabytes})
}
