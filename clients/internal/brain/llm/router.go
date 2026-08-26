package llm

import (
	"context"
	"fmt"
	"sort"
)

// Router chooses a provider and refuses the ones privacy forbids.
//
// Every path that obtains a provider goes through Provider(), and the guard
// lives inside it rather than at the call sites. That is the difference between
// a rule and a convention: a call site can forget to check, and one that
// forgets is indistinguishable from one that chose not to.
type Router struct {
	mode      Mode
	providers map[string]Provider
	fallback  string
	embedder  Embedder
}

// NewRouter builds a router. The local provider is always registered; anything
// else is registered only when configured.
func NewRouter(mode Mode, defaultProvider string, providers ...Provider) *Router {
	r := &Router{
		mode:      mode,
		providers: map[string]Provider{},
		fallback:  defaultProvider,
	}

	for _, p := range providers {
		if p == nil {
			continue
		}

		r.providers[p.Name()] = p

		if e, ok := p.(Embedder); ok && r.embedder == nil {
			r.embedder = e
		}
	}

	if r.fallback == "" {
		r.fallback = Local
	}

	return r
}

func (r *Router) Mode() Mode { return r.mode }

// Provider returns the named provider, or the default when name is empty.
func (r *Router) Provider(name string) (Provider, error) {
	if name == "" {
		name = r.fallback
	}

	if err := r.mode.GuardProvider(name); err != nil {
		return nil, err
	}

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("no provider named %q is configured", name)
	}

	return p, nil
}

// Embedder returns the local embedder.
//
// There is deliberately no way to select a remote one. Embedding is applied to
// the contents of a person's disk, so it stays on the machine in every mode.
func (r *Router) Embedder() (Embedder, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("no local embedding model is configured")
	}

	return r.embedder, nil
}

// Available lists the providers that privacy permits and that are reachable.
type Availability struct {
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	Permitted bool   `json:"permitted"`
	Default   bool   `json:"default"`
}

// Availabilities describes every registered provider for the interface,
// including the ones privacy forbids — showing a provider as forbidden is more
// honest than hiding it and letting the user wonder where it went.
func (r *Router) Availabilities(ctx context.Context) []Availability {
	out := make([]Availability, 0, len(r.providers))

	for name, p := range r.providers {
		permitted := r.mode.AllowsProvider(name)

		out = append(out, Availability{
			Name:      name,
			Permitted: permitted,
			// A provider privacy forbids is not probed. Reaching out to check
			// on something that may not be used is the leak in miniature.
			Reachable: permitted && p.Available(ctx),
			Default:   name == r.fallback,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}
