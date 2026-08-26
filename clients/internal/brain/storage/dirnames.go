package storage

import "os"

// readDirNames lists the entries of a directory by name.
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(entries))

	for _, e := range entries {
		out = append(out, e.Name())
	}

	return out, nil
}
