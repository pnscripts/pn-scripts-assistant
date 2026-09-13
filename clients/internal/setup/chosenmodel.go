package setup

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/config"
)

/*
 * Which model answers, written down.
 *
 * Setup could install four models and record none of them. The settings file
 * kept whatever it shipped with, so the running program fell back to choosing
 * for itself — which is a reasonable last resort and a poor answer to somebody
 * who has just deliberately installed a particular model and watched it
 * download.
 *
 * Two settings, and the difference between them matters. The name is written
 * whenever anything is installed, so the file is not a lie about what is
 * there. The flag that says a person chose it is written only when a person
 * actually did — the interface says "chosen by you" against it, and setting
 * that because setup happened to install one model would put a sentence on
 * screen that is not true.
 */

func (s *Server) handleChosenModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "I could not read that."})

		return
	}

	name := strings.TrimSpace(body.Model)
	if name == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Which model?"})

		return
	}

	if !modelsHere()[name] && !modelsHere()[name+":latest"] {
		s.writeJSON(w, map[string]any{"ok": false, "error": name + " is not installed."})

		return
	}

	if err := s.rememberModel(name, true); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

		return
	}

	s.writeJSON(w, map[string]any{"ok": true, "model": name})
}

/*
 * rememberModel records the model to answer with.
 *
 * deliberate says a person pressed a row rather than this being the only thing
 * installed. AUTO_MODEL is left alone either way: it is what lets small talk
 * go to a quicker model while real work goes to this one, and turning it off
 * is a mid-session pin rather than a starting point.
 */
func (s *Server) rememberModel(name string, deliberate bool) error {
	if err := s.writeSetting("OLLAMA_DEFAULT_MODEL", name, 0o600); err != nil {
		return err
	}

	if !deliberate {
		return nil
	}

	return s.writeSetting("OLLAMA_MODEL_CHOSEN", "1", 0o600)
}

/*
 * noteFirstModel records the first model installed, if nothing is recorded.
 *
 * The common path is one model, and asking somebody which of one thing should
 * answer is a question with no content. Written without the chosen flag, so
 * the interface still says the program picked it.
 */
func (s *Server) noteFirstModel(name string) {
	cfg, err := config.LoadFrom(s.envPath)
	if err == nil && strings.TrimSpace(cfg.OllamaModel) != "" && cfg.ModelChosen {
		return
	}

	_ = s.rememberModel(name, false)
}
