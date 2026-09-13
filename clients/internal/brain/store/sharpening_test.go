package store

import (
	"testing"
)

/*
 * A fact that keeps proving useful ranks above one that never has.
 *
 * The only feedback available without asking somebody to rate their own
 * assistant, and a real one: a fact that keeps coming up when the subject
 * comes up is a fact about something that keeps coming up.
 */
func TestAFactThatKeepsBeingUsefulRisesSlightly(t *testing.T) {
	db := open(t)

	// Two facts, the second a marginally worse match.
	better, _ := db.AddFact("a", "the better match", []float32{1, 0, 0})
	worse, _ := db.AddFact("a", "the worse match", []float32{0.97, 0.24, 0})

	found, err := db.Search([]float32{1, 0, 0}, 5, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	if found[0].ID != better {
		t.Fatalf("before any use, the best match is %d", found[0].ID)
	}

	// The worse one has proved useful many times.
	for i := 0; i < 30; i++ {
		if err := db.Recalled([]int64{worse}); err != nil {
			t.Fatal(err)
		}
	}

	found, _ = db.Search([]float32{1, 0, 0}, 5, 0.1)

	if found[0].ID != worse {
		t.Errorf("thirty useful recalls did not lift it: %+v", found[0])
	}

	// And the parts of the ranking stay tellable apart, so anything showing
	// its working can.
	for _, f := range found {
		if f.Similarity <= 0 {
			t.Errorf("fact %d has no similarity recorded", f.ID)
		}
	}
}

/*
 * But usefulness can never lift an irrelevant fact over a relevant one.
 *
 * The failure a bigger bonus would cause, and it would be invisible: the
 * answer would still read plausibly, built on the wrong thing.
 */
func TestUsefulnessCannotBeatRelevance(t *testing.T) {
	db := open(t)

	right, _ := db.AddFact("a", "what was asked about", []float32{1, 0, 0})
	wrong, _ := db.AddFact("a", "something else entirely", []float32{0.3, 0.95, 0})

	for i := 0; i < 500; i++ {
		db.Recalled([]int64{wrong})
	}

	found, err := db.Search([]float32{1, 0, 0}, 5, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	if found[0].ID != right {
		t.Errorf("five hundred recalls made an unrelated fact the best match: %+v", found[0])
	}
}

/*
 * Reading a place again replaces what it used to say.
 *
 * A document read in March and rewritten in June left both readings in memory,
 * equally confident, and the answer depended on which happened to be worded
 * more like the question.
 */
func TestReadingAPlaceAgainReplacesWhatItSaid(t *testing.T) {
	db := open(t)

	march, err := db.AddFactFrom("document", "the March version", "document:/notes.md",
		[]float32{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}

	june, err := db.AddFactFrom("document", "the June version", "document:/notes.md",
		[]float32{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Supersede(march, june, "read again from /notes.md"); err != nil {
		t.Fatal(err)
	}

	found, _ := db.Search([]float32{1, 0, 0}, 5, 0.1)

	if len(found) != 1 || found[0].ID != june {
		t.Fatalf("recall returned %+v", found)
	}

	// The old reading is kept rather than deleted: what it used to think is a
	// real question, and a replacement made on a bad reading should be
	// undoable.
	believed, _ := db.Believed(march)

	if believed {
		t.Error("the March version is still believed")
	}

	if err := db.Restore(march); err != nil {
		t.Fatal(err)
	}

	if believed, _ := db.Believed(march); !believed {
		t.Error("a replacement could not be undone, so it was not kept at all")
	}
}

// What each place said is findable, which is how a place that changed has its
// old readings retired.
func TestWhatOnePlaceSaidIsFindable(t *testing.T) {
	db := open(t)

	db.AddFactFrom("document", "one", "document:/notes.md", []float32{1, 0, 0})
	db.AddFactFrom("document", "two", "document:/notes.md", []float32{0, 1, 0})
	db.AddFactFrom("document", "elsewhere", "document:/other.md", []float32{0, 0, 1})

	// A conversation, which has no place and must never be swept with one.
	db.AddFact("conversation", "Petar bills in advance", []float32{0.5, 0.5, 0})

	from, err := db.FactsFromSource("document:/notes.md")
	if err != nil {
		t.Fatal(err)
	}

	if len(from) != 2 {
		t.Errorf("%d facts came from that file, want 2", len(from))
	}

	sources, err := db.LivingSources()
	if err != nil {
		t.Fatal(err)
	}

	if len(sources) != 2 {
		t.Errorf("it believes in %d places, want 2: %v", len(sources), sources)
	}
}

/*
 * What has been decided is read back at last.
 *
 * Every approval and refusal has been written down since the gate was built,
 * and nothing ever looked at any of it — so the gate could never get less
 * annoying, and a question put repeatedly about something already settled
 * stops being read and starts being clicked through.
 */
func TestWhatHasBeenDecidedIsReadBack(t *testing.T) {
	db := open(t)

	conv, _ := db.NewConversation("t")

	decide := func(tool, how string, times int) {
		for i := 0; i < times; i++ {
			id, err := db.RecordInvocation(conv, tool, "{}", "do the thing", "mutating")
			if err != nil {
				t.Fatal(err)
			}

			if _, err := db.DecideInvocation(id, how); err != nil {
				t.Fatal(err)
			}
		}
	}

	decide("write_file", InvocationApproved, 6)
	decide("send_email", InvocationDenied, 5)
	decide("run_command", InvocationApproved, 2)
	decide("run_command", InvocationDenied, 2)

	// And one still waiting, which is not a decision.
	db.RecordInvocation(conv, "write_file", "{}", "do the thing", "mutating")

	habits, err := db.Habits()
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]Habit{}
	for _, h := range habits {
		by[h.Tool] = h
	}

	if got := by["write_file"]; got.Approved != 6 || got.Refused != 0 {
		t.Errorf("write_file: %+v", got)
	}

	if got := by["send_email"]; got.Refused != 5 || got.Approved != 0 {
		t.Errorf("send_email: %+v", got)
	}

	// Settled means enough decisions, all the same way. Something answered
	// both ways is a judgement somebody is still making.
	if !by["write_file"].Settled(4) {
		t.Error("six yeses and no noes is not settled")
	}

	if !by["send_email"].Settled(4) {
		t.Error("five noes and no yeses is not settled")
	}

	if by["run_command"].Settled(4) {
		t.Error("two each way was taken for a habit")
	}

	if by["write_file"].Last.IsZero() {
		t.Error("nothing says when it was last decided, so a changed mind cannot be seen")
	}

}

/*
 * Everything that means "what it knows" means "what it still believes".
 *
 * One rule, in every place that counts or lists memory, because they are all
 * read as the same claim. The listing showed a fact whose file had been
 * deleted, sitting beside a live one with nothing to tell them apart; the
 * count went into the system prompt, so the model was told every turn that it
 * knew more than it did; and the voice vocabulary was built from words it had
 * stopped believing.
 */
func TestWhatItKnowsMeansWhatItStillBelieves(t *testing.T) {
	db := open(t)

	live, _ := db.AddFactFrom("document", "still true", "document:/here.md", []float32{1, 0, 0})
	old, _ := db.AddFactFrom("document", "the March version", "document:/notes.md", []float32{0, 1, 0})
	lost, _ := db.AddFactFrom("document", "from a deleted file", "document:/gone.md", []float32{0, 0, 1})

	newer, _ := db.AddFactFrom("document", "the June version", "document:/notes.md", []float32{0, 1, 0})

	if err := db.Supersede(old, newer, "read again"); err != nil {
		t.Fatal(err)
	}

	if err := db.Retire(lost, "its source is gone"); err != nil {
		t.Fatal(err)
	}

	counted, err := db.CountFacts()
	if err != nil {
		t.Fatal(err)
	}

	if counted != 2 {
		t.Errorf("it says it knows %d things, want 2 — the count goes in the prompt", counted)
	}

	listed, err := db.RecentFacts(50)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range listed {
		if f.ID == old || f.ID == lost {
			t.Errorf("the listing shows %q, which it no longer believes", f.Content)
		}
	}

	if len(listed) != 2 {
		t.Errorf("the listing shows %d things, want 2", len(listed))
	}

	// The vocabulary the speech recogniser is given, too: a name it has
	// stopped believing should not be a word it listens harder for.
	all, err := db.AllFacts()
	if err != nil {
		t.Fatal(err)
	}

	if len(all) != 2 {
		t.Errorf("the vocabulary is built from %d facts, want 2", len(all))
	}

	spread, err := db.FactsByCategory()
	if err != nil {
		t.Fatal(err)
	}

	if spread["document"] != 2 {
		t.Errorf("the spread says %d documents, want 2", spread["document"])
	}

	superseded, retired, err := db.CountRetired()
	if err != nil {
		t.Fatal(err)
	}

	if superseded != 1 || retired != 1 {
		t.Errorf("it has unlearned %d replaced and %d gone, want 1 and 1", superseded, retired)
	}

	_ = live
}
