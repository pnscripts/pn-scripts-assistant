package progress

import "testing"

/*
 * A step carries the tool's own words in Bulgarian beside the English.
 *
 * The approval card has been Bulgarian on a Bulgarian page since the card
 * was kept in both languages, but Doing now went on reading "Чака вашето
 * решение: Create ..." with the English card inside the Bulgarian sentence:
 * the step knew only the English line. The Bulgarian card travels with the
 * step now, without the words around it ("Waiting for you: "), which the
 * page says in its language, and it stays with the row when the question is
 * decided.
 */
func TestAStepCarriesItsCardInBulgarian(t *testing.T) {
	fresh(t)

	Begin()
	SetToolIn("read_file", "Read notes.txt", "Прочитане на notes.txt")

	if now := Now(); now.Note != "Read notes.txt" || now.SummaryBG != "Прочитане на notes.txt" || now.Tool != "read_file" {
		t.Errorf("the running step: %+v", now)
	}

	WaitingIn("Waiting for you: Create notes.txt", "Създаване на notes.txt", 11)

	if now := Now(); now.Kind != "waiting" || now.Note != "Waiting for you: Create notes.txt" ||
		now.SummaryBG != "Създаване на notes.txt" {
		t.Errorf("the waiting step: %+v", now)
	}

	Done()

	all := Recent()
	if len(all) != 2 || all[0].SummaryBG != "Прочитане на notes.txt" || all[1].SummaryBG != "Създаване на notes.txt" {
		t.Fatalf("the history: %+v", all)
	}

	// Decided, the row says how in English and keeps the same Bulgarian
	// card: the page puts its own "Отказано: " before it.
	if !Decided(11, Refused) {
		t.Fatal("the row of question 11 was not found")
	}

	last := Recent()[1]
	if last.Note != "Refused: Create notes.txt" || last.SummaryBG != "Създаване на notes.txt" {
		t.Errorf("the decided row: %+v", last)
	}
}

// A step said without a Bulgarian card has none, and a later step never
// inherits the card of the one before it.
func TestTheBulgarianCardBelongsToItsOwnStep(t *testing.T) {
	fresh(t)

	Begin()
	SetToolIn("read_file", "Read notes.txt", "Прочитане на notes.txt")

	for name, next := range map[string]func(){
		"Set":           func() { Set("thinking", "Thinking") },
		"SetTool":       func() { SetTool("list_dir", "List /tmp") },
		"Waiting":       func() { Waiting("Waiting for you: Run it", 12) },
		"Progressing":   func() { Progressing("learning", "Reading a.txt: passage 2 of 3") },
		"SetBackground": func() { SetBackground("learning", "Learning from the last conversation") },
		"Begin":         func() { Begin() },
	} {
		SetToolIn("read_file", "Read notes.txt", "Прочитане на notes.txt")
		next()

		if now := Now(); now.SummaryBG != "" {
			t.Errorf("after %s the step still carries the last card: %+v", name, now)
		}

		if all := Recent(); name != "Begin" && all[len(all)-1].SummaryBG != "" {
			t.Errorf("after %s the newest row carries the last card: %+v", name, all[len(all)-1])
		}
	}

	Done()

	if now := Now(); now.SummaryBG != "" {
		t.Errorf("nothing is running, and it says %+v", now)
	}
}

// The same step said twice is still one row, and it keeps its card.
func TestARepeatedStepKeepsItsCard(t *testing.T) {
	fresh(t)

	SetToolIn("read_file", "Read notes.txt", "Прочитане на notes.txt")
	SetToolIn("read_file", "Read notes.txt", "Прочитане на notes.txt")

	if all := Recent(); len(all) != 1 || all[0].SummaryBG != "Прочитане на notes.txt" {
		t.Errorf("%+v", all)
	}
}
