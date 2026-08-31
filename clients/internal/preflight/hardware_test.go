package preflight

import "testing"

/*
 * Every machine is offered a choice, and always a sensible one.
 *
 * A single suggestion is the right answer for somebody with no way to judge
 * between eight models, but it made a trade on their behalf — a larger model
 * that answers well and slowly — and that is exactly the trade people differ
 * on. Somebody who wants a reply in two seconds and somebody who wants the
 * best this machine can produce were both served a single choice made for
 * them.
 */
func TestEveryMachineIsOfferedAChoice(t *testing.T) {
	for _, hw := range []Hardware{
		{RAMGB: 32, HasGPU: true},
		{RAMGB: 31},
		{RAMGB: 8},
		{RAMGB: 4},
	} {
		options := ModelOptions(hw)

		if len(options) == 0 {
			t.Errorf("%dGB, gpu=%v: offered nothing at all", hw.RAMGB, hw.HasGPU)

			continue
		}

		// Few enough to read at a glance. More than this is the list that made
		// a single suggestion the better design in the first place.
		if len(options) > 4 {
			t.Errorf("%dGB: offered %d models, which is a list rather than a choice",
				hw.RAMGB, len(options))
		}

		var recommended int

		for _, o := range options {
			if o.Model == "" || o.Label == "" || o.SpeedNote == "" {
				t.Errorf("%dGB: an option is missing its description: %+v", hw.RAMGB, o)
			}

			if o.Recommended {
				recommended++
			}
		}

		// Exactly one, or the page has nothing to mark and somebody with no
		// way to judge is back where they started.
		if recommended != 1 {
			t.Errorf("%dGB: %d options marked recommended, want exactly 1",
				hw.RAMGB, recommended)
		}
	}
}

/*
 * The quick model is offered again.
 *
 * It used to be avoided because it mishandled tools — asked to say a word it
 * called write_file instead. That was true, and it is now handled before the
 * model sees anything: conversation is offered no tools at all, and the ones
 * that change the machine have to be asked for. On a processor the difference
 * between three billion parameters and seven is the difference between a reply
 * and a wait, so hiding it costs more than it saves.
 */
func TestTheQuickModelIsOfferedOnAProcessor(t *testing.T) {
	options := ModelOptions(Hardware{RAMGB: 31})

	var quick, balanced bool

	for _, o := range options {
		if o.Model == "llama3.2:3b" {
			quick = true
		}

		if o.Model == "qwen2.5-coder:7b" {
			balanced = true
		}
	}

	if !quick {
		t.Error("a machine with no graphics card was not offered the quick model, " +
			"so its only option answers in minutes")
	}

	if !balanced {
		t.Error("the abler model was not offered, so there is nothing to choose between")
	}
}
