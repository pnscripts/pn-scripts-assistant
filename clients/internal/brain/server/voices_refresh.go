package server

import (
	"bufio"
	"context"
	"io"
	"strings"

	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/preflight"
)

// refreshTheVoices is preflight.RefreshVoices; a variable so a test can stand
// in for the download.
var refreshTheVoices = preflight.RefreshVoices

/*
 * refreshVoices fetches the voices this release installs for a copy whose
 * voices an older release installed.
 *
 * Run once when the server starts. 0.13's three voices were fine-tuned from
 * recordings licensed for research only and are no longer spoken with; the
 * person agreed to the voice's download when they installed it, and this
 * only replaces what that download brought. Without it they would hear the
 * system voice until they found the Update button beside Voice (speaking).
 *
 * Not while looking things up online is switched off: then that part says it
 * is out of date and offers Update, which asks first.
 */
func (s *Server) refreshVoices(ctx context.Context) {
	if !s.brain.Cfg.LookOnline {
		return
	}

	started := false

	// What the fetch says, said aside line by line, so a question answered
	// while it downloads keeps its step on screen.
	reader, writer := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)

		lines := bufio.NewScanner(reader)

		for lines.Scan() {
			if said := strings.TrimSpace(lines.Text()); said != "" {
				started = true

				progress.Aside("learning", said)
			}
		}
	}()

	replaced, err := refreshTheVoices(writer)

	writer.Close()
	<-done

	if started {
		defer progress.BackgroundDone()
	}

	if err != nil {
		s.log.Warn("could not fetch the new voices", "error", err)

		return
	}

	if !replaced {
		return
	}

	// Spoken with from the next sentence, not after a restart. The first one
	// starts the synthesiser itself, a second slower, as after any change of
	// voice.
	speech.ForgetPiper()

	s.log.Info("fetched the voices this version speaks with; the older ones are no longer used")
}
