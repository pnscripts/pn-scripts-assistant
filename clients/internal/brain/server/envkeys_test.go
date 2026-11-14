package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
)

/*
 * A key typed on the Models page is the key the providers are rebuilt
 * from, at once, for every company the page offers; and a key that is only
 * in the environment is not written down when the page saves something
 * else.
 */
func TestAKeyTypedOnTheModelsPageReachesTheProvidersAtOnce(t *testing.T) {
	for _, name := range []string{"anthropic", "openai", "openrouter"} {
		t.Run(name, func(t *testing.T) {
			for _, s := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY"} {
				t.Setenv(s, "")
			}

			ts, _, b := newServer(t)

			has := func() bool {
				for _, a := range b.Router.Availabilities(context.Background()) {
					if a.Name == name {
						return true
					}
				}

				return false
			}

			if has() {
				t.Fatalf("%s is there before any key", name)
			}

			send(t, ts.URL+"/api/providers/connect", `{"provider":"`+name+`","key":"zz-typed-key-`+name+`-12345678"}`)

			if !has() {
				t.Fatalf("a key typed for %s did not reach the providers", name)
			}

			send(t, ts.URL+"/api/providers/connect", `{"provider":"`+name+`","forget":true}`)

			if has() {
				t.Errorf("forgotten, %s is still offered", name)
			}
		})
	}
}

func TestSavingOnTheModelsPageDoesNotWriteTheEnvironmentsKeys(t *testing.T) {
	ts, _, b := newServer(t)

	t.Setenv("OPENAI_API_KEY", "zz-openai-from-the-shell-1")
	t.Setenv("OPENROUTER_API_KEY", "zz-openrouter-from-the-shell-2")
	t.Setenv("GROQ_API_KEY", "zz-groq-from-the-shell-3")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg, err := config.Load(b.Root)
	if err != nil {
		t.Fatal(err)
	}

	b.Cfg = cfg

	send(t, ts.URL+"/api/providers/connect", `{"provider":"openai","model":"gpt-zz"}`)
	send(t, ts.URL+"/api/providers/connect", `{"provider":"anthropic","key":"zz-anthropic-typed-4"}`)

	raw, _ := os.ReadFile(filepath.Join(b.Root, config.FileName))

	for _, env := range []string{"zz-openai-from-the-shell-1", "zz-openrouter-from-the-shell-2", "zz-groq-from-the-shell-3"} {
		if strings.Contains(string(raw), env) {
			t.Errorf("%s was written into the settings:\n%s", env, raw)
		}
	}

	if !strings.Contains(string(raw), "zz-anthropic-typed-4") || !strings.Contains(string(raw), "OPENAI_MODEL=gpt-zz") {
		t.Errorf("what was typed is not saved:\n%s", raw)
	}
}
