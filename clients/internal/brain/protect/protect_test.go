package protect

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func fresh(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	Use(Choices{})
	OwnFolder(root)

	t.Cleanup(func() {
		Use(Choices{})
		OwnFolder("")
	})

	return root
}

/*
 * Nothing is out of reach; the things that matter come to the owner first.
 *
 * This is his machine and his files. The list does not forbid anything — it
 * decides what gets a question, and a question is the one step the attack this
 * exists for cannot survive: a page that talks the brain into reading a key
 * and posting it somewhere depends entirely on nobody seeing it happen.
 */
func TestItAsksRatherThanRefuses(t *testing.T) {
	fresh(t)

	rule, ask := Ask("/home/petar/.ssh/id_rsa")

	if !ask {
		t.Fatal("a private key raised no question at all")
	}

	if rule.What == "" || rule.Why == "" {
		t.Errorf("the question comes with no reason to weigh: %+v", rule)
	}

	if _, ask := Ask("/home/petar/Projects/app/README.md"); ask {
		t.Error("an ordinary file was held up")
	}
}

/*
 * Answering once is answering.
 *
 * A question put a second time about a thing already decided is a question
 * that stops being read and starts being clicked through — and at that point
 * the prompt looks like protection while being worse than none.
 */
func TestItLearnsTheAnswerAndStopsAsking(t *testing.T) {
	root := fresh(t)

	const key = "/home/petar/.ssh/id_rsa"

	if _, ask := Ask(key); !ask {
		t.Fatal("it did not ask the first time")
	}

	Learn(root, key)

	if _, ask := Ask(key); ask {
		t.Error("it asked again about a file already allowed")
	}

	// One file, not the rule it matched. Saying yes to one key must not open
	// the whole folder.
	if _, ask := Ask("/home/petar/.ssh/id_ed25519"); !ask {
		t.Error("allowing one key allowed every key in the folder")
	}

	// And it survives a restart, since a decision that has to be made again
	// tomorrow was not really recorded.
	Use(Choices{})
	Use(Load(root))

	if _, ask := Ask(key); ask {
		t.Error("what it learned was lost when it reopened")
	}

	// Taking it back is what makes remembering safe to do automatically.
	if err := Forget(root, key); err != nil {
		t.Fatal(err)
	}

	if _, ask := Ask(key); !ask {
		t.Error("it kept allowing a file that was taken back")
	}
}

// Somebody who does not want to be asked about a whole category can say so,
// and somebody who wants to be asked about something of their own can add it.
func TestTheOwnerDecidesWhatIsWorthAsking(t *testing.T) {
	root := fresh(t)

	if err := Save(root, Choices{Off: []string{"browsers"}, Extra: []string{"/tax-returns/"}}); err != nil {
		t.Fatal(err)
	}

	if _, ask := Ask("/home/petar/.mozilla/firefox/key4.db"); ask {
		t.Error("a rule that was switched off still asked")
	}

	if _, ask := Ask("/home/petar/Documents/tax-returns/2025.pdf"); !ask {
		t.Error("something the owner added was not asked about")
	}

	/*
	 * Except the one that cannot be switched off. Somebody turning off the
	 * system password file is not expressing a preference.
	 */
	if err := Save(root, Choices{Off: []string{"system"}}); err != nil {
		t.Fatal(err)
	}

	if _, ask := Ask("/etc/shadow"); !ask {
		t.Error("/etc/shadow was opened up")
	}
}

/*
 * The brain's own folder, wherever its owner put it.
 *
 * Its settings hold the mail password and every API key in plain text, and its
 * database is everything it has ever been told. A rule naming a folder called
 * PN-BRAIN-DATA would protect nobody who chose a different name.
 */
func TestTheBrainsOwnFolderIsAskedAboutToo(t *testing.T) {
	root := fresh(t)

	for _, path := range []string{
		filepath.Join(root, "brain.conf"),
		filepath.Join(root, "brain.sqlite"),
		filepath.Join(root, "anything", "else.txt"),
	} {
		if _, ask := Ask(path); !ask {
			t.Errorf("the brain's own %s was not asked about", filepath.Base(path))
		}
	}
}

/*
 * The protected thing is found whatever the argument is called.
 *
 * Tools name it path, file, directory and folder, and a list of which tool
 * uses which would be one more thing to keep up to date and to get wrong.
 */
func TestItFindsTheFileWhateverTheArgumentIsCalled(t *testing.T) {
	fresh(t)

	for _, args := range []string{
		`{"path":"/home/petar/.aws/credentials"}`,
		`{"file":"/home/petar/.aws/credentials"}`,
		`{"directory":"/home/petar/.aws/"}`,
		`{"files":["/tmp/fine.txt","/home/petar/.aws/credentials"]}`,
		`{"where":{"deeper":"/home/petar/.aws/credentials"}}`,
	} {
		if _, _, ask := InArguments(json.RawMessage(args)); !ask {
			t.Errorf("missed the protected file in %s", args)
		}
	}

	if _, _, ask := InArguments(json.RawMessage(`{"path":"/home/petar/notes.md"}`)); ask {
		t.Error("an ordinary call was held up")
	}
}
