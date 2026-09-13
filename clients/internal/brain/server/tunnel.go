package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/pair"
	"pn-scripts-assistant/internal/brain/tunnel"
)

/*
 * Reaching it from outside the house.
 *
 * A tunnel rather than a relay, so nothing new is exposed to the internet: the
 * phone joins the home network, the brain goes on answering only its own
 * network, and every rule already built — the pairing, the tokens, the
 * decisions staying at the desk — holds unchanged, because from the brain's
 * point of view nothing has changed at all.
 *
 * All of this is at-the-desk-only. Setting up the way in from outside is not
 * something to be done from outside.
 */
func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	_, local, _ := net.SplitHostPort(s.brain.Cfg.Addr)

	// Which paired devices have a way in, and which do not, so the list is
	// about devices rather than about keys.
	ways := map[string]any{}

	for _, p := range t.Peers {
		ways[p.Device] = map[string]any{
			"address":   p.Address,
			"added":     p.Added,
			"collected": p.Private == "",
		}
	}

	ok(w, map[string]any{
		"installed": tunnel.Installed(),
		"started":   t.Started(),
		"up":        tunnel.Up(),
		"endpoint":  t.Endpoint,
		"port":      t.Port,
		"missing":   t.Ready(),
		"ways":      ways,
		"devices":   s.paired.List(),

		// Where a device goes once it is through, and the one thing only its
		// owner can do.
		"inside":  "https://" + tunnel.Machine + ":" + pair.NetworkPort(local),
		"forward": fmt.Sprintf("UDP port %d, to this computer", t.Port),
	})
}

/*
 * handleTunnelSetup makes this machine's key and records where the house is.
 *
 * The address is typed rather than looked up. Looking it up means asking a
 * service on the internet what somebody's home address is, which is exactly
 * the sort of small favour this program does not do.
 */
func (s *Server) handleTunnelSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if err := t.Start(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	t.Endpoint = strings.TrimSpace(body.Endpoint)

	if err := t.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"started": true, "missing": t.Ready()})
}

// handleTunnelAdd gives one paired device a way in from outside.
func (s *Server) handleTunnelAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	device, found := findDevice(s.paired.List(), id)
	if !found {
		fail(w, http.StatusNotFound, "that device is not paired")

		return
	}

	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if _, err := t.Add(device.ID, device.Name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if err := t.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"added": device.ID})
}

/*
 * handleTunnelConfig hands a device its own configuration, once.
 *
 * Collected over the home network while the device is still at home, which is
 * the whole reason this is a download and not a QR code: the device can
 * already reach this, because it was paired here. Nothing has to be
 * photographed, and nothing is typed.
 *
 * The key is forgotten afterwards, so the only copy is on the device it
 * belongs to.
 */
func (s *Server) handleTunnelConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	peer, found := t.Peer(id)
	if !found {
		fail(w, http.StatusNotFound, "that device has no way in yet")

		return
	}

	if peer.Private == "" {
		fail(w, http.StatusGone,
			"that device already took its key, and there is only ever one copy. "+
				"Remove its way in and give it a new one")

		return
	}

	_, local, _ := net.SplitHostPort(s.brain.Cfg.Addr)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+tunnel.Interface+`.conf"`)

	fmt.Fprint(w, t.PeerConfig(peer, pair.NetworkPort(local)))

	// Forgotten only once it has actually been written out.
	t.Collected(id)

	if err := t.Save(s.brain.Root); err != nil {
		s.log.Warn("could not forget a collected key", "device", id, "error", err)
	}
}

// handleTunnelRemove takes a device's way in away without unpairing it.
func (s *Server) handleTunnelRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	t.Remove(id)

	if err := t.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": id})
}

/*
 * handleTunnelRun brings the tunnel up or takes it down.
 *
 * Creating a network interface is something only the system may do, so this
 * asks for a password once, through the same graphical prompt the installer
 * uses. What it is about to run is in the answer to the status call, written
 * out — because "give me your password" with no sentence after it is how
 * people learn to type it without reading.
 */
func (s *Server) handleTunnelRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Up bool `json:"up"`
	}

	json.NewDecoder(r.Body).Decode(&body)

	t, err := tunnel.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if !body.Up {
		if err := tunnel.Stop(s.asRoot); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

		ok(w, map[string]any{"up": false, "said": "The tunnel is down."})

		return
	}

	if err := tunnel.Start(r.Context(), s.brain.Root, t, s.asRoot); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{
		"up": true,
		"said": "The tunnel is up. It stays up until this computer restarts — " +
			"there is a command in the panel to make it start on its own.",
	})
}

/*
 * asRoot runs one command with a graphical password prompt.
 *
 * pkexec rather than sudo, for the reason the installer already found: sudo
 * has nowhere to prompt when there is no terminal, and simply hangs.
 */
func (s *Server) asRoot(argv []string) ([]byte, error) {
	if _, err := exec.LookPath("pkexec"); err != nil {
		return nil, fmt.Errorf(
			"there is no way to ask for a password on this machine; run it in a " +
				"terminal instead")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pkexec", append([]string{"--"}, argv...)...)

	return cmd.CombinedOutput()
}

func findDevice(devices []pair.Device, id string) (pair.Device, bool) {
	for _, d := range devices {
		if d.ID == id {
			return d, true
		}
	}

	return pair.Device{}, false
}
