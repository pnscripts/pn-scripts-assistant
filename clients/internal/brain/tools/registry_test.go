package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

type fake struct {
	name    string
	runtime bool
}

func (f fake) Name() string                     { return f.name }
func (f fake) Description() string              { return "a fake" }
func (f fake) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (f fake) Risk() Risk                       { return Safe }
func (f fake) Summarize(json.RawMessage) string { return "nothing" }

func (f fake) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }

type runtimeFake struct{ fake }

func (runtimeFake) AddedAtRunTime() {}

/*
 * A skill cannot take the name of something built in.
 *
 * The approval gate is written against tool names: what somebody reads in a
 * summary and what actually runs are tied together by the name alone. A skill
 * called run_command would not be a skill, it would be a redefinition of what
 * running a command means — and it would pass the gate once, as a piece of
 * text, and then never need to again.
 */
func TestSomethingAddedLaterCannotShadowSomethingBuiltIn(t *testing.T) {
	r := NewRegistry(fake{name: "run_command"})

	if err := r.Register(runtimeFake{fake{name: "run_command", runtime: true}}); err == nil {
		t.Fatal("a skill was allowed to take the name of a built-in tool")
	}

	got, _ := r.Get("run_command")

	if got.(fake).runtime {
		t.Error("the built-in tool was replaced anyway")
	}
}

// But one added at runtime may be replaced by another, or re-teaching a skill
// would fail the second time.
func TestOneAddedLaterCanBeReplaced(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(runtimeFake{fake{name: "invoicing"}}); err != nil {
		t.Fatal(err)
	}

	if err := r.Register(runtimeFake{fake{name: "invoicing", runtime: true}}); err != nil {
		t.Fatalf("re-teaching a skill failed: %v", err)
	}

	got, _ := r.Get("invoicing")

	if !got.(runtimeFake).runtime {
		t.Error("the second version was not the one kept")
	}
}

// The version changes whenever the list does, so anything caching a view of it
// — the agent caches the schemas — can tell its copy is stale.
func TestTheVersionChangesWithTheList(t *testing.T) {
	r := NewRegistry(fake{name: "read_file"})

	start := r.Version()

	if err := r.Register(runtimeFake{fake{name: "invoicing"}}); err != nil {
		t.Fatal(err)
	}

	added := r.Version()

	if added == start {
		t.Fatal("adding a tool did not change the version")
	}

	r.Unregister("invoicing")

	if r.Version() == added {
		t.Error("removing a tool did not change the version")
	}

	// Removing something that is not there changes nothing, so deleting a
	// skill twice does not make every cache rebuild.
	settled := r.Version()

	r.Unregister("invoicing")

	if r.Version() != settled {
		t.Error("removing nothing still counted as a change")
	}
}

/*
 * A skill can be taught while a task is reading the list.
 *
 * The turn that teaches one runs on its own goroutine and a background task
 * reads the registry on another. This was a plain map with no lock, from when
 * it was built at startup and never touched again.
 */
func TestTheRegistryCanBeReadWhileItIsChanged(t *testing.T) {
	r := NewRegistry(fake{name: "read_file"})

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		for i := 0; i < 200; i++ {
			r.Register(runtimeFake{fake{name: "invoicing"}})
			r.Unregister("invoicing")
		}
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < 200; i++ {
			r.All()
			r.Get("read_file")
			r.Version()
		}
	}()

	wg.Wait()
}
