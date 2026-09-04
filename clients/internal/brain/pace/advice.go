package pace

import "fmt"

/*
 * What is actually holding this machine back, and what would change it.
 *
 * A list of measured stages tells somebody where the time goes and not what to
 * do about it, and the answer to "what do I change" is different on every
 * machine this runs on. Four processor cores and an eight-billion-parameter
 * model is not a program that needs tuning — it is arithmetic — and no amount
 * of engineering on this side moves it. A graphics card with the same program
 * spends its whole delay on things this program chose.
 *
 * So each finding names the stage, says what it costs, and says the one thing
 * that would change it. Ordered by what would help most, and only findings
 * that are actually true of this machine: telling somebody with a card to
 * install a smaller model would be worse than saying nothing.
 */
type Finding struct {
	// What is slow, in the words the timings use.
	Stage string `json:"stage"`
	// How many milliseconds it is costing, when that is known.
	Costs int `json:"costs_ms"`
	// Why it costs that.
	Because string `json:"because"`
	// The one thing that would change it.
	Change string `json:"change"`
	// Doing names a thing the program can do about it, when there is one, so
	// the interface can offer it rather than describe it.
	Doing string `json:"doing,omitempty"`

	/*
	 * Fetch is the model to install, when that is the answer.
	 *
	 * Named here rather than left to the interface so that the sentence
	 * offering it and the thing actually downloaded cannot drift apart. A
	 * button that fetches something other than what the paragraph beside it
	 * promised is worse than no button.
	 */
	Fetch string `json:"fetch,omitempty"`
}

// Machine is what the advice needs to know about where it is running.
type Machine struct {
	Cores    int
	MemoryGB int
	HasGPU   bool
	GPUName  string

	// Working is the model that answers, and Quick the small one for
	// conversation. Quick empty means none is installed.
	Working string
	Quick   string

	// Suggest is the small model this machine would install if asked, which
	// the caller knows and this package deliberately does not: choosing it is
	// the model package's job.
	Suggest string
}

/*
 * RealTime is the wait that stops feeling like waiting.
 *
 * Not a round number pulled out of the air: it is roughly the pause people
 * leave for each other in conversation before it reads as hesitation. Past it,
 * somebody starts wondering whether the thing heard them — which is the
 * failure this is all about, and it is why the target is a wait rather than a
 * throughput.
 */
const RealTime = 800

/*
 * Findings works out what to say about the last few turns.
 *
 * Written against the median rather than the worst turn, because the worst is
 * always a tool that went off and read a file, and a machine judged on that
 * would be told to fix something that was working correctly.
 */
func Findings(typical Milestones, over int, m Machine) []Finding {
	var out []Finding

	if over == 0 {
		return nil
	}

	// The model, which on a machine without a card is nearly always the whole
	// of it and is the one thing this program cannot make faster.
	if typical.Thinking > RealTime {
		out = append(out, modelFinding(typical, m))
	}

	/*
	 * Then the two costs that are this program's own.
	 *
	 * They are reported even when they are small next to the model, because
	 * they are what is left over on a machine where the model is quick, and
	 * somebody moving to such a machine wants to know what will still be there
	 * when they get it.
	 */
	if typical.Speaking > 400 {
		out = append(out, Finding{
			Stage:   "speaking",
			Costs:   typical.Speaking,
			Because: "the voice takes this long to produce its first sound after a sentence is ready",
			Change: "a synthesiser is kept started and waiting, so this is generation " +
				"rather than startup. What is left is the model that makes the voice",
		})
	}

	if typical.Hearing > 600 {
		out = append(out, Finding{
			Stage:   "hearing",
			Costs:   typical.Hearing,
			Because: "the words are made out only after the recording ends",
			Change: "a resident recogniser removes the model loading from this; " +
				"what is left scales with how long you spoke",
		})
	}

	return out
}

/*
 * modelFinding is the one that differs most from machine to machine.
 *
 * Three quite different situations wear the same symptom. A card that is not
 * being used is a setup problem; no card and a large model is arithmetic, and
 * the answer is a smaller model for the turns that do not need a large one;
 * no card and no small model installed is the same arithmetic with the fix
 * simply missing.
 */
func modelFinding(typical Milestones, m Machine) Finding {
	f := Finding{
		Stage:   "thinking",
		Costs:   typical.Thinking,
		Because: fmt.Sprintf("%s runs on this machine's processor", short(typical.Model, m.Working)),
	}

	if m.HasGPU {
		f.Because = fmt.Sprintf("%s is thinking for this long even with %s to run on",
			short(typical.Model, m.Working), m.GPUName)
		f.Change = "a smaller model, or fewer tools offered per turn — the tool " +
			"descriptions are read before the first word of every answer"

		return f
	}

	/*
	 * With no card the ceiling is memory bandwidth, not cleverness.
	 *
	 * Every token means reading the whole model out of memory, so an
	 * eight-billion-parameter model at four bits is around five gigabytes per
	 * word produced. On desktop memory that is a handful of words a second at
	 * the very best, and it is why a conversational answer from a large model
	 * cannot be quick here however the program is written.
	 */
	if m.Quick == "" {
		f.Change = "install a small model for conversation — a three-billion " +
			"one answers a greeting several times quicker, and anything that " +
			"needs doing still goes to the larger one"
		f.Doing = "install-quick-model"
		f.Fetch = m.Suggest

		if f.Fetch != "" {
			f.Change = fmt.Sprintf("install %s for conversation — it answers a "+
				"greeting several times quicker on a processor, and anything "+
				"that needs doing still goes to %s", f.Fetch,
				short("", m.Working))
		}

		return f
	}

	f.Change = fmt.Sprintf("%s already answers the conversational turns; the "+
		"rest need the larger model, and on a processor that is arithmetic "+
		"rather than tuning — every word means reading the whole model out of "+
		"memory", m.Quick)

	return f
}

// short is the model that actually answered, falling back to the one this
// machine would normally use.
func short(measured, configured string) string {
	if measured != "" {
		return measured
	}

	if configured != "" {
		return configured
	}

	return "the model"
}

/*
 * Verdict is the one-line answer to "is this real time".
 *
 * Deliberately blunt. A page of stages invites somebody to hunt for the
 * comforting number; the honest summary is whether an answer starts before
 * they start wondering, and by how much it misses.
 */
func Verdict(typical Milestones, over int) string {
	if over == 0 {
		return "Nothing measured yet — say something to it and ask again."
	}

	switch {
	case typical.ToFirst <= RealTime:
		return fmt.Sprintf("About %dms from your last word to my first sound, "+
			"over %d turns. That is conversation speed.", typical.ToFirst, over)

	case typical.ToFirst < 3000:
		return fmt.Sprintf("About %.1fs from your last word to my first sound, "+
			"over %d turns. Noticeable, but you would not wonder whether I heard you.",
			float64(typical.ToFirst)/1000, over)
	}

	return fmt.Sprintf("About %.1fs from your last word to my first sound, over "+
		"%d turns. That is long enough to wonder whether I heard you at all.",
		float64(typical.ToFirst)/1000, over)
}
