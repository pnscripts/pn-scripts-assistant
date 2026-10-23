package scam

import (
	"strings"
	"testing"
)

// fakeDNS answers from a list, so no test asks the real network.
type fakeDNS map[string]bool

func (f fakeDNS) Resolves(d string) (bool, bool) {
	v, ok := f[d]

	return v, ok
}

func TestAPhishThatImitatesABrandIsAScam(t *testing.T) {
	v := Check(Message{
		From:    "PayPal Security <service@paypa1-secure.com>",
		Subject: "Your account will be suspended",
		Body:    "Verify your account within 24 hours: https://paypa1-secure.com/login",
	}, nil)

	if v.Level != LikelyScam {
		t.Fatalf("%+v", v)
	}

	if !strings.Contains(v.String(), "imitates paypal") {
		t.Errorf("does not say why: %s", v)
	}
}

func TestASenderDomainThatDoesNotExist(t *testing.T) {
	look := fakeDNS{"nowhere-invoices.biz": false}

	v := Check(Message{From: "billing@nowhere-invoices.biz", Subject: "Invoice", Body: "See attached."}, look)
	if v.Level != LikelyScam || !strings.Contains(v.String(), "does not exist") {
		t.Fatalf("%+v", v)
	}
}

func TestOfflineIsNotNonexistent(t *testing.T) {
	// No answer at all — no network — must never read as "does not exist".
	v := Check(Message{From: "a@example.org", Subject: "Hello", Body: "Lunch on Friday?"}, fakeDNS{})
	if v.Warns() {
		t.Fatalf("%+v", v)
	}
}

func TestUrgencyAndACredentialAskFromAStranger(t *testing.T) {
	v := Check(Message{
		From:    "Support Team <helpdesk2024@gmail.com>",
		ReplyTo: "claims@other-desk.net",
		Subject: "URGENT: confirm your password",
		Body:    "Act now or your mailbox is locked. Send us your password and the one-time code.",
	}, nil)

	if v.Level != LikelyScam {
		t.Fatalf("%+v", v)
	}

	for _, want := range []string{"replies go to", "free mail", "password", "pressure"} {
		if !strings.Contains(v.String(), want) {
			t.Errorf("missing %q in %s", want, v)
		}
	}
}

func TestLinksThatHideWhereTheyGo(t *testing.T) {
	for _, body := range []string{
		"Open https://xn--pypal-4ve.com/ now",
		"Open http://192.168.4.20/login",
		"Open https://www.paypal.com@evil.example/login",
		"Open https://apple.com.account-check.io/",
	} {
		v := Check(Message{From: "x@example.org", Subject: "Hi", Body: body}, nil)
		if !v.Warns() {
			t.Errorf("%q was not flagged", body)
		}
	}
}

func TestOrdinaryMailIsLeftAlone(t *testing.T) {
	for _, m := range []Message{
		{From: "GitHub <noreply@github.com>", Subject: "[GitHub] Please reset your password",
			Body: "This link expires in 24 hours: https://github.com/password_reset/abc"},
		{From: "Amazon.de <versand@amazon.de>", Subject: "Ihre Bestellung", Body: "https://www.amazon.de/orders"},
		{From: "Maria <maria@example.org>", Subject: "Photos from Sunday", Body: "Here they are: https://example.org/album"},
		{From: "newsletter@shop.example.co.uk", Subject: "Pop-ups and new arrivals", Body: "https://shop.example.co.uk/new"},
	} {
		if v := Check(m, fakeDNS{"github.com": true, "amazon.de": true, "example.org": true, "example.co.uk": true}); v.Warns() {
			t.Errorf("%s was flagged: %s", m.From, v)
		}
	}
}

func TestWordsAloneOnlyMakeItSuspicious(t *testing.T) {
	v := Check(Message{From: "friend@example.org", Subject: "urgent", Body: "Please send the bank details for the wire transfer today, it is urgent."}, nil)
	if v.Level != Suspicious {
		t.Fatalf("%+v", v)
	}
}

func TestRegistrable(t *testing.T) {
	for host, want := range map[string]string{
		"mail.example.co.uk": "example.co.uk",
		"a.b.paypal.com":     "paypal.com",
		"localhost":          "localhost",
	} {
		if got := Registrable(host); got != want {
			t.Errorf("%s → %s", host, got)
		}
	}
}
