package aidisclosure

import (
	"strings"
	"testing"
)

// The mail header follows the AI-Disclosure field of draft-abaris-aicdh:
// a structured dictionary whose mode says the AI wrote it and a person
// approved it.
func TestTheMailHeaderIsAStructuredAIDisclosure(t *testing.T) {
	if MailHeader != "AI-Disclosure" {
		t.Errorf("MailHeader = %q", MailHeader)
	}

	if !strings.HasPrefix(MailValue, "mode=ai-originated") {
		t.Errorf("MailValue = %q", MailValue)
	}

	if strings.ContainsAny(MailValue, "\r\n") {
		t.Errorf("MailValue breaks the header: %q", MailValue)
	}

	if !strings.Contains(MailValue, `provider="PN Scripts Assistant"`) {
		t.Errorf("MailValue does not name what wrote it: %q", MailValue)
	}
}

// Document properties say the same in words a person reads in File, Properties.
func TestTheDocumentMarkSaysItWasWrittenByAnAI(t *testing.T) {
	if !strings.Contains(DocumentNote, "AI") || !strings.Contains(DocumentNote, "PN Scripts Assistant") {
		t.Errorf("DocumentNote = %q", DocumentNote)
	}

	if DocumentKeywords != "AI-generated" {
		t.Errorf("DocumentKeywords = %q", DocumentKeywords)
	}

	for _, s := range []string{DocumentNote, DocumentKeywords, Producer} {
		if strings.ContainsAny(s, `()\<>&"`) {
			t.Errorf("%q needs escaping in PDF or XML; keep the marks plain", s)
		}
	}
}
