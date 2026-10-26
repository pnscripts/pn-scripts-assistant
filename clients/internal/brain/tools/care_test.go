package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/care"
	"pn-scripts-assistant/internal/brain/store"
)

func careFor(t *testing.T, reply string) (CustomerCare, *store.DB, *[]string) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	var asked []string

	return CustomerCare{
		Book: db,
		Desk: MailDesk{Drafts: db},
		Ask: func(_ context.Context, system, user string) (string, string, error) {
			asked = append(asked, system+"\n---\n"+user)

			return reply, "care-model", nil
		},
	}, db, &asked
}

func TestACustomerReplyIsWrittenFromTheBusinesssOwnWords(t *testing.T) {
	c, db, asked := careFor(t, "Здравейте, Иване,\n\nДа, доставяме до Пловдив за 2 работни дни, цената е 6 лв. [1]. "+
		"За съботна доставка: [[ASK: доставяме ли в събота]]")

	folder := t.TempDir()
	_ = os.WriteFile(filepath.Join(folder, "условия.txt"), []byte("Връщане\n\nСтоката може да се върне в 14 дни."), 0o600)

	if err := c.SetProfile(care.Profile{Name: "Зелен магазин", Signature: "Мария, Зелен магазин", Tone: "friendly", Folder: folder}); err != nil {
		t.Fatal(err)
	}

	if err := c.SetProfile(care.Profile{Tone: "rude"}); err == nil {
		t.Error("an unknown tone was kept")
	}

	if err := c.SetProfile(care.Profile{Folder: "relative/folder"}); err == nil {
		t.Error("a relative folder was kept")
	}

	_, _ = db.AddCareAnswer("Доставяте ли до Пловдив?", "Да, за 2 работни дни, цената е 6 лв.")

	w, err := c.Reply(context.Background(), "Здравейте, доставяте ли до Пловдив и в събота?", "")
	if err != nil {
		t.Fatal(err)
	}

	if w.Lang != "bg" || w.Kind.Name != "question" || len(w.Gaps) != 1 || !strings.HasSuffix(w.Text, "Мария, Зелен магазин") ||
		strings.Contains(w.Text, "[1]") || len(w.BasedOn) != 1 || w.Model != "care-model" {
		t.Errorf("%+v", w)
	}

	if !strings.Contains((*asked)[0], "Зелен магазин") || !strings.Contains((*asked)[0], "Bulgarian") ||
		!strings.Contains((*asked)[0], "6 лв") {
		t.Errorf("the writer was not given the business's words: %s", (*asked)[0])
	}

	// Kept as a draft, it waits — and with a gap in it, it cannot be sent.
	out, err := CustomerReply{With: c}.Execute(context.Background(),
		json.RawMessage(`{"message":"Доставяте ли до Пловдив в събота?","to":"ivan@example.com"}`))
	if err != nil || !strings.Contains(out, "Kept as draft") || !strings.Contains(out, "The owner must answer") {
		t.Fatalf("%v %s", err, out)
	}

	drafts, _ := db.Drafts(store.DraftWaiting, 10)
	if len(drafts) != 1 || drafts[0].To != "ivan@example.com" {
		t.Fatalf("%+v", drafts)
	}

	if _, err := SendWaitingDraft(MailDesk{Drafts: db, Account: nil}, drafts[0].ID); err == nil {
		t.Error("a draft was let through")
	}
}

func TestADraftWithAGapCannotBeSent(t *testing.T) {
	_, db, _ := careFor(t, "")

	dr, _ := db.AddDraft("a@example.com", "Re: x", "Hello, [[ASK: the price]]", "", "")

	desk := MailDesk{Drafts: db}
	desk.Account = nil

	if _, err := SendWaitingDraft(desk, dr.ID); err == nil || !strings.Contains(err.Error(), "[[") {
		t.Fatalf("the gap was not the reason: %v", err)
	}

	if back, _ := db.Draft(dr.ID); back.Status != store.DraftWaiting {
		t.Errorf("the draft stopped waiting: %+v", back)
	}
}

func TestTheAnswerBookTools(t *testing.T) {
	c, _, _ := careFor(t, "")
	ctx := context.Background()

	change := ChangeBusinessAnswers{With: c}

	if out, err := change.Execute(ctx, json.RawMessage(`{"action":"add","question":"Opening hours?","answer":"9 to 6, Monday to Friday."}`)); err != nil || !strings.Contains(out, "Added") {
		t.Fatalf("%v %s", err, out)
	}

	_, _ = change.Execute(ctx, json.RawMessage(`{"action":"add","question":"Do you deliver?","answer":"Yes, in 2 days."}`))

	out, _ := BusinessAnswers{With: c}.Execute(ctx, json.RawMessage(`{"query":"when are you open, hours"}`))
	if !strings.Contains(out, "Opening hours?") || strings.Contains(out, "deliver") {
		t.Errorf("%s", out)
	}

	if _, err := change.Execute(ctx, json.RawMessage(`{"action":"remove","id":99}`)); !errors.Is(err, store.ErrNoCareAnswer) {
		t.Errorf("%v", err)
	}

	if _, err := change.Execute(ctx, json.RawMessage(`{"action":"burn"}`)); err == nil {
		t.Error("an unknown action was accepted")
	}

	if _, err := (CustomerCare{Book: c.Book}).Reply(ctx, "hi", ""); !errors.Is(err, ErrNoCareModel) {
		t.Errorf("%v", err)
	}
}
