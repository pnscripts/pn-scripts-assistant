package redact

import (
	"strings"
	"testing"
)

func TestSecretsDoNotSurviveBeingKept(t *testing.T) {
	Add("correct-horse-battery-staple")

	for _, text := range []string{
		`curl -H "Authorization: Bearer abcdefghijklmnop1234" https://x`,
		"ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
		"git clone https://ghp_abcdefghijklmnopqrstuvwxyz0123@github.com/x",
		"Unity -accessToken gTe-CST88WW8RlJbBNBzhsfn9Q3X -projectPath /p",
		"password=correct-horse-battery-staple",
		"the mail password is correct-horse-battery-staple, keep it",
		`{"api_key": "AIzaSyA-abcdefghijklmnopqrstuvwxyz0123"}`,
	} {
		got := Text(text)

		for _, secret := range []string{"abcdefghijklmnop1234", "abcdefghijklmnopqrstuvwxyz", "gTe-CST88WW8",
			"correct-horse", "AIzaSyA"} {
			if strings.Contains(got, secret) {
				t.Errorf("%q kept %q: %q", text, secret, got)
			}
		}
	}

	plain := "go test ./... exited with status 0 after 12s in /home/p/project"
	if Text(plain) != plain {
		t.Errorf("ordinary text was changed: %q", Text(plain))
	}
}
