//go:build windows

package storage

// mountPoints lists the drive letters that exist.
//
// Untested: there is no Windows machine in this project's development
// environment, so this compiles but has not been run.
func mountPoints() ([]mount, error) {
	var out []mount

	for letter := 'A'; letter <= 'Z'; letter++ {
		path := string(letter) + `:\`

		// spaceOn fails for a letter nothing is mounted on, which is how an
		// absent drive is recognised without a separate API call.
		if _, _, err := spaceOn(path); err != nil {
			continue
		}

		out = append(out, mount{Path: path, Type: "ntfs"})
	}

	return out, nil
}
