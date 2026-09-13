package tools

import (
	"context"
	"strings"
	"testing"
)

/*
 * "My external drive" has to be answerable by looking.
 *
 * Asked to find projects on one, the assistant asked which drive was meant —
 * three times, while the drive stayed mounted throughout. Not unhelpfulness:
 * nothing it had could answer the question, so asking was the only move it
 * had. A mount point is not something anybody knows or should have to.
 */
func TestTheDrivesToolNamesEachDriveAndItsKind(t *testing.T) {
	out, err := ListDrives{}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if out == "" {
		t.Fatal("it listed nothing at all")
	}

	if !strings.Contains(out, "the home folder") {
		t.Error("the home folder is missing, and it is where somebody's own work usually is")
	}
}

/*
 * And exactly one drive holds the brain.
 *
 * Every absolute path begins with "/", so a plain prefix test marks the root
 * filesystem as holding the brain wherever the brain actually is — an answer
 * naming two homes, one of them wrong.
 */
func TestOnlyOneDriveIsSaidToHoldTheBrain(t *testing.T) {
	out, err := ListDrives{Root: "/media/somebody/stick/PN-SCRIPTS-ASSISTANT-DATA"}.
		Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if n := strings.Count(out, "the brain's own memory is kept here"); n > 1 {
		t.Errorf("%d drives claim to hold the brain:\n%s", n, out)
	}
}
