package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * Who it is, asked while somebody is still here to answer.
 *
 * Setup used to write five things — the provider keys, the voice, the protect
 * rules and where the brain lives — and none of them said anything about the
 * assistant itself. Everybody who finished ended up with one called
 * "Assistant", answering to "Assistant", with no spoken language set.
 *
 * The last of those is not cosmetic. With no language, whisper is left to
 * guess, and its guess for a language it is unsure of is to translate rather
 * than transcribe — so somebody speaking Bulgarian is answered in English
 * about something they did not say. config.go carries a warning about exactly
 * this. It was a warning about a setting nothing ever set.
 */

// Language is one option for what somebody speaks.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

/*
 * spokenLanguages is what whisper is told to expect.
 *
 * The same eight the running program offers, named in their own language
 * because somebody choosing "Български" is doing so precisely because they do
 * not want to read English. The empty code means "let it guess", which is the
 * old behaviour and is kept as a choice rather than as a default nobody made.
 */
func spokenLanguages() []Language {
	return []Language{
		{Code: "", Name: "Let it guess"},
		{Code: "bg", Name: "Български"},
		{Code: "en", Name: "English"},
		{Code: "de", Name: "Deutsch"},
		{Code: "fr", Name: "Français"},
		{Code: "es", Name: "Español"},
		{Code: "it", Name: "Italiano"},
		{Code: "ru", Name: "Русский"},
		{Code: "tr", Name: "Türkçe"},
	}
}

func (s *Server) identity() map[string]any {
	cfg, _ := config.LoadFrom(s.envPath)

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = config.DefaultName
	}

	return map[string]any{
		"name":         name,
		"default_name": config.DefaultName,
		"wake_word":    cfg.WakeWord,
		"language":     cfg.Language,
		"languages":    spokenLanguages(),
	}
}

/*
 * saveIdentity records a name and a language.
 *
 * Pointers, so a field that was not sent is left alone rather than cleared —
 * the same shape /protection uses, and for the same reason: the page saves one
 * thing at a time and must not blank the others on the way past.
 */
func (s *Server) saveIdentity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     *string `json:"name"`
		Language *string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "I could not read that."})

		return
	}

	if body.Name != nil {
		if err := s.saveName(*body.Name); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}
	}

	if body.Language != nil {
		if err := s.saveLanguage(*body.Language); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}
	}

	s.writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) saveName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("give it a name, or leave %s", config.DefaultName)
	}

	// Runes, not bytes. A name in Cyrillic is two bytes a letter, and a limit
	// counted in bytes would refuse half the length it promised.
	if utf8.RuneCountInString(name) > 40 {
		return fmt.Errorf("that is a long name — keep it under 40 letters")
	}

	cfg, _ := config.LoadFrom(s.envPath)

	if err := s.writeSetting("BRAIN_NAME", name, 0o600); err != nil {
		return err
	}

	/*
	 * Naming it also gives it something to answer to — but only while nothing
	 * has been tuned.
	 *
	 * The rule is copied from the running program's own settings handler
	 * rather than invented here, so the two cannot disagree about what
	 * "untouched" means. Somebody who has deliberately set a different wake
	 * word keeps it.
	 */
	untouched := strings.TrimSpace(cfg.WakeWord) == "" ||
		strings.EqualFold(cfg.WakeWord, cfg.Name) ||
		strings.EqualFold(cfg.WakeWord, config.DefaultWakeWord)

	if untouched {
		return s.writeSetting("BRAIN_WAKE_WORD", name, 0o600)
	}

	return nil
}

func (s *Server) saveLanguage(code string) error {
	code = strings.TrimSpace(code)

	known := false

	for _, l := range spokenLanguages() {
		if l.Code == code {
			known = true

			break
		}
	}

	if !known {
		return fmt.Errorf("I do not know that language")
	}

	// In this process as well as in the file: the voice sample on this very
	// step picks a voice for the language, and it should follow the answer
	// straight away rather than after a restart.
	speech.SetLanguage(code)

	return s.writeSetting("BRAIN_LANGUAGE", code, 0o600)
}
