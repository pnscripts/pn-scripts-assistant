package brain

import (
	"fmt"
	"path/filepath"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/pictures"
)

/*
 * Which way a picture gets made.
 *
 * The same fork as everything else that could leave this machine, and enforced
 * in one place so it cannot be forgotten in another. A description of what
 * somebody wants a picture of is often the most revealing sentence they will
 * write all week — more revealing than most of what they type into a chat —
 * so it goes to a company's server only when privacy has been opened, exactly
 * as a conversation does.
 *
 * Local first whenever it is possible, because a picture made here costs
 * minutes and a picture made there costs money and secrecy.
 */
type studioOf struct{ b *Brain }

func (s studioOf) Painter() (pictures.Painter, error) {
	local := pictures.Local{BaseURL: s.b.Cfg.PicturesURL}

	/*
	 * Privacy that keeps a conversation here keeps a picture here too.
	 *
	 * AllowsProvider, not AllowsWeb, and the difference is not academic: in
	 * research mode the brain may reach the internet while the model stays on
	 * this machine, so AllowsWeb is true — and a first version of this used it
	 * and would have sent a description of somebody's picture to a company's
	 * server in the mode whose entire purpose is that nothing of theirs is
	 * answered elsewhere.
	 *
	 * Whether it is running is not the question here. If nothing may leave,
	 * the only honest answer is the machine, and Local says what to do about
	 * it when asked.
	 */
	if !s.b.Mode.AllowsProvider("openai") {
		return local, nil
	}

	/*
	 * Open, so either is allowed — and the local one still wins when it is
	 * actually there.
	 *
	 * Asked rather than assumed: an image server that is installed but not
	 * started is the ordinary case, and silently billing somebody at a paid
	 * service because their own was not running would be the wrong surprise.
	 */
	if pictures.Reachable(s.b.Cfg.PicturesURL) {
		return local, nil
	}

	service, key := s.b.pictureService()

	if key == "" {
		return local, fmt.Errorf(
			"nothing can make a picture yet: the image server on this machine is not "+
				"running at %s, and no paid service has a key. Start one, or add a key "+
				"in Providers", s.b.Cfg.PicturesURL)
	}

	return pictures.Paid{
		ProviderName: service.ID,
		BaseURL:      service.BaseURL,
		APIKey:       key,
		Model:        s.b.Cfg.PictureModel,
	}, nil
}

/*
 * pictureService is which paid service makes pictures.
 *
 * Only the ones that actually have an images endpoint, which is far fewer than
 * the ones that answer chat. Offering the whole list would mean somebody with
 * a Groq key being told a picture was on its way and getting a 404 four
 * minutes later.
 */
func (b *Brain) pictureService() (llm.Service, string) {
	for _, id := range []string{"openai", "openrouter"} {
		key := b.Cfg.ProviderKeys[id]

		if key == "" {
			continue
		}

		for _, service := range llm.Services() {
			if service.ID != id {
				continue
			}

			// Through the router, so a service privacy forbids is not reached
			// by a side door that happens to draw rather than write.
			if !b.Mode.AllowsProvider(id) {
				continue
			}

			return service, key
		}
	}

	return llm.Service{}, ""
}

// PicturesFolder is where pictures and films go: inside the brain's own
// folder, so they travel with it rather than with the machine.
func (s studioOf) PicturesFolder() string {
	return filepath.Join(s.b.Root, "pictures")
}
