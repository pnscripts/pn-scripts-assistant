package tools

import "os"

// Small wrappers, so the document reader reads as prose rather than as a list
// of package-qualified calls.

func tempDir(prefix string) (string, error) { return os.MkdirTemp("", prefix) }

func removeAll(dir string) { _ = os.RemoveAll(dir) }

func readFileAt(path string) (string, error) {
	body, err := os.ReadFile(path)

	return string(body), err
}
