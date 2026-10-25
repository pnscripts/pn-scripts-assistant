package maildesk

import (
	"strings"
	"testing"
	"time"
)

func TestNewMailIsSortedByWhatItNeeds(t *testing.T) {
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

	items := []Item{
		{Ref: "1", From: "Anna Petrova <anna@example.com>", Subject: "Можете ли да изпратите офертата?", Date: "Mon, 26 Oct 2026 08:00:00 +0000"},
		{Ref: "2", From: "Bob <bob@example.org>", Subject: "Photos from the weekend", Date: "Mon, 26 Oct 2026 07:00:00 +0000", Seen: true},
		{Ref: "3", From: "Shop <news@shop.example>", Subject: "Autumn sale", Date: "Mon, 26 Oct 2026 06:00:00 +0000", Bulk: true},
		{Ref: "4", From: "GitHub <noreply@github.com>", Subject: "Build passed", Date: "Mon, 26 Oct 2026 05:00:00 +0000"},
		{Ref: "5", From: "Bank Support <support@bank-secure-login.example>", ReplyTo: "x@other.example", Subject: "Urgent: verify your account password", Date: "Mon, 26 Oct 2026 04:00:00 +0000"},
		{Ref: "6", From: "Old <old@example.com>", Subject: "Last week", Date: "Mon, 19 Oct 2026 08:00:00 +0000"},
		{Ref: "7", From: "Carol <carol@example.com>", Subject: "Re: plans", Date: "not a date"},
	}

	d := Sort(items, now.Add(-24*time.Hour), nil)

	refs := func(list []Item) string {
		var out []string
		for _, it := range list {
			out = append(out, it.Ref)
		}

		return strings.Join(out, ",")
	}

	if d.Total != 6 || d.Unseen != 5 {
		t.Errorf("total %d unseen %d", d.Total, d.Unseen)
	}

	if got := refs(d.Suspicious); got != "5" {
		t.Errorf("suspicious %s", got)
	}

	if got := refs(d.Reply); got != "1,7" {
		t.Errorf("needs a reply %s", got)
	}

	if got := refs(d.People); got != "2" {
		t.Errorf("from people %s", got)
	}

	if got := refs(d.Bulk); got != "3,4" {
		t.Errorf("bulk %s", got)
	}

	for _, it := range append(append(d.Suspicious, d.Reply...), d.Bulk...) {
		if it.Why == "" || it.WhyBG == "" {
			t.Errorf("%s has no reason in both languages", it.Ref)
		}
	}

	if d.Suspicious[0].Scam == "" {
		t.Error("the scam verdict was not kept")
	}
}

func TestAddressesAndSubjects(t *testing.T) {
	for in, want := range map[string]string{"Offer": "Re: Offer", "Re: Offer": "Re: Offer", "ОТГ: Оферта": "ОТГ: Оферта", "Fwd: x": "Re: Fwd: x"} {
		if got := ReplySubject(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}

	if Address("Anna <anna@example.com>") != "anna@example.com" || Name("Anna <anna@example.com>") != "Anna" {
		t.Error("address or name")
	}

	if got := strings.Join(Senders([]Item{{From: "A <a@x>"}, {From: "B <b@x>"}, {From: "A <a@x>"}}, 5), "|"); got != "A (2)|B" {
		t.Errorf("senders %s", got)
	}

	for _, auto := range []string{"no-reply@x.com", "GitHub <notifications@github.com>", "news@shop.example", "Mailer-Daemon@x"} {
		if !Automatic(auto) {
			t.Errorf("%s is not seen as automatic", auto)
		}
	}

	if Automatic("Anna Newsome <anna@example.com>") {
		t.Error("a person was taken for a machine")
	}
}
