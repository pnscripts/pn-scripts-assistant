package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-brain/internal/brain/smarthome"
)

/*
 * The things in the house.
 *
 * The code to talk to Home Assistant has been here the whole time and there
 * was no interface at all — the address and the token could only be typed into
 * the settings file, and nothing anywhere showed whether it had worked. So the
 * capability existed in the sense that a library existed.
 *
 * The token is treated the way the model keys are: never sent back, only
 * whether one is set and its last four characters. It is a key to somebody's
 * front door lock as often as to their lamps.
 */

// handleDevices lists what is connected, or says plainly why nothing is.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	cfg := s.brain.Cfg

	answer := map[string]any{
		"url":        cfg.HomeAssistantURL,
		"has_token":  cfg.HomeAssistantToken != "",
		"token_tail": tail(cfg.HomeAssistantToken),
		"connected":  false,
		"devices":    []smarthome.Device{},
	}

	home := smarthome.New(cfg.HomeAssistantURL, cfg.HomeAssistantToken)

	if !home.Configured() {
		/*
		 * Which half is missing, not "not configured".
		 *
		 * An address with no token and a token with no address are different
		 * mistakes with different next steps, and telling somebody neither is
		 * how a setup page becomes a guessing game.
		 */
		switch {
		case cfg.HomeAssistantURL == "" && cfg.HomeAssistantToken == "":
			answer["why"] = "Not set up yet. It needs the address of your Home " +
				"Assistant and a long-lived access token from your profile page there."

		case cfg.HomeAssistantURL == "":
			answer["why"] = "There is a token but no address."

		default:
			answer["why"] = "There is an address but no token. Home Assistant makes " +
				"one under your profile, at the bottom, called a long-lived access token."
		}

		ok(w, answer)

		return
	}

	devices, err := home.Devices(r.Context())
	if err != nil {
		answer["why"] = "Could not reach it: " + err.Error()

		ok(w, answer)

		return
	}

	answer["connected"] = true
	answer["devices"] = devices
	answer["why"] = ""

	ok(w, answer)
}

// handleConnectDevices stores the address and token for the house.
func (s *Server) handleConnectDevices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL   *string `json:"url"`
		Token *string `json:"token"`

		// Forget is its own field for the same reason as the model keys: an
		// empty box means "leave it alone", never "disconnect my house".
		Forget bool `json:"forget"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	cfg := s.brain.Cfg

	if body.URL != nil {
		cfg.HomeAssistantURL = strings.TrimRight(strings.TrimSpace(*body.URL), "/")
	}

	if body.Token != nil && strings.TrimSpace(*body.Token) != "" {
		cfg.HomeAssistantToken = strings.TrimSpace(*body.Token)
	}

	if body.Forget {
		cfg.HomeAssistantURL = ""
		cfg.HomeAssistantToken = ""
	}

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "I could not write it down: "+err.Error())

		return
	}

	s.brain.Cfg = cfg

	/*
	 * Rebuilt so it works now rather than after a restart.
	 *
	 * The tools hold a client built at startup from the old settings, so
	 * typing an address into a running program would otherwise change the file
	 * and nothing else — the same bug privacy and the model keys both had.
	 */
	s.brain.ReloadHome()

	s.brain.Log.Info("home assistant connection changed", "url", cfg.HomeAssistantURL)

	ok(w, map[string]any{"saved": true})
}

/*
 * handleSetDevice turns something on or off.
 *
 * Behind the same permission as everything else that acts: this is somebody's
 * lights, and in a good many houses their locks. The brain asks before doing
 * it and so does the page — the difference is that here a person is looking at
 * the switch they are pressing, which is its own kind of consent.
 */
func (s *Server) handleSetDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	if strings.TrimSpace(body.ID) == "" {
		fail(w, http.StatusBadRequest, "Which device?")

		return
	}

	home := smarthome.New(s.brain.Cfg.HomeAssistantURL, s.brain.Cfg.HomeAssistantToken)

	if !home.Configured() {
		fail(w, http.StatusBadRequest, "Nothing is connected yet.")

		return
	}

	if err := home.SetState(r.Context(), body.ID, body.State); err != nil {
		fail(w, http.StatusBadGateway, err.Error())

		return
	}

	s.brain.Log.Info("device changed from the panel", "device", body.ID, "state", body.State)

	ok(w, map[string]any{"id": body.ID, "state": body.State})
}
