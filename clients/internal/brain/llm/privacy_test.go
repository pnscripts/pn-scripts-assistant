package llm

import (
	"context"
	"strings"
	"testing"
)

// The single most important rule in the system: what the brain has learned
// never goes to a third party, in any mode. If this test ever fails, the
// program is leaking a dossier.
func TestMemoryNeverLeavesForThirdParties(t *testing.T) {
	for _, mode := range []Mode{ModePrivate, ModeResearch, ModeOpen} {
		for _, provider := range []string{"anthropic", "openai", "gemini", "anything-else"} {
			if AllowsMemoryFor(provider) {
				t.Errorf("mode %q: memory was allowed to reach %q", mode, provider)
			}
		}

		if !AllowsMemoryFor(Local) {
			t.Errorf("mode %q: memory was withheld from the local model", mode)
		}
	}
}

func TestParseModeFallsBackToStrictest(t *testing.T) {
	cases := map[string]Mode{
		"private":  ModePrivate,
		"research": ModeResearch,
		"open":     ModeOpen,
		"OPEN":     ModeOpen,
		"Research": ModeResearch,
		// A typo in configuration must not silently open the machine up.
		"opn":     ModePrivate,
		"":        ModePrivate,
		"public":  ModePrivate,
		"disable": ModePrivate,
	}

	for in, want := range cases {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOnlyOpenModeAllowsThirdPartyProviders(t *testing.T) {
	cases := []struct {
		mode     Mode
		provider string
		want     bool
	}{
		{ModePrivate, Local, true},
		{ModePrivate, "anthropic", false},
		{ModeResearch, Local, true},
		{ModeResearch, "anthropic", false},
		{ModeOpen, Local, true},
		{ModeOpen, "anthropic", true},
	}

	for _, c := range cases {
		if got := c.mode.AllowsProvider(c.provider); got != c.want {
			t.Errorf("%q.AllowsProvider(%q) = %v, want %v", c.mode, c.provider, got, c.want)
		}
	}
}

func TestGuardExplainsHowToChangeIt(t *testing.T) {
	err := ModePrivate.GuardProvider("anthropic")
	if err == nil {
		t.Fatal("private mode permitted a third-party provider")
	}

	// A refusal a user cannot act on is a dead end, so the message has to name
	// the setting.
	if !strings.Contains(err.Error(), "BRAIN_PRIVACY=open") {
		t.Errorf("refusal does not say how to change it: %v", err)
	}
}

func TestWebOnlyOutsidePrivate(t *testing.T) {
	if ModePrivate.AllowsWeb() {
		t.Error("private mode allowed web access")
	}

	if !ModeResearch.AllowsWeb() || !ModeOpen.AllowsWeb() {
		t.Error("research or open mode blocked web access")
	}
}

// A fake provider so the router can be tested without a model.
type fakeProvider struct {
	name      string
	reachable bool
}

func (f fakeProvider) Name() string                   { return f.name }
func (f fakeProvider) Available(context.Context) bool { return f.reachable }
func (f fakeProvider) Chat(context.Context, Request) (Response, error) {
	return Response{Content: "hello", Provider: f.name}, nil
}

func TestRouterRefusesForbiddenProviderEvenWhenAskedDirectly(t *testing.T) {
	r := NewRouter(ModePrivate, Local,
		fakeProvider{name: Local, reachable: true},
		fakeProvider{name: "anthropic", reachable: true},
	)

	if _, err := r.Provider("anthropic"); err == nil {
		t.Fatal("router handed out a provider that privacy forbids")
	}

	if _, err := r.Provider(Local); err != nil {
		t.Fatalf("router refused the local provider: %v", err)
	}
}

func TestRouterAllowsThirdPartyWhenOpen(t *testing.T) {
	r := NewRouter(ModeOpen, Local,
		fakeProvider{name: Local, reachable: true},
		fakeProvider{name: "anthropic", reachable: true},
	)

	p, err := r.Provider("anthropic")
	if err != nil {
		t.Fatalf("open mode refused a third-party provider: %v", err)
	}

	if p.Name() != "anthropic" {
		t.Errorf("got provider %q", p.Name())
	}
}

func TestRouterDefaultsToConfiguredProvider(t *testing.T) {
	r := NewRouter(ModePrivate, Local, fakeProvider{name: Local, reachable: true})

	p, err := r.Provider("")
	if err != nil {
		t.Fatalf("empty name did not resolve to the default: %v", err)
	}

	if p.Name() != Local {
		t.Errorf("default resolved to %q, want %q", p.Name(), Local)
	}
}

// A provider privacy forbids must not be probed over the network: reaching out
// to check on something that may not be used is the leak in miniature.
func TestForbiddenProvidersAreNotProbed(t *testing.T) {
	r := NewRouter(ModePrivate, Local,
		fakeProvider{name: Local, reachable: true},
		fakeProvider{name: "anthropic", reachable: true},
	)

	for _, a := range r.Availabilities(context.Background()) {
		if a.Name != "anthropic" {
			continue
		}

		if a.Permitted {
			t.Error("anthropic reported as permitted in private mode")
		}

		if a.Reachable {
			t.Error("a forbidden provider was probed and reported reachable")
		}
	}
}

func TestDescribeNamesTheMode(t *testing.T) {
	for _, m := range []Mode{ModePrivate, ModeResearch, ModeOpen} {
		d := m.Describe()

		if d.Mode != m {
			t.Errorf("Describe() for %q reported mode %q", m, d.Mode)
		}

		if d.Summary == "" || d.Detail == "" {
			t.Errorf("Describe() for %q is missing text: %+v", m, d)
		}
	}
}
