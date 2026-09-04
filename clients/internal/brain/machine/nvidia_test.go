package machine

import (
	"context"
	"errors"
	"testing"
)

/*
 * The three cards on the machine this was built for.
 *
 * Taken from what nvidia-smi actually prints there rather than invented, so
 * the parser is checked against the thing it has to read: an idle card with a
 * model resident in its memory, which is the ordinary state of an inference
 * server and the one a wall display exists to show.
 */
const threeCards = `0, NVIDIA GeForce RTX 5060 Ti, 0, 0, 11502, 16311, 41, 0, 4.00, 180.00, 210, 405
1, NVIDIA GeForce RTX 5060 Ti, 0, 0, 12512, 16311, 39, 0, 4.00, 180.00, 210, 405
2, NVIDIA GeForce RTX 5060 Ti, 0, 0, 11672, 16311, 36, 0, 1.00, 180.00, 210, 405`

func TestReadingThreeCards(t *testing.T) {
	cards := parseNvidia(threeCards)

	if len(cards) != 3 {
		t.Fatalf("read %d cards, want 3", len(cards))
	}

	first := cards[0]

	if first.Name != "NVIDIA GeForce RTX 5060 Ti" {
		t.Errorf("name is %q", first.Name)
	}

	if first.TemperatureC != 41 || first.PowerWatts != 4 || first.PowerCapWatts != 180 {
		t.Errorf("temperature/power read as %v/%v of %v",
			first.TemperatureC, first.PowerWatts, first.PowerCapWatts)
	}

	if first.MemoryTotalBytes != 16311*1024*1024 {
		t.Errorf("total memory is %d bytes", first.MemoryTotalBytes)
	}

	/*
	 * The memory figure is how full it is, not how busy the bus was.
	 *
	 * nvidia-smi's utilization.memory is the fraction of time the memory bus
	 * was in use, which for a card holding a model and answering nothing is
	 * zero — while the card is in fact 70% full. The second is what anybody
	 * reading a dashboard means by "GPU memory", and showing the first under
	 * that label is how a wall display says a loaded card is empty.
	 */
	if got := first.MemoryPercent; got < 70 || got > 71 {
		t.Errorf("memory is %.1f%% full; 11502 of 16311 MiB is about 70.5%%", got)
	}

	if cards[2].PowerWatts != 1 {
		t.Errorf("the third card draws %v watts, not 1", cards[2].PowerWatts)
	}
}

/*
 * A card that declines to answer a column does not take the row with it.
 *
 * Laptop cards report no fan, some drivers publish no power limit, and a
 * virtualised card answers "[N/A]" to half of it. Throwing away the whole
 * reading because one column was missing would show no cards at all on a
 * machine that has them.
 */
func TestAMissingFigureIsNotAMissingCard(t *testing.T) {
	cards := parseNvidia(
		"0, NVIDIA T1000, 12, [N/A], 500, 4096, 45, [N/A], [N/A], [N/A], 300, 810")

	if len(cards) != 1 {
		t.Fatalf("read %d cards, want 1", len(cards))
	}

	c := cards[0]

	if c.UtilPercent != 12 || c.TemperatureC != 45 {
		t.Errorf("the figures it did give were lost: %+v", c)
	}

	if c.FanPercent != -1 || c.PowerWatts != -1 || c.PowerCapWatts != -1 {
		t.Errorf("a figure it declined to give was read as a number: %+v", c)
	}

	if c.MemoryUsedBytes != 500*1024*1024 {
		t.Errorf("used memory is %d bytes", c.MemoryUsedBytes)
	}
}

/*
 * A machine with no NVIDIA card says so, and is not a fault.
 *
 * Most machines have none — including the one this program was written on —
 * and an empty table with no explanation reads as something being broken.
 */
func TestNoCardIsAnAnswerNotAnError(t *testing.T) {
	was := askNvidia
	askNvidia = func(context.Context) (string, error) { return "", errors.New("exec: not found") }

	t.Cleanup(func() { askNvidia = was })

	cards, note := graphicsCards()

	if len(cards) != 0 {
		t.Errorf("cards were invented: %+v", cards)
	}

	if note == "" {
		t.Error("it shows nothing and says nothing about why")
	}
}
