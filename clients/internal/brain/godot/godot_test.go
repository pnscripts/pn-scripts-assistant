package godot

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * "4.10" is later than "4.9" and sorts before it.
 *
 * Compared as numbers, because telling somebody an update exists when it does
 * not is worse than staying quiet — they go and look, and find nothing, and
 * stop believing the next one.
 */
func TestAVersionIsComparedAsNumbers(t *testing.T) {
	newer := [][2]string{
		{"4.2.stable.official", "4.3-stable"},
		{"4.9-stable", "4.10-stable"},
		{"3.5.2-stable", "4.0-stable"},
		{"4.3.stable", "4.3.1-stable"},
	}

	for _, c := range newer {
		if !Newer(c[0], c[1]) {
			t.Errorf("%s should be newer than %s", c[1], c[0])
		}
	}

	same := [][2]string{
		{"4.3-stable", "4.3-stable"},
		{"4.3.1-stable", "4.3-stable"},
		{"4.10-stable", "4.9-stable"},
		// Unreadable answers false: quiet beats wrong.
		{"custom build", "4.3-stable"},
		{"4.3-stable", "who knows"},
	}

	for _, c := range same {
		if Newer(c[0], c[1]) {
			t.Errorf("%s should not be reported as newer than %s", c[1], c[0])
		}
	}
}

/*
 * The reference branch matches the engine that will run the code.
 *
 * A project on 4.2 given master's reference is being told about methods it
 * does not have, which is the failure the lookup exists to prevent arriving by
 * a different door.
 */
func TestTheReferenceMatchesTheInstalledEngine(t *testing.T) {
	for version, want := range map[string]string{
		"4.3.stable.official": "4.3",
		"4.2.1-stable":        "4.2",
		"3.5.3-stable":        "3.5",
		"something odd":       "master",
		"":                    "master",
	} {
		if got := BranchFor(version); got != want {
			t.Errorf("BranchFor(%q) = %q, want %q", version, got, want)
		}
	}
}

/*
 * A project's addons are not projects of their own.
 *
 * The same mistake the document scanner made with node_modules, and it would
 * present the same way: somebody's one game reported as eleven.
 */
func TestAddonsAreNotSeparateGames(t *testing.T) {
	root := t.TempDir()

	game := filepath.Join(root, "MyGame")
	addon := filepath.Join(game, "addons", "dialogue")

	os.MkdirAll(addon, 0o755)
	os.WriteFile(filepath.Join(game, "project.godot"),
		[]byte("config_version=5\n\n[application]\n\nconfig/name=\"Deep Space\"\n"), 0o644)
	os.WriteFile(filepath.Join(addon, "project.godot"), []byte("config_version=5\n"), 0o644)

	found, err := Projects(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 {
		t.Fatalf("found %d projects, want 1: %+v", len(found), found)
	}

	// And it is called what its author called it, not what the folder is.
	if found[0].Name != "Deep Space" {
		t.Errorf("the project is called %q, want its own name", found[0].Name)
	}
}

// A class name goes into a URL, so it has to be a name.
func TestAClassNameIsNotAPath(t *testing.T) {
	for _, bad := range []string{"../../etc/passwd", "Node2D/../x", "a b", "", "x.xml"} {
		if _, err := Lookup(t.Context(), nil, bad, "master"); err == nil {
			t.Errorf("%q was accepted as a class name", bad)
		}
	}
}

// The signature is the part a model cannot guess.
func TestASignatureIsWrittenAsGDScript(t *testing.T) {
	m := Method{
		Name:       "move_and_collide",
		Qualifiers: "const",
		Return:     Typed{Type: "KinematicCollision2D"},
		Params: []Param{
			{Name: "motion", Type: "Vector2"},
			{Name: "test_only", Type: "bool", Default: "false"},
		},
	}

	want := "move_and_collide(motion: Vector2, test_only: bool = false) -> KinematicCollision2D const"

	if got := m.Signature(); got != want {
		t.Errorf("signature is\n  %s\nwant\n  %s", got, want)
	}
}
