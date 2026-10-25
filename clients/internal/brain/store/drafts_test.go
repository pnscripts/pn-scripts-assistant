package store

import (
	"errors"
	"testing"
)

/*
 * A draft waits until it is sent or discarded, can be changed while it
 * waits, and can be sent only once however many times somebody presses.
 */
func TestADraftIsSentOnceAndOnlyOnce(t *testing.T) {
	db := openDeeds(t)

	if _, err := db.AddDraft(" ", "s", "b", "", ""); err == nil {
		t.Error("a draft to nobody was kept")
	}

	dr, err := db.AddDraft("anna@example.com", "Re: Offer", "Thank you.", "Anna <anna@example.com> — Offer", "")
	if err != nil {
		t.Fatal(err)
	}

	if dr, err = db.ChangeDraft(dr.ID, "anna@example.com", "Re: Offer", "Thank you, Anna."); err != nil || dr.Body != "Thank you, Anna." {
		t.Fatal(dr, err)
	}

	waiting, _ := db.Drafts(DraftWaiting, 0)
	if len(waiting) != 1 || waiting[0].Answers != "Anna <anna@example.com> — Offer" {
		t.Fatalf("%+v", waiting)
	}

	claimed, err := db.ClaimDraft(dr.ID)
	if err != nil || claimed.Body != "Thank you, Anna." {
		t.Fatal(claimed, err)
	}

	if _, err := db.ClaimDraft(dr.ID); !errors.Is(err, ErrNoDraft) {
		t.Error("a draft being sent was claimed a second time")
	}

	if err := db.SentDraft(dr.ID, "message id <x@y>"); err != nil {
		t.Fatal(err)
	}

	sent, _ := db.Draft(dr.ID)
	if sent.Status != DraftSent || sent.SentAt == nil {
		t.Errorf("%+v", sent)
	}

	if _, err := db.ChangeDraft(dr.ID, "x@example.com", "", ""); !errors.Is(err, ErrNoDraft) {
		t.Error("a sent draft was changed")
	}

	if err := db.SetDraftStatus(dr.ID, DraftDiscarded, ""); !errors.Is(err, ErrNoDraft) {
		t.Error("a sent draft was discarded")
	}
}

func TestAFailedDraftCanBeTriedAgainOrDiscarded(t *testing.T) {
	db := openDeeds(t)

	dr, _ := db.AddDraft("bob@example.com", "Hello", "Hi", "", "suspicious: reply goes elsewhere")

	if _, err := db.ClaimDraft(dr.ID); err != nil {
		t.Fatal(err)
	}

	_ = db.FailedDraft(dr.ID, "the server refused the password")

	failed, _ := db.Draft(dr.ID)
	if failed.Status != DraftFailed || failed.Note != "the server refused the password" || failed.Warning == "" {
		t.Fatalf("%+v", failed)
	}

	if err := db.SetDraftStatus(dr.ID, DraftDiscarded, ""); err != nil {
		t.Fatal(err)
	}

	if all, _ := db.Drafts("", 0); len(all) != 1 || all[0].Status != DraftDiscarded {
		t.Errorf("%+v", all)
	}
}
