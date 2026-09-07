package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/preflight"
)

/*
 * Installing the missing pieces from inside the program.
 *
 * Setup could do this and the running program could not, so anything not
 * chosen on the first day meant going back to setup — which is a screen for
 * starting, not for changing your mind six weeks later. "The models and other
 * things must be able to install from inside the program, from lists."
 *
 * The same list setup reads, not a second copy of it. A parallel list is a
 * list that goes stale: four capabilities were used by this program and
 * checked by nothing for months precisely because nobody remembered to add
 * them in two places.
 */

// handleParts lists everything the brain runs on and what state it is in.
func (s *Server) handleParts(w http.ResponseWriter, r *http.Request) {
	type part struct {
		Name string `json:"name"`

		// Why it exists and what is lost without it, both, because those are
		// two different questions and a list that answers only the first
		// cannot be judged by somebody deciding what to install.
		Why         string `json:"why"`
		Consequence string `json:"consequence"`

		State  string `json:"state"`
		Detail string `json:"detail,omitempty"`
		Where  string `json:"where,omitempty"`

		// Size is what the download costs, which is the question everybody has
		// and nothing was answering.
		Size string `json:"size,omitempty"`

		Optional    bool `json:"optional"`
		Installable bool `json:"installable"`
		NeedsRoot   bool `json:"needs_password"`

		Hint string `json:"hint,omitempty"`
	}

	out := []part{}

	for _, r := range preflight.Requirements() {
		state, detail := r.Check()

		p := part{
			Name:        r.Name,
			Why:         r.Why,
			Consequence: r.Consequence,
			State:       state.Label(),
			Detail:      detail,
			Size:        r.Size,
			Optional:    r.Optional,
			Installable: r.InstallFunc != nil || r.InstallCmd != nil,
			NeedsRoot:   r.NeedsRoot,
			Hint:        r.ManualHint,
		}

		if r.Where != nil {
			p.Where = r.Where()
		}

		out = append(out, p)
	}

	ok(w, map[string]any{"parts": out})
}

/*
 * handleInstallPart installs one of them, behind the conversation.
 *
 * A background job rather than a request that waits: building the recogniser
 * is minutes and a model is gigabytes, and a page holding a connection open
 * for either is a page that looks hung and a request that times out halfway
 * through something it cannot undo.
 *
 * Mutating in every sense — it writes to the machine and some of it asks for a
 * password — so it is a deliberate press on a named thing, never something the
 * brain decides to do.
 */
func (s *Server) handleInstallPart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	wanted := strings.TrimSpace(body.Name)

	for _, req := range preflight.Requirements() {
		if req.Name != wanted {
			continue
		}

		if req.InstallFunc == nil && req.InstallCmd == nil {
			fail(w, http.StatusBadRequest,
				"That one cannot be installed from here. "+req.ManualHint)

			return
		}

		if s.brain.Jobs == nil {
			fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

			return
		}

		part := req

		_, err := s.brain.Jobs.Start("Installing "+part.Name, func(ctx context.Context) (string, error) {
			return installOne(ctx, part)
		})
		if err != nil {
			fail(w, http.StatusConflict, err.Error())

			return
		}

		s.brain.Log.Info("installing a part from the program", "part", part.Name)

		ok(w, map[string]any{
			"name": part.Name,
			"started": fmt.Sprintf(
				"Installing %s in the background. It appears under Activity while it runs.",
				part.Name),
		})

		return
	}

	fail(w, http.StatusNotFound, "I do not know a part by that name.")
}

/*
 * installOne runs whichever kind of install this requirement has.
 *
 * The output is kept and returned rather than discarded, because when one of
 * these fails the reason is in what the installer printed — and "it did not
 * work" about a twenty-minute build is not an answer anybody can act on.
 */
func installOne(_ context.Context, req preflight.Requirement) (string, error) {
	var said strings.Builder

	// The same entry point setup uses, so a piece installed from the running
	// program and one installed on the first day arrive by the same road —
	// including the swap from sudo to pkexec, which is what makes a password
	// prompt possible when there is no terminal to type into.
	if err := preflight.Install(req, &said); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", req.Name, err, lastFew(said.String()))
	}

	// Checked afterwards rather than trusted: an installer that exits zero and
	// leaves nothing behind is a thing that happens, and reporting success for
	// it means somebody looks for the feature and does not find it.
	if state, detail := req.Check(); state != preflight.OK {
		return "", fmt.Errorf(
			"%s finished but is still not there%s", req.Name,
			map[bool]string{true: ": " + detail, false: ""}[detail != ""])
	}

	return fmt.Sprintf("%s is installed.", req.Name), nil
}

// lastFew is the end of an installer's output, which is where it says what
// went wrong. The rest is a progress bar drawn in text.
func lastFew(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}

	return strings.Join(lines, "\n")
}

// so the io import is used by the InstallFunc signature above.
var _ io.Writer = (*strings.Builder)(nil)
