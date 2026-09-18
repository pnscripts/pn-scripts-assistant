package store

import "testing"

// What an agent learned on one project stays with that project.
func TestAMemoryStaysWithItsProject(t *testing.T) {
	db := open(t)

	a, _ := db.NewTask(Task{Name: "acme game", Goal: "x", Project: "/work/acme"})
	b, _ := db.NewTask(Task{Name: "other game", Goal: "x", Project: "/work/other"})

	db.Remember(Memory{Agent: "dev", Kind: Learned, TaskID: a,
		Content: "ACME's unreleased game codename BLUEBIRD keeps its level layout in levels/secret.json"})

	if got, _ := db.RecallIn("dev", "design the level layout", "/work/other", 3); len(got) != 0 {
		t.Errorf("another project recalled it: %+v", got)
	}

	if got, _ := db.RecallIn("dev", "design the level layout", "", 3); len(got) != 0 {
		t.Errorf("work on no project recalled it: %+v", got)
	}

	if got, _ := db.RecallIn("dev", "design the level layout", "/work/acme", 3); len(got) != 1 {
		t.Errorf("its own project did not recall it: %+v", got)
	}

	_ = b
}
