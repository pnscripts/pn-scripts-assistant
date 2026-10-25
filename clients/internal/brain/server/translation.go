package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/glossary"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/translate"
)

/*
 * The Translation page: translate a text, translate a file into a new one
 * beside it, and keep the glossary — the owner's agreed translations, which
 * every translation uses and is checked against.
 *
 * Translating asks the model through the brain, where privacy is enforced.
 * Writing a file is at the desk only (guard.go), and never replaces one.
 */

type glossaryBody struct {
	Source string `json:"source"`
	Target string `json:"target"`
	From   string `json:"from"`
	To     string `json:"to"`
	Note   string `json:"note"`
	CSV    string `json:"csv"`
}

type languageView struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	NameBG string `json:"name_bg"`
}

func languages() []languageView {
	out := make([]languageView, 0, len(translate.Languages))

	for _, l := range translate.Languages {
		out = append(out, languageView{Code: l.Code, Name: l.Name, NameBG: l.NameBG})
	}

	return out
}

func (s *Server) handleGlossary(w http.ResponseWriter, r *http.Request) {
	terms, err := glossary.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"terms": terms, "languages": languages(), "formats": translate.Formats})
}

func (s *Server) handleGlossaryCSV(w http.ResponseWriter, r *http.Request) {
	terms, err := glossary.Load(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="glossary.csv"`)

	// A byte-order mark, so a spreadsheet opens the Cyrillic as Cyrillic.
	_, _ = w.Write([]byte("\ufeff" + glossary.CSV(terms)))
}

func (s *Server) handleGlossaryAdd(w http.ResponseWriter, r *http.Request) {
	var body glossaryBody

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	term, replaced, err := glossary.Add(s.brain.Root, glossary.Term{
		Source: body.Source, Target: body.Target, From: body.From, To: body.To, Note: body.Note,
	})
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"term": term, "replaced": replaced})
}

func (s *Server) handleGlossaryRemove(w http.ResponseWriter, r *http.Request) {
	var body glossaryBody

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	found, err := glossary.Remove(s.brain.Root, glossary.Term{Source: body.Source, From: body.From, To: body.To})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if !found {
		fail(w, http.StatusNotFound, "That term is not in the glossary.")

		return
	}

	ok(w, map[string]any{"removed": true})
}

// handleGlossaryImport adds every line of a pasted or saved spreadsheet.
func (s *Server) handleGlossaryImport(w http.ResponseWriter, r *http.Request) {
	var body glossaryBody

	if !decodeUpTo(w, r, &body, 2<<20) {
		return
	}

	terms, problems := glossary.FromCSV(strings.TrimPrefix(body.CSV, "\ufeff"), body.From, body.To)

	added := 0

	for _, t := range terms {
		if _, _, err := glossary.Add(s.brain.Root, t); err != nil {
			problems = append(problems, t.Source+": "+err.Error())

			continue
		}

		added++
	}

	if problems == nil {
		problems = []string{}
	}

	ok(w, map[string]any{"added": added, "problems": problems})
}

type translateBody struct {
	Text string `json:"text"`
	Path string `json:"path"`
	Out  string `json:"out"`
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	var body translateBody

	if !decodeUpTo(w, r, &body, 4*mostText) {
		return
	}

	if strings.TrimSpace(body.Text) == "" {
		fail(w, http.StatusBadRequest, "There is no text to translate.")

		return
	}

	if utf8.RuneCountInString(body.Text) > mostText {
		fail(w, http.StatusRequestEntityTooLarge, "That text is too long to translate here; save it as a file and translate the file.")

		return
	}

	res, err := s.brain.Translator().Text(r.Context(), body.Text, strings.ToLower(body.From), strings.ToLower(body.To))
	translated(w, res, err)
}

func (s *Server) handleTranslateFile(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	var body translateBody

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	path := strings.TrimSpace(body.Path)

	if !filepath.IsAbs(path) || (body.Out != "" && !filepath.IsAbs(body.Out)) {
		fail(w, http.StatusBadRequest, "Give the full path of the file, starting from the top of the disk.")

		return
	}

	if err := tools.GuardSensitive(r.Context(), path); err != nil {
		fail(w, http.StatusForbidden, err.Error())

		return
	}

	if _, err := os.Stat(path); err != nil {
		fail(w, http.StatusNotFound, "There is no file at that path.")

		return
	}

	res, err := s.brain.Translator().File(r.Context(), path, body.Out, strings.ToLower(body.From), strings.ToLower(body.To))
	translated(w, res, err)
}

func translated(w http.ResponseWriter, res translate.Result, err error) {
	switch {
	case errors.Is(err, translate.ErrNoEngine) || errors.Is(err, brain.ErrNoModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to translate with.")
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		ok(w, res)
	}
}
