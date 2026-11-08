package machinecare

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A session that was closed says so; one that was cut off says when it was
// last seen working.
func TestAnUncleanStopIsNoticed(t *testing.T) {
	root := t.TempDir()
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

	if last, err := Begin(root, t0); err != nil || last.Known {
		t.Fatalf("a first start knows something: %+v %v", last, err)
	}

	if err := End(root, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	last, _ := Begin(root, t0.Add(2*time.Hour))
	if !last.Known || last.Unclean || !last.LastSeen.Equal(t0.Add(time.Hour)) {
		t.Fatalf("a closed session: %+v", last)
	}

	// Cut off: no End, and the database was last written at 11:40.
	written := t0.Add(2*time.Hour + 40*time.Minute)
	_ = os.WriteFile(filepath.Join(root, "brain.sqlite-wal"), []byte("x"), 0o600)
	_ = os.Chtimes(filepath.Join(root, "brain.sqlite-wal"), written, written)

	last, _ = Begin(root, t0.Add(5*time.Hour))
	if !last.Unclean || !last.Started.Equal(t0.Add(2*time.Hour)) || !last.LastSeen.Equal(written) {
		t.Fatalf("a cut-off session: %+v", last)
	}
}

func TestLeftoversAreOnlyFinishedJobsOfThisUser(t *testing.T) {
	const me, self = 1000, 500

	hour := time.Hour

	procs := []Proc{
		{PID: 10, PPID: 1, UID: me, Age: hour, Command: "/usr/bin/google-chrome --headless=new --mute-audio about:blank"},
		{PID: 11, PPID: 2000, UID: me, Age: hour, Command: "Xvfb :137 -screen 0 1280x800x24", Parent: "systemd"},
		{PID: 12, PPID: self, UID: me, Age: hour, Command: "sleep 600"},
		{PID: 13, PPID: 13, UID: me, Age: hour, Command: "python3 -m http.server 8000", Parent: "bash"},
		{PID: 14, PPID: 1, UID: me, Age: hour, Command: "/usr/bin/google-chrome --profile-directory=Default"},
		{PID: 15, PPID: 1, UID: 0, Age: hour, Command: "sleep 600"},
		{PID: 16, PPID: 1, UID: me, Age: 5 * time.Minute, Command: "tail -f /tmp/x.log"},
		{PID: 17, PPID: 1, UID: me, TTY: true, Age: hour, Command: "tail -f /tmp/x.log"},
		{PID: 18, PPID: 12, UID: me, Age: hour, Command: "timeout 60 sleep 30"},
		{PID: self, PPID: 1, UID: me, Age: hour, Command: "pn-scripts-assistant serve --headless"},
	}

	var got []int

	for _, l := range Find(procs, me, self) {
		got = append(got, l.PID)

		if l.PID == 12 && (!l.Ours || l.Kind != "waiting command") {
			t.Errorf("%+v", l)
		}
	}

	// The browser with a window (14), root's process (15), a young one (16),
	// one in a terminal (17), one whose parent is alive (13) and the
	// assistant itself are left alone.
	if !reflect.DeepEqual(got, []int{10, 11, 12, 18}) {
		t.Errorf("%v", got)
	}
}

func TestTheProcessesHereAreRead(t *testing.T) {
	procs, err := Processes()
	if err != nil {
		t.Skip("not on this system")
	}

	for _, p := range procs {
		if p.PID == os.Getpid() {
			if p.UID != os.Getuid() || !strings.Contains(p.Command, "machinecare") || p.PPID != os.Getppid() || p.Age < 0 {
				t.Errorf("%+v", p)
			}

			return
		}
	}

	t.Error("this test's own process was not read")
}

// Only a process on the list is stopped, and it is.
func TestStopEndsOnlyALeftover(t *testing.T) {
	if _, err := Processes(); err != nil {
		t.Skip("not on this system")
	}

	if err := Stop(os.Getpid(), "anything"); err != ErrNotALeftover {
		t.Errorf("the test itself: %v", err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}

	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	// Young, so not a leftover yet: refused.
	if err := Stop(cmd.Process.Pid, "sleep 30"); err != ErrNotALeftover {
		t.Errorf("a young process was stopped: %v", err)
	}
}

func TestRestartReadiness(t *testing.T) {
	dkms := strings.Join([]string{
		"rtl8821au/5.12.5.2, 6.8.0-147-generic, x86_64: installed",
		"rtl8821au/5.12.5.2, 6.8.0-149-generic, x86_64: installed",
		"nvidia, 550.1, 6.8.0-147-generic, x86_64: installed",
		"nvidia, 550.1, 6.8.0-149-generic, x86_64: built",
	}, "\n")

	r := Judge(Inputs{Running: "6.8.0-147-generic", Installed: []string{"6.8.0-94-generic", "6.8.0-149-generic", "6.8.0-147-generic"},
		DKMS: dkms, Busy: []string{"Write the report"}, Installing: []string{"apt"},
		Repos: []Repo{{Dir: "/p/a", Uncommitted: 2}, {Dir: "/p/b", Unpushed: 3}, {Dir: "/p/c"}}})

	var keys []string

	for _, i := range r.Items {
		keys = append(keys, i.Key)
	}

	if r.Ready || !reflect.DeepEqual(keys, []string{"kernel_waiting", "dkms_missing", "installing", "busy", "uncommitted", "unpushed"}) {
		t.Fatalf("%v %+v", keys, r)
	}

	if r.Items[1].Args["module"] != "nvidia" || !r.Items[1].Blocking {
		t.Errorf("%+v", r.Items[1])
	}

	// Everything built and nothing running: ready, with the new kernel said.
	r = Judge(Inputs{Running: "6.8.0-147-generic", Installed: []string{"6.8.0-147-generic", "6.8.0-149-generic"},
		DKMS: strings.Join(strings.Split(dkms, "\n")[:2], "\n")})
	if !r.Ready || len(r.Items) != 1 || r.Items[0].Key != "kernel_waiting" {
		t.Errorf("%+v", r)
	}

	if r := Judge(Inputs{Running: "6.8.0-149-generic", Installed: []string{"6.8.0-149-generic"}}); !r.Ready || len(r.Items) != 0 {
		t.Errorf("%+v", r)
	}

	if Newest([]string{"6.8.0-94-generic", "6.8.0-149-generic", "6.8.0-100-generic"}) != "6.8.0-149-generic" {
		t.Error("versions are compared as text")
	}
}
