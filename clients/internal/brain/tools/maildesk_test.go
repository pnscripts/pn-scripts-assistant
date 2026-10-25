package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/mail"
	"pn-scripts-assistant/internal/brain/store"
)

func deskFor(t *testing.T) (MailDesk, *store.DB) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return MailDesk{
		Account: func() mail.Account { return mail.Account{} },
		Local:   ReadMailbox{Home: fakeThunderbird(t)},
		Drafts:  db,
	}, db
}

func TestTheDigestSortsThunderbirdsInbox(t *testing.T) {
	desk, _ := deskFor(t)

	out, err := call(t, MailDigest{Desk: desk}, map[string]any{"hours": 24 * 3650})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"From Thunderbird on this computer: 2 messages", "From people (1)",
		"[local:ImapMail/imap.example.com/INBOX:3] Plugin Review — Проверка of your plugin",
		"Newsletters and notifications (1): Grid Operator"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	if strings.Contains(out, "deleted one") {
		t.Error("a deleted message was listed")
	}
}

/*
 * A reply drafted to a message takes its address and subject from it, sends
 * nothing, and waits; sending it needs the owner's yes every time and shows
 * the whole message.
 */
func TestAReplyIsDraftedAndWaitsForAYes(t *testing.T) {
	desk, db := deskFor(t)

	out, err := call(t, DraftReply{Desk: desk}, map[string]any{
		"answers": "local:ImapMail/imap.example.com/INBOX:3", "body": "Благодаря, ще прегледам.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Nothing has been sent") {
		t.Errorf("%s", out)
	}

	waiting, _ := db.Drafts(store.DraftWaiting, 0)
	if len(waiting) != 1 || waiting[0].To != "reviews@example.org" || waiting[0].Subject != "Re: Проверка of your plugin" ||
		!strings.Contains(waiting[0].Answers, "Plugin Review") {
		t.Fatalf("%+v", waiting)
	}

	send := SendDraft{Desk: desk}
	raw, _ := json.Marshal(map[string]any{"id": waiting[0].ID})

	if send.Risk() != Mutating || !strings.Contains(ConsentFor(send, raw), "in your name") {
		t.Error("sending a draft does not ask every time")
	}

	if summary := send.Summarize(raw); !strings.Contains(summary, "Send to reviews@example.org") ||
		!strings.Contains(summary, "Благодаря, ще прегледам.") {
		t.Errorf("the approval does not show the message: %s", summary)
	}

	if _, err := send.Execute(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Errorf("sending with no account: %v", err)
	}

	if still, _ := db.Draft(waiting[0].ID); still.Status != store.DraftWaiting {
		t.Errorf("a draft that could not be sent is %s", still.Status)
	}

	if _, err := call(t, DraftReply{Desk: desk}, map[string]any{"body": "Hi"}); err == nil {
		t.Error("a draft to nobody was kept")
	}

	if _, err := call(t, DraftReply{Desk: desk}, map[string]any{"answers": "local:nowhere:9", "body": "Hi"}); err == nil {
		t.Error("a reply to a message that is not there was kept")
	}
}

func TestWithNoMailboxTheDigestSaysSo(t *testing.T) {
	desk := MailDesk{Account: func() mail.Account { return mail.Account{} }, Local: ReadMailbox{Home: t.TempDir()}}

	if _, err := call(t, MailDigest{Desk: desk}, map[string]any{}); err != ErrNoMail {
		t.Errorf("%v", err)
	}
}
