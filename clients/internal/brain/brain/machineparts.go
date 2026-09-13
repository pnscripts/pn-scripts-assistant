package brain

import (
	"context"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * What the machine-work tools stand on.
 *
 * The same requirement list the setup screen and the Parts panel read, turned
 * into the shape the tools speak. Not a second list: the reason four
 * capabilities went unchecked for months is that somebody had to remember to
 * add them in two places, and nobody did.
 */

// machineParts is the requirement list as the tools see it.
func machineParts() []tools.MachinePart {
	reqs := preflight.Requirements()
	out := make([]tools.MachinePart, 0, len(reqs))

	for _, r := range reqs {
		state, detail := r.Check()

		p := tools.MachinePart{
			Name:        r.Name,
			Why:         r.Why,
			Consequence: r.Consequence,
			State:       state.Label(),
			Detail:      detail,
			Size:        r.Size,
			Optional:    r.Optional,
			Installable: r.InstallFunc != nil || r.InstallCmd != nil,
			Removable:   r.Removable(),
		}

		if r.Where != nil {
			p.Where = r.Where()
		}

		out = append(out, p)
	}

	return out
}

/*
 * startInstalling puts an install behind the conversation.
 *
 * Behind it rather than in it: a model is gigabytes and the recogniser is
 * minutes of compiling, and a turn that waits for either is a turn that has
 * stopped answering. The job announces itself when it finishes, so the answer
 * to "install the voice" is "I have started, it is 488MB" and the news of it
 * landing arrives on its own.
 */
func (b *Brain) startInstalling(name string) (string, error) {
	for _, req := range preflight.Requirements() {
		if req.Name != name {
			continue
		}

		if req.InstallFunc == nil && req.InstallCmd == nil {
			hint := req.ManualHint
			if hint != "" {
				hint = " " + hint
			}

			return "", fmt.Errorf("%s cannot be installed from here.%s", req.Name, hint)
		}

		if b.Jobs == nil {
			return "", fmt.Errorf("there is nowhere to run that")
		}

		part := req

		if _, err := b.Jobs.Start("Installing "+part.Name, func(context.Context) (string, error) {
			return preflight.InstallAndVerify(part)
		}); err != nil {
			return "", err
		}

		b.Log.Info("installing a part, asked for in conversation", "part", part.Name)

		size := ""
		if part.Size != "" {
			size = " It is " + part.Size + "."
		}

		return fmt.Sprintf("Installing %s in the background.%s I will say when it is done.",
			part.Name, size), nil
	}

	return "", fmt.Errorf("there is no piece called %q", name)
}

/*
 * startPulling downloads one named model.
 *
 * Ollama resumes a partial pull, so a download interrupted by closing the
 * program is continued rather than restarted — which is the difference
 * between losing four gigabytes and losing nothing.
 */
func (b *Brain) startPulling(name string) (string, error) {
	if b.Jobs == nil {
		return "", fmt.Errorf("there is nowhere to run that")
	}

	client := models.New(b.Cfg.OllamaURL)

	wanted := strings.TrimSpace(name)
	if wanted == "" {
		return "", fmt.Errorf("say which model to download")
	}

	if _, err := b.Jobs.Start("Downloading "+wanted, func(ctx context.Context) (string, error) {
		if err := client.Pull(ctx, wanted, nil); err != nil {
			return "", fmt.Errorf("downloading %s: %w", wanted, err)
		}

		return wanted + " is downloaded and ready to use.", nil
	}); err != nil {
		return "", err
	}

	b.Log.Info("downloading a model, asked for in conversation", "model", wanted)

	return "Downloading " + wanted + " in the background. I will say when it is ready.", nil
}

/*
 * startRemoving takes a piece off, behind the conversation.
 *
 * Backgrounded like installing, and for a smaller version of the same reason:
 * removing whisper.cpp is deleting a built source tree of several thousand
 * files, which is not instant on a spinning disk or a memory card.
 */
func (b *Brain) startRemoving(name string) (string, error) {
	for _, req := range preflight.Requirements() {
		if req.Name != name {
			continue
		}

		if !req.Removable() {
			return "", fmt.Errorf("%s came with your system rather than from here, so "+
				"removing it is your package manager's job — other things on this "+
				"machine may be using it", req.Name)
		}

		if b.Jobs == nil {
			return "", fmt.Errorf("there is nowhere to run that")
		}

		part := req

		if _, err := b.Jobs.Start("Removing "+part.Name, func(context.Context) (string, error) {
			return preflight.RemoveAndVerify(part)
		}); err != nil {
			return "", err
		}

		b.Log.Info("removing a part, asked for in conversation", "part", part.Name)

		return "Removing " + part.Name + ". I will say when it is gone.", nil
	}

	return "", fmt.Errorf("there is no piece called %q", name)
}
