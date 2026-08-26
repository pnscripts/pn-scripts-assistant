//go:build darwin

package storage

// mountPoints lists the places a brain could live on macOS.
//
// Deliberately simple: the boot volume and whatever is in /Volumes, which is
// where macOS mounts everything else. Reading the real mount table needs
// getmntinfo through cgo, and that would make the whole program need a C
// toolchain to build for a list that is this short in practice.
//
// Untested: there is no macOS machine in this project's development
// environment, so this compiles but has not been run.
func mountPoints() ([]mount, error) {
	out := []mount{{Path: "/", Type: "apfs"}}

	entries, err := readDirNames("/Volumes")
	if err != nil {
		return out, nil
	}

	for _, name := range entries {
		out = append(out, mount{Path: "/Volumes/" + name, Type: "external"})
	}

	return out, nil
}
