package team

import (
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

func installed() llm.Sizes {
	return llm.Sizes{Work: "qwen3:8b", Quick: "llama3.2:3b", Best: "qwen3:14b"}
}

/*
 * An agent on a hosted service is not sent a model from this machine.
 *
 * The bug this exists to stop, and it was silent: the size roles resolve to
 * what is installed here, so an agent with provider: anthropic and uses: best
 * was sending "qwen3:14b" to Anthropic as the model name. Nothing checked.
 * Empty is the right answer — the service then uses its own default, which is
 * the only model this program can be certain it has.
 */
func TestAHostedAgentIsNotSentAModelFromThisMachine(t *testing.T) {
	best := Agent{Name: "writer", Uses: UsesBest, Provider: "anthropic"}

	if got := best.ModelOn("anthropic", installed()); got != "" {
		t.Errorf("Anthropic was told to use %q", got)
	}

	// And locally the roles work exactly as they did.
	if got := best.ModelOn(llm.Local, installed()); got != "qwen3:14b" {
		t.Errorf("locally it chose %q", got)
	}
}

// Unless somebody named a model, in which case they have decided and it is
// used wherever the agent is asked.
func TestANamedModelIsUsedWhereverItIsAsked(t *testing.T) {
	pinned := Agent{
		Name: "writer", Uses: UsesBest, Provider: "anthropic",
		Prefers: []string{"claude-sonnet-4-5"},
	}

	for _, through := range []string{"anthropic", llm.Local, ""} {
		if got := pinned.ModelOn(through, installed()); got != "claude-sonnet-4-5" {
			t.Errorf("through %q it chose %q", through, got)
		}
	}
}

/*
 * A provider this program does not recognise is treated as local.
 *
 * The safe way round. The worst case is offering something a model name it
 * does not have; the other way round would be silently sending work to a
 * company because its name was not in a list.
 */
func TestAnUnknownProviderIsTreatedAsLocal(t *testing.T) {
	quick := Agent{Name: "researcher", Uses: UsesQuick}

	if got := quick.ModelOn("something-on-this-machine", installed()); got != "llama3.2:3b" {
		t.Errorf("it chose %q", got)
	}
}

/*
 * An agent that must stay here stays here, whatever the task was started with.
 *
 * The one place an agent's setting outranks the task's, and it is still a
 * narrowing: the local provider is the one privacy never forbids, so this can
 * only ever move work towards this machine and never away from it.
 */
func TestAnAgentThatMustStayHereStaysHere(t *testing.T) {
	books := Agent{Name: "bookkeeper", Needs: Needs{Local: true}, Provider: "openai"}

	if got := books.Through("anthropic"); got != llm.Local {
		t.Errorf("the bookkeeper was asked through %q", got)
	}

	// Its own service otherwise, then the task's.
	pinned := Agent{Name: "writer", Provider: "openai"}

	if got := pinned.Through("anthropic"); got != "openai" {
		t.Errorf("a pinned agent was asked through %q", got)
	}

	if got := (Agent{Name: "plain"}).Through("anthropic"); got != "anthropic" {
		t.Errorf("an unpinned agent was asked through %q", got)
	}
}

// What a file says about needs survives being written and read back, since the
// file is the record.
func TestNeedsSurviveBeingWrittenDown(t *testing.T) {
	want := Needs{Tools: true, Local: true}

	if got := readNeeds(want.Written()); got != want {
		t.Errorf("needs came back as %+v", got)
	}

	if got := readNeeds("vision, thinks"); !got.Vision || !got.Thinks || got.Local {
		t.Errorf("needs came back as %+v", got)
	}

	if (Needs{}).Wants() {
		t.Error("an agent with no requirements says it has some")
	}
}
