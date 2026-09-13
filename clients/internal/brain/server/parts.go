package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"pn-scripts-assistant/internal/brain/storage"
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

		// Removable is whether this program put it there and can take it away.
		// False for the system packages, which are shared with the rest of the
		// machine — see Requirement.RemoveFunc.
		Removable bool `json:"removable"`

		// OnDisk is what removing it would give back, measured rather than
		// declared: a 30MB download can be half a gigabyte once built.
		OnDisk     int64  `json:"on_disk,omitempty"`
		OnDiskText string `json:"on_disk_text,omitempty"`

		// FreeGB is what is left on the filesystem it lives on.
		FreeGB    int  `json:"free_gb,omitempty"`
		NeedsRoot bool `json:"needs_password"`

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
			Removable:   r.Removable(),
			NeedsRoot:   r.NeedsRoot,
			Hint:        r.ManualHint,
		}

		if r.Where != nil {
			p.Where = r.Where()
		}

		/*
		 * Measured only for what is installed and removable.
		 *
		 * Walking a directory tree is not free and this list is read on a
		 * timer, so it is done for the four pieces where the answer is used —
		 * the ones the Remove list offers — and skipped for everything else.
		 */
		if p.Removable && state == preflight.OK {
			p.OnDisk = r.SizeOnDisk()
			p.OnDiskText = storage.InWords(p.OnDisk)
		}

		if p.Where != "" {
			if _, usable, err := storage.SpaceOn(p.Where); err == nil {
				p.FreeGB = int(usable / (1 << 30))
			}
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
	// The same entry point setup uses, so a piece installed from the running
	// program and one installed on the first day arrive by the same road —
	// including the swap from sudo to pkexec, which is what makes a password
	// prompt possible when there is no terminal to type into.
	//
	// Installing, verifying and trimming the output live in preflight now, so
	// this path and the one the assistant reaches by talking cannot drift.
	return preflight.InstallAndVerify(req)
}

/*
 * handleRemovePart takes one piece back off the machine.
 *
 * The counterpart of installing, and it has to exist here for the same reason
 * installing does: the alternative is knowing which nine directories to delete
 * and doing it in a terminal, which is the wall this program exists to remove.
 *
 * Only pieces this program installed. The system packages have no remover, and
 * this refuses rather than shelling out to apt: removing poppler because the
 * assistant offered a button would break printing, and nobody pressing a
 * button in an assistant expects that.
 */
func (s *Server) handleRemovePart(w http.ResponseWriter, r *http.Request) {
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

		if !req.Removable() {
			fail(w, http.StatusBadRequest, req.Name+" came with your system rather than "+
				"from here, so removing it is your package manager's job — other things "+
				"on this machine may be using it.")

			return
		}

		if s.brain.Jobs == nil {
			fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

			return
		}

		part := req

		if _, err := s.brain.Jobs.Start("Removing "+part.Name, func(context.Context) (string, error) {
			return preflight.RemoveAndVerify(part)
		}); err != nil {
			fail(w, http.StatusConflict, err.Error())

			return
		}

		s.brain.Log.Info("removing a part from the program", "part", part.Name)

		ok(w, map[string]any{
			"name":    part.Name,
			"started": "Removing " + part.Name + ". It appears under Activity while it runs.",
		})

		return
	}

	fail(w, http.StatusNotFound, "I do not know a part by that name.")
}

/*
 * handleRemoveModel deletes one model, by name.
 *
 * Its own endpoint rather than part of removing Ollama, because those are
 * different decisions and somebody clearing space usually wants the second
 * without the first: models are gigabytes each, Ollama is thirty-eight
 * megabytes, and removing the thing that runs them to reclaim the space they
 * take is the wrong end of the problem.
 *
 * Refuses to remove the two the brain is currently using without being told
 * twice. Deleting the model it thinks with leaves a program that starts,
 * accepts what you say and answers nothing — and deleting the embedding model
 * leaves one that remembers nothing while behaving as though it does.
 */
func (s *Server) handleRemoveModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`

		// AnyModel allows removing one that is in use, once somebody has been
		// told which it is.
		AnyModel bool `json:"even_if_in_use"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil ||
		strings.TrimSpace(body.Name) == "" {
		fail(w, http.StatusBadRequest, "Which model?")

		return
	}

	name := strings.TrimSpace(body.Name)
	bare := strings.TrimSuffix(name, ":latest")

	if !body.AnyModel {
		for what, using := range map[string]string{
			"thinks with":    s.brain.Cfg.OllamaModel,
			"remembers with": s.brain.Cfg.EmbedModel,
		} {
			if using == "" {
				continue
			}

			if using == name || strings.TrimSuffix(using, ":latest") == bare {
				fail(w, http.StatusConflict, "That is the model it "+what+
					" right now. Choose another one first, or remove it anyway.")

				return
			}
		}
	}

	if s.brain.Jobs == nil {
		fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

		return
	}

	if _, err := s.brain.Jobs.Start("Removing "+name, func(context.Context) (string, error) {
		// ollama's own command: the store is content-addressed and layers are
		// shared between models, so deleting files by hand would either leave
		// most of the space taken or break a model that shares them.
		out, err := exec.CommandContext(context.Background(), "ollama", "rm", name).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("removing %s: %w %s", name, err,
				strings.TrimSpace(string(out)))
		}

		return name + " has been removed.", nil
	}); err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	s.brain.Log.Info("removing a model from the program", "model", name)

	ok(w, map[string]any{"name": name, "started": "Removing " + name + "."})
}
