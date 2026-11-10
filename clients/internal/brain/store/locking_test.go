package store

import (
	"fmt"
	"sync"
	"testing"
)

/*
 * A transaction that reads before it writes waits its turn instead of
 * failing.
 *
 * SQLite starts a transaction as a reader. When it then writes while another
 * connection has written since it started reading, the upgrade cannot wait:
 * it fails at once with "database is locked" (SQLITE_BUSY), and busy_timeout
 * does not help. AddSteps reads the highest position and then inserts, so a
 * goal's steps were lost now and again while the job before it was still
 * writing (TestAGoalWorksInItsOwnFolderAndReportsWhatWasPrepared, release
 * rehearsal 10.11.2026, about once in seventy runs). Every transaction here
 * writes, so each takes the write lock when it begins and busy_timeout turns
 * the contention into a short wait.
 */
func TestTransactionsThatReadFirstWaitForTheWriteLock(t *testing.T) {
	db := open(t)

	id, err := db.NewTask(Task{Name: "contended", Goal: "write steps while others write", State: TaskWorking})
	if err != nil {
		t.Fatal(err)
	}

	const writers, rounds = 6, 25

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	fail := func(err error) {
		mu.Lock()
		errs = append(errs, err)
		mu.Unlock()
	}

	for w := 0; w < writers; w++ {
		wg.Add(2)

		go func(w int) {
			defer wg.Done()

			for r := 0; r < rounds; r++ {
				step := TaskStep{Instruction: fmt.Sprintf("step %d.%d", w, r)}
				if err := db.AddSteps(id, []TaskStep{step, step}); err != nil {
					fail(err)
				}
			}
		}(w)

		// Plain writes beside them, the way a running job records progress.
		go func(w int) {
			defer wg.Done()

			for r := 0; r < rounds; r++ {
				if err := db.SetTaskJob(id, int64(w*rounds+r)); err != nil {
					fail(err)
				}
			}
		}(w)
	}

	wg.Wait()

	for _, err := range errs {
		t.Error(err)
	}

	steps, err := db.Steps(id)
	if err != nil {
		t.Fatal(err)
	}

	if len(steps) != writers*rounds*2 {
		t.Fatalf("%d steps written, want %d", len(steps), writers*rounds*2)
	}

	// And no two steps claim the same place.
	seen := map[int]bool{}
	for _, s := range steps {
		if seen[s.Position] {
			t.Errorf("two steps at position %d", s.Position)
		}

		seen[s.Position] = true
	}
}
