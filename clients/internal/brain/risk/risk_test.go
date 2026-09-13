package risk

import "testing"

// The specification's own examples, which are the closest thing there is to a
// definition of what these four words mean.
func TestTheSpecificationsExamples(t *testing.T) {
	cases := []struct {
		say  string
		want Level
	}{
		{"Write a function that parses the config file", Low},
		{"Send the draft email to the client", Medium},
		{"Deploy the new build to production", High},
		{"Pay the electricity invoice from the business account", Critical},
		{"Transfer money to the supplier", Critical},
		{"Change the dosage of her medication", Critical},
		{"Sign the contract with the landlord", Critical},
		{"Плати фактурата за ток", Critical},
		{"Изтрий старите копия", High},
		{"Изпрати имейл на счетоводителя", Medium},
	}

	for _, c := range cases {
		if got, _ := InWords(c.say); got != c.want {
			t.Errorf("%q is %s, want %s", c.say, got, c.want)
		}
	}
}

// Matched at the start of a word and never inside one. "Prepay" and "display"
// are not "pay", and a false critical on every third instruction is how a
// warning stops being read.
func TestACueIsTheStartOfAWord(t *testing.T) {
	for _, say := range []string{
		"Display the prepaid balance",
		"Summarise the paperwork",
		"Read the spending report",
	} {
		if got, cue := InWords(say); got == Critical {
			t.Errorf("%q came out critical on %q", say, cue)
		}
	}
}

// Levels only add caution, so what is missing or misspelt is low rather than
// something that invents a question nobody asked for.
func TestParseAndMax(t *testing.T) {
	if Parse("") != Low || Parse("CRITICAL") != Critical || Parse("severe") != Low {
		t.Error("parse does not read the four words, or invents a fifth")
	}

	if Max(Low, High, Medium) != High || Max() != Low {
		t.Error("max is not the most serious")
	}

	if !Critical.AtLeast(High) || Medium.AtLeast(High) {
		t.Error("the order is wrong")
	}

	if Critical.Below() != High || Low.Below() != Low {
		t.Error("below is not one step down")
	}
}
