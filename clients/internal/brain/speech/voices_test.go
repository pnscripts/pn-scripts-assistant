package speech

import (
	"os"
	"path/filepath"
)

import "testing"

/*
 * "A woman's voice or a man's" is the question people actually have.
 *
 * "alba, amy, lessac, northern_english_male" is not an answer to it; it is a
 * list of names that has to be researched before it can be chosen from.
 */
func TestPickingAVoiceByKind(t *testing.T) {
	// The names carry no pattern, which is why this is a list and not a rule.
	women := []string{"en_GB-alba-medium", "en_US-amy-medium", "it_IT-paola-medium"}
	men := []string{"en_US-lessac-medium", "en_GB-northern_english_male-medium",
		"de_DE-thorsten-medium", "bg_BG-dimitar-medium"}

	for _, id := range women {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is not marked as a woman's voice", id)
		}
	}

	for _, id := range men {
		if voiceSex[id] != "man" {
			t.Errorf("%s is not marked as a man's voice", id)
		}
	}

	// Nothing is guessed at: an unknown voice is offered without a description
	// rather than assigned one.
	if voiceSex["xx_XX-unknown-medium"] != "" {
		t.Error("a voice nobody has described was given a kind anyway")
	}
}

/*
 * A piper reached through a symlink still has its voices found.
 *
 * Unpacking piper into ~/.local/share/piper and linking it from ~/.local/bin
 * is the ordinary way to install it — it is what this program's own installer
 * does — and the voices sit beside the target, not beside the link. Searching
 * only beside the link found nothing, so the neural voice was installed,
 * reported as installed, and unreachable: every answer came out in the robotic
 * fallback with nothing anywhere explaining why.
 */
func TestVoicesAreFoundThroughASymlink(t *testing.T) {
	real := t.TempDir()
	linked := t.TempDir()

	// A piper installation: the binary, with its voices in a folder beside it.
	binary := filepath.Join(real, "piper")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	voices := filepath.Join(real, "voices")
	if err := os.MkdirAll(voices, 0o755); err != nil {
		t.Fatal(err)
	}

	model := filepath.Join(voices, "en_GB-test-medium.onnx")
	for _, f := range []string{model, model + ".json"} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// And the link somebody actually runs.
	link := filepath.Join(linked, "piper")
	if err := os.Symlink(binary, link); err != nil {
		t.Skipf("cannot make symlinks here: %v", err)
	}

	found := piperVoiceFiles(&Piper{Binary: link})

	var got bool

	for _, f := range found {
		if filepath.Base(f) == "en_GB-test-medium.onnx" {
			got = true
		}
	}

	if !got {
		t.Errorf("the voice beside the real binary was not found through the link; looked at %v", found)
	}
}

/*
 * A machine voice is the honest default for a machine.
 *
 * CurrentVoice used to fall through to the first entry in the list, which is
 * sorted by name — so the default was whichever neural voice happened to sort
 * first, chosen by the alphabet rather than by anybody. It also meant the
 * assistant introduced itself in a human voice nobody had picked.
 */
func TestTheRobotIsTheDefaultVoice(t *testing.T) {
	SetVoice("")

	available := Voices()

	var hasRobot bool

	for _, v := range available {
		if v.Sex == "robot" {
			hasRobot = true
		}
	}

	if !hasRobot {
		t.Skip("nothing on this machine can be a robot")
	}

	/*
	 * A robot, rather than one particular robot.
	 *
	 * There are two now: a neural voice with the machine timbre put on it,
	 * which is clear, and espeak, which is not and is kept for the machine
	 * that has no neural voice installed. Which one is available is a fact
	 * about the machine; that the default is a machine voice is the promise.
	 */
	if got := CurrentVoice(); got.Sex != "robot" {
		t.Errorf("the default voice is %q (%s), want a robot", got.ID, got.Sex)
	}
}

// And it is offered as a kind somebody can ask for by name, not only as what
// is left when nothing else is installed.
func TestTheRobotCanBeAskedForByKind(t *testing.T) {
	asked := PickVoice("robot", "")

	if asked == "" {
		t.Skip("nothing on this machine can be a robot")
	}

	var sex string

	for _, v := range Voices() {
		if v.ID == asked {
			sex = v.Sex
		}
	}

	if sex != "robot" {
		t.Errorf("asking for a robot gave %q, which is a %q", asked, sex)
	}
}

/*
 * The robot is built from a woman's voice.
 *
 * Asked for, and the only part of the robot that is a matter of taste: the
 * machine quality comes from the delivery rather than from the timbre, so the
 * voice underneath is free to be whichever one somebody wants to listen to.
 */
func TestTheRobotIsBuiltFromAWomansVoice(t *testing.T) {
	for _, id := range RobotModels[:len(RobotModels)-1] {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is the %q voice, and it is not the last resort", id, voiceSex[id])
		}
	}

	// The last is a man's, deliberately: a robot that sounds like the wrong
	// person beats no robot at all on a machine that has only that model.
	last := RobotModels[len(RobotModels)-1]

	if voiceSex[last] != "man" {
		t.Errorf("the last resort is %q", voiceSex[last])
	}
}

/*
 * The robot never takes the woman's voice that is offered in the list.
 *
 * The model the robot is made of is hidden, so a robot built from alba would
 * silently remove the woman's voice somebody could otherwise have chosen —
 * leaving a list with a man and a robot on it and no way to say what happened.
 */
func TestTheRobotDoesNotTakeAVoiceThatIsOffered(t *testing.T) {
	for _, id := range RobotModels {
		if id == "en_GB-alba-medium" {
			t.Error("the robot would take the woman's voice out of the list")
		}
	}
}

// Whichever of them is installed is the one used, in order, so a machine
// missing the first still gets a neural robot rather than falling back to
// formant synthesis from 1985.
func TestTheRobotUsesWhicheverModelIsThere(t *testing.T) {
	if got := RobotModel(map[string]string{}); got != "" {
		t.Errorf("a machine with no voices gave %q", got)
	}

	only := map[string]string{"en_US-lessac-medium": "/somewhere/lessac.onnx"}

	if got := RobotModel(only); got != "en_US-lessac-medium" {
		t.Errorf("the only installed model gave %q", got)
	}

	both := map[string]string{
		"en_US-lessac-medium":      "/somewhere/lessac.onnx",
		"en_GB-jenny_dioco-medium": "/somewhere/jenny.onnx",
	}

	if got := RobotModel(both); got != "en_GB-jenny_dioco-medium" {
		t.Errorf("with both installed it chose %q", got)
	}
}
