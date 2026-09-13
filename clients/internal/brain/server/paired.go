package server

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/pair"
	"pn-scripts-assistant/internal/brain/tunnel"
)

/*
 * Which devices may reach this, and how one becomes one.
 *
 * Everything here except claiming a code is at-the-desk-only, which is the
 * point: pairing a device requires standing in front of the computer the brain
 * is on, and that is the whole of the security story. There is no account, no
 * password to guess, and nothing anywhere else to be broken into.
 */
func (s *Server) handlePaired(w http.ResponseWriter, r *http.Request) {
	code, until, can := s.paired.Waiting()

	certificate, _ := pair.Certificate(s.brain.Root)

	// The port the encrypted listener is on, not the local one. They differ by
	// one, and telling somebody the wrong one is the whole feature failing in
	// a way that looks like their network being broken.
	_, local, _ := net.SplitHostPort(s.brain.Cfg.Addr)

	port := pair.NetworkPort(local)

	ok(w, map[string]any{
		"devices": s.paired.List(),
		"reach":   s.brain.Cfg.Reach,
		"open":    s.brain.Cfg.OpenToNetwork(),

		// The code being offered right now, so the screen can keep showing it
		// and stop when it runs out.
		"code":       code,
		"code_until": until,
		"code_can":   can,

		// Where to go and what to check, for somebody holding a phone.
		"addresses":   pair.Addresses(port),
		"fingerprint": pair.Fingerprint(certificate),
	})
}

// handleOfferCode puts a code on the screen for somebody to type into a device
// in the same room.
func (s *Server) handleOfferCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Can string `json:"can"`
	}

	json.NewDecoder(r.Body).Decode(&body)

	code, until, err := s.paired.Offer(body.Can)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"code": code, "until": until, "for": HowLongToType(until)})
}

// HowLongToType is the time left, in words, so a screen can say it rather than
// making somebody work out a clock difference.
func HowLongToType(until time.Time) string {
	left := time.Until(until).Round(time.Second)

	if left <= 0 {
		return "gone"
	}

	if left < time.Minute {
		return "under a minute"
	}

	return "about " + itoaMinutes(int(left.Minutes())) + " minutes"
}

func itoaMinutes(n int) string {
	switch n {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	default:
		return "several"
	}
}

func (s *Server) handleStopOffering(w http.ResponseWriter, r *http.Request) {
	s.paired.StopOffering()

	ok(w, map[string]any{"stopped": true})
}

/*
 * handleClaim is the one route a stranger may call.
 *
 * It is how a device stops being a stranger, so it cannot require being one
 * already. What protects it is that the code is short-lived, single-use, and
 * only exists while somebody is standing at the computer having asked for it —
 * and that a wrong guess spends it, so there is nothing to try repeatedly.
 */
func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	device, token, err := s.paired.Accept(body.Code, body.Name, fromAddress(r))
	if err != nil {
		fail(w, http.StatusForbidden, err.Error())

		return
	}

	s.log.Info("a device was paired", "device", device.ID, "name", device.Name,
		"can", device.Can, "from", device.From)

	/*
	 * The token goes back as a cookie, and is never shown again.
	 *
	 * Strict, so a page on another site cannot make a request that carries it.
	 * Secure, because this only ever travels over TLS off the machine. And
	 * long-lived, because a device that has to be paired again every week is a
	 * device somebody stops pairing and starts working around.
	 */
	http.SetCookie(w, &http.Cookie{
		Name:     TokenCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().AddDate(1, 0, 0),
	})

	ok(w, map[string]any{"device": device.ID, "name": device.Name, "can": device.Can})
}

// handleForgetDevice unpairs one. A phone left in a taxi is the case this
// exists for, so it takes effect at once rather than at the next restart.
func (s *Server) handleUnpair(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.paired.Revoke(id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	/*
	 * And its way in from outside goes with it.
	 *
	 * A phone left in a taxi is the case all of this exists for, and a device
	 * unpaired but still holding a tunnel key would still be able to reach the
	 * home network — where it would find a locked door, but a locked door is
	 * not what somebody unpairing a lost phone is asking for.
	 */
	if t, err := tunnel.Load(s.brain.Root); err == nil && t.Remove(id) {
		if err := t.Save(s.brain.Root); err != nil {
			s.log.Warn("could not take away a device's way in", "device", id, "error", err)
		} else {
			s.log.Info("a device's way in from outside was removed too", "device", id)
		}
	}

	s.log.Info("a device was unpaired", "device", id)

	ok(w, map[string]any{"forgotten": id})
}

/*
 * handleReach opens or closes the door.
 *
 * Refused unless something is already paired, and that is not fussiness: a
 * door opened for nobody is all of the risk and none of the point, and turning
 * the setting on before pairing anything is exactly the order somebody would
 * naturally do it in.
 *
 * It takes effect on the next start rather than at once, because the listener
 * is bound. Said plainly in the answer rather than left to be discovered.
 */
func (s *Server) handleReach(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reach string `json:"reach"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	wanted := config.ReachHere

	if body.Reach == config.ReachNetwork {
		if !s.paired.Any() {
			fail(w, http.StatusBadRequest,
				"pair a device first — opening this for nobody is all of the risk and "+
					"none of the point")

			return
		}

		wanted = config.ReachNetwork
	}

	cfg := s.brain.Cfg
	cfg.Reach = wanted

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	s.brain.Cfg = cfg

	s.log.Info("how far the assistant answers was changed", "reach", wanted)

	said := "It will answer only on this computer from the next time it starts."

	if wanted == config.ReachNetwork {
		said = "From the next time it starts, paired devices on your network can reach " +
			"it. Nothing else can."
	}

	ok(w, map[string]any{"reach": wanted, "said": said})
}
