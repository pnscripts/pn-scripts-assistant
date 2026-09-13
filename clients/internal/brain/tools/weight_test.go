package tools

import (
	"encoding/json"
	"testing"

	"pn-scripts-assistant/internal/brain/risk"
)

// One tool, very different calls. Listing a folder and formatting a disk are
// both run_command, and the approval question has to be able to tell them
// apart or it tells the person approving nothing.
func TestACommandIsWeighedOnWhatItRuns(t *testing.T) {
	cases := map[string]risk.Level{
		`["ls","-la"]`:                      risk.Medium,
		`["go","test","./..."]`:             risk.Medium,
		`["git","push","--force"]`:          risk.High,
		`["sudo","apt","install","ffmpeg"]`: risk.High,
		`["rm","-rf","build"]`:              risk.High,
		`["rm","-rf","/"]`:                  risk.Critical,
		`["sudo","mkfs.ext4","/dev/sdb1"]`:  risk.Critical,
		`["dd","if=x.img","of=/dev/sda"]`:   risk.Critical,
		`["systemctl","restart","nginx"]`:   risk.High,
		`["/usr/bin/shred","secrets.txt"]`:  risk.Critical,
	}

	for argv, want := range cases {
		got := LevelOf(RunCommand{}, json.RawMessage(`{"argv":`+argv+`}`))

		if got != want {
			t.Errorf("%s is %s, want %s", argv, got, want)
		}
	}
}

// Everything that says nothing about weight follows from the gate: looking is
// low, changing is medium.
func TestAToolWithNoWeighingFollowsItsRisk(t *testing.T) {
	if LevelOf(ListDirectory{}, nil) != risk.Low {
		t.Error("looking is not low")
	}

	if LevelOf(SendEmail{}, nil) != risk.Medium {
		t.Error("sending an email is not medium, which is what the specification calls it")
	}
}

func TestWritingWhereTheSystemLivesIsHigh(t *testing.T) {
	if got := LevelOf(WriteFile{}, json.RawMessage(`{"path":"/etc/hosts"}`)); got != risk.High {
		t.Errorf("writing /etc/hosts is %s", got)
	}

	if got := LevelOf(WriteFile{}, json.RawMessage(`{"path":"/home/someone/notes.md"}`)); got != risk.Medium {
		t.Errorf("writing a note is %s", got)
	}
}
