package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/pdfform"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The PDF forms page: a form's fields read out and laid out to fill in,
 * and the form filled into a new file beside it, every value read back
 * first. The original is never changed and nothing is written over.
 */

func formFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pdfform.ErrEncrypted), errors.Is(err, pdfform.ErrSigned):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, pdfform.ErrNoForm), errors.Is(err, pdfform.ErrNotPDF):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, tools.ErrFileThere):
		fail(w, http.StatusConflict, err.Error())
	default:
		fail(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) handleFormFields(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	fields, err := tools.FormFields(r.Context(), strings.TrimSpace(body.Path))
	if err != nil {
		formFailed(w, err)

		return
	}

	ok(w, map[string]any{"fields": fields, "out": tools.FilledName(strings.TrimSpace(body.Path))})
}

func (s *Server) handleFormFill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string            `json:"path"`
		Values map[string]string `json:"values"`
		Out    string            `json:"out"`
	}

	if !decodeUpTo(w, r, &body, 1<<18) {
		return
	}

	done, err := tools.FillForm(r.Context(), strings.TrimSpace(body.Path), body.Values, body.Out)
	if err != nil {
		if len(done.Problems) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "problems": done.Problems})

			return
		}

		formFailed(w, err)

		return
	}

	ok(w, done)
}
