package llm

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Router chooses a provider and refuses the ones privacy forbids.
//
// Every path that obtains a provider goes through Provider(), and the guard
// lives inside it rather than at the call sites. That is the difference between
// a rule and a convention: a call site can forget to check, and one that
// forgets is indistinguishable from one that chose not to.
type Router struct {
	/*
	 * mode is behind a lock because it can change while the brain is running.
	 *
	 * Privacy used to be fixed when the program started, so changing it in the
	 * panel wrote the file, said "Saved", and did nothing until the next
	 * launch — while the panel went on reporting the old setting, which made a
	 * saved change look like a failed one. It takes effect now, which means it
	 * is read on one goroutine while being written on another.
	 */
	mu   sync.RWMutex
	mode Mode

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

func (r *Router) Mode() Mode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.mode
}

/*
 * UseMode changes what is allowed to leave this machine.
 *
 * The one setting where the direction of the change decides how careful this
 * has to be. Tightening has to take effect at once and completely, because
 * anything still running under the old rule is a disclosure somebody has just
 * said they did not want. Loosening can afford to be gradual and is not.
 *
 * Everything that obtains a provider goes through Provider(), so setting this
 * one field is the whole of it: there is no path that captured the old mode
 * and could go on using it.
 */
func (r *Router) UseMode(mode Mode) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.mode = mode
}

// Provider returns the named provider, or the default when name is empty.
func (r *Router) Provider(name string) (Provider, error) {
	if name == "" {
		name = r.fallback
	}

	if err := r.Mode().GuardProvider(name); err != nil {
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
	// Read once, so every provider in one listing is judged against the same
	// rule even if it changes while the list is being built.
	mode := r.Mode()

	out := make([]Availability, 0, len(r.providers))

	for name, p := range r.providers {
		permitted := mode.AllowsProvider(name)

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
