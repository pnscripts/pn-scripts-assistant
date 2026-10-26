package server

import (
	"errors"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/books"
	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Invoices page: the invoice register, a month's VAT with when it is
 * due, the registration threshold, and the rules they rest on with their
 * sources and the date they were checked. Information, never a filing:
 * nothing here sends anything anywhere.
 */

func (s *Server) handleBooks(w http.ResponseWriter, r *http.Request) {
	k := s.brain.Bookkeeping()

	list, err := k.Register(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"settings": k.Settings(), "pack": k.Pack(), "packs": books.Packs(), "invoices": list,
		"findings": books.RegisterFindings(list)})
}

func (s *Server) handleBooksSettings(w http.ResponseWriter, r *http.Request) {
	var body tools.BooksSettings

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if err := s.brain.Bookkeeping().SetSettings(body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"settings": s.brain.Bookkeeping().Settings(), "pack": s.brain.Bookkeeping().Pack()})
}

func (s *Server) handleBooksRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		books.Invoice

		Keep bool `json:"keep"`
	}

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	rec, err := s.brain.Bookkeeping().Record(body.Invoice, body.Keep)

	switch {
	case errors.Is(err, tools.ErrDuplicateInvoice):
		fail(w, http.StatusConflict, err.Error())
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		ok(w, rec)
	}
}

func (s *Server) handleBooksRemove(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.RemoveInvoice(id); errors.Is(err, store.ErrNoInvoice) {
		fail(w, http.StatusNotFound, "That invoice is not there.")

		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": id})
}

func (s *Server) handleBooksRead(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	var body struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	rec, err := s.brain.Bookkeeping().ReadInvoice(r.Context(), strings.TrimSpace(body.Path), body.Kind)

	switch {
	case errors.Is(err, brain.ErrNoModel), errors.Is(err, tools.ErrNoBooksModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to read the invoice; type its fields in instead.")
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		ok(w, rec)
	}
}

func (s *Server) handleBooksImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Sheet string `json:"sheet"`
		Kind  string `json:"kind"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if body.Kind != books.Sale && body.Kind != books.Purchase {
		fail(w, http.StatusBadRequest, "Say whether these are sales or purchases.")

		return
	}

	im, err := s.brain.Bookkeeping().Import(r.Context(), strings.TrimSpace(body.Path), body.Sheet, body.Kind)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, im)
}

func (s *Server) handleBooksSummary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Period string `json:"period"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	sum, err := s.brain.Bookkeeping().Summary(body.Period)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, sum)
}
