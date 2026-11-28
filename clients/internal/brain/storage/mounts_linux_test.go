package storage

import "testing"

// A pseudo filesystem (proc, sysfs, tmpfs, overlay...) never holds data, so it
// is never offered as a place for the brain. The list of them is read from
// /proc/mounts and exists on Linux only, so the check lives beside it.
func TestDrivesExcludePseudoFilesystems(t *testing.T) {
	drives, err := Drives("/")
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range drives {
		if pseudoFilesystems[d.Filesystem] {
			t.Errorf("offered a pseudo filesystem: %+v", d)
		}
	}
}
