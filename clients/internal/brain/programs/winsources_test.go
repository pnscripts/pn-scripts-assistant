package programs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

/*
 * The Windows sources, read on any system: the registry is answered from a
 * .reg export (the format regedit and `reg export` write), the Start menu
 * is a folder of shortcuts built here byte by byte from [MS-SHLLINK], and
 * Get-AppxPackage is answered with what it prints. The same sources are
 * read for real by winsources_windows_test.go on a Windows computer (CI).
 */

// regExport reads a .reg export into the entries under each key: what the
// registry reader hands the scanner.
func regExport(t *testing.T, text string) map[string][]RegEntry {
	t.Helper()

	out := map[string][]RegEntry{}

	var (
		cur    *RegEntry
		parent string
	)

	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "Windows Registry Editor"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			full := strings.Trim(line, "[]")
			full = strings.Replace(full, "HKEY_LOCAL_MACHINE", "HKLM", 1)
			full = strings.Replace(full, "HKEY_CURRENT_USER", "HKCU", 1)
			i := strings.LastIndex(full, `\`)
			parent = full[:i]
			out[parent] = append(out[parent], RegEntry{Key: full[i+1:], Values: map[string]any{}})
			cur = &out[parent][len(out[parent])-1]
		case cur != nil:
			name, value, ok := strings.Cut(line, "=")
			if !ok {
				t.Fatalf("not a value line: %q", line)
			}

			name = strings.Trim(name, `"`)

			switch {
			case strings.HasPrefix(value, `"`):
				s, err := strconv.Unquote(strings.ReplaceAll(value, `\\`, `\\`))
				if err != nil {
					t.Fatalf("value %q: %v", value, err)
				}

				cur.Values[name] = s
			case strings.HasPrefix(value, "dword:"):
				n, err := strconv.ParseUint(strings.TrimPrefix(value, "dword:"), 16, 32)
				if err != nil {
					t.Fatal(err)
				}

				cur.Values[name] = n
			}
		}
	}

	return out
}

// A trimmed export of a real Windows 11 machine's three uninstall keys,
// with the kinds of entry the Settings list leaves out.
const uninstallExport = `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\7-Zip]
"DisplayName"="7-Zip 24.09 (x64)"
"DisplayVersion"="24.09"
"Publisher"="Igor Pavlov"
"InstallLocation"="C:\\Program Files\\7-Zip\\"
"UninstallString"="\"C:\\Program Files\\7-Zip\\Uninstall.exe\""
"NoModify"=dword:00000001

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{90160000-008C-0000-1000-0000000FF1CE}]
"DisplayName"="Office 16 Click-to-Run Extensibility Component"
"DisplayVersion"="16.0.17928.20114"
"SystemComponent"=dword:00000001
"WindowsInstaller"=dword:00000001

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{3C3901C5-3455-3E0A-A214-0B093A5070A6}]
"DisplayName"="Microsoft Visual C++ 2022 X64 Minimum Runtime - 14.40.33810"
"DisplayVersion"="14.40.33810"
"WindowsInstaller"=dword:00000001

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\KB5034441]
"DisplayName"="Security Update for Windows (KB5034441)"
"ParentKeyName"="OperatingSystem"
"ReleaseType"="Security Update"
"UninstallString"="wusa.exe /uninstall /kb:5034441"

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Connection Manager]
"SystemComponent"=dword:00000001

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\NoName]
"UninstallString"="C:\\nothing.exe"

[HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Leftover]
"DisplayName"="A program that left only its name"

[HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Notepad++]
"DisplayName"="Notepad++ (32-bit x86)"
"DisplayVersion"="8.7.1"
"UninstallString"="C:\\Program Files (x86)\\Notepad++\\uninstall.exe"
"ReleaseType"=""

[HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\7-Zip]
"DisplayName"="7-Zip 24.09 (x64)"
"DisplayVersion"="24.09"
"UninstallString"="C:\\other.exe"

[HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall\Discord]
"DisplayName"="Discord"
"DisplayVersion"="1.0.9171"
"InstallLocation"="C:\\Users\\you\\AppData\\Local\\Discord"
"UninstallString"="\"C:\\Users\\you\\AppData\\Local\\Discord\\Update.exe\" --uninstall"

[HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall\Hotfix-1]
"DisplayName"="Hotfix for something"
"ReleaseType"="Hotfix"
"UninstallString"="x.exe"
`

func fakeRegistry(keys map[string][]RegEntry) func(string) ([]RegEntry, error) {
	return func(key string) ([]RegEntry, error) {
		if es, ok := keys[key]; ok {
			return es, nil
		}

		return nil, os.ErrNotExist
	}
}

func TestInstalledProgramsAreReadAsSettingsListsThem(t *testing.T) {
	s := Scanner{Windows: true, Registry: fakeRegistry(regExport(t, uninstallExport))}

	got, err := s.Scan(context.Background(), Win)
	if err != nil {
		t.Fatal(err)
	}

	want := []Program{
		{Kind: Win, ID: "Discord", Name: "Discord", Version: "1.0.9171", Path: `C:\Users\you\AppData\Local\Discord`},
		{Kind: Win, ID: "7-Zip", Name: "7-Zip 24.09 (x64)", Version: "24.09", Path: `C:\Program Files\7-Zip\`},
		{Kind: Win, ID: "{3C3901C5-3455-3E0A-A214-0B093A5070A6}", Name: "Microsoft Visual C++ 2022 X64 Minimum Runtime - 14.40.33810", Version: "14.40.33810"},
		{Kind: Win, ID: "Notepad++", Name: "Notepad++ (32-bit x86)", Version: "8.7.1"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("installed programs:\n got %+v\nwant %+v", got, want)
	}
}

func TestAMissingUninstallKeyIsNoProgramsThereButAFailingOneIsAnError(t *testing.T) {
	// A fresh user has no HKCU key at all.
	s := Scanner{Windows: true, Registry: fakeRegistry(map[string][]RegEntry{})}

	got, err := s.Scan(context.Background(), Win)
	if err != nil || len(got) != 0 {
		t.Fatalf("no keys: %v %v", got, err)
	}

	// Refused: an error, never "everything removed".
	s.Registry = func(string) ([]RegEntry, error) { return nil, errors.New("access is denied") }

	if _, err := s.Scan(context.Background(), Win); err == nil {
		t.Fatal("a registry that cannot be read must be an error")
	}

	// Not Windows: the source is not here.
	if _, err := (Scanner{}).Scan(context.Background(), Win); !errors.Is(err, ErrNotHere) {
		t.Fatalf("no registry: %v", err)
	}
}

func TestTheUninstallKeysAreWatchedNotPolled(t *testing.T) {
	s := Scanner{Windows: true, Registry: fakeRegistry(nil)}

	got := s.Targets(context.Background(), Win)
	if len(got) != len(uninstallKeys) {
		t.Fatalf("targets %+v", got)
	}

	for i, tg := range got {
		if !tg.Registry || !tg.Tree || tg.Dir != uninstallKeys[i] {
			t.Fatalf("target %d: %+v", i, tg)
		}
	}
}

// ---- shortcuts ----

// lnk builds a shell link the way Windows writes one ([MS-SHLLINK] 2.1 to
// 2.5): a header, an optional item id list, the link info with the target's
// local base path (ANSI, and Unicode when unicode is set), the string data
// (a description and the arguments), and the environment block when env is
// given, ended by a terminal block.
func lnk(target string, unicode bool, env string) []byte {
	var b bytes.Buffer

	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }

	const (
		hasIDList   = 0x01
		hasLinkInfo = 0x02
		hasName     = 0x04
		hasArgs     = 0x20
		isUnicode   = 0x80
		hasEnv      = 0x200
	)

	flags := uint32(hasIDList | hasName | hasArgs | isUnicode)
	if target != "" {
		flags |= hasLinkInfo
	}

	if env != "" {
		flags |= hasEnv
	}

	// Header: size, CLSID, flags, attributes, three times, size, icon,
	// show command, hot key, reserved.
	le(uint32(0x4C))
	b.Write([]byte{0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46})
	le(flags)
	le(uint32(0x20))
	b.Write(make([]byte, 24))
	le(uint32(0))
	le(uint32(0))
	le(uint32(1))
	le(uint16(0))
	b.Write(make([]byte, 10))

	// An item id list with one opaque item and its terminator.
	item := []byte{0x14, 0x00, 0x1F, 0x50, 0xE0, 0x4F, 0xD0, 0x20, 0xEA, 0x3A, 0x69, 0x10, 0xA2, 0xD8, 0x08, 0x00, 0x2B, 0x30, 0x30, 0x9D}
	le(uint16(len(item) + 2))
	b.Write(item)
	le(uint16(0))

	if target != "" {
		ansi := append([]byte(target), 0)
		volume := []byte{0x10, 0, 0, 0, 3, 0, 0, 0, 0x11, 0x22, 0x33, 0x44, 0x10, 0, 0, 0, 0}

		header := 0x1C
		if unicode {
			header = 0x24
		}

		volumeAt := header
		baseAt := volumeAt + len(volume)
		suffixAt := baseAt + len(ansi)
		next := suffixAt + 1

		var wide []byte
		if unicode {
			for _, u := range utf16.Encode([]rune(target + "\x00")) {
				wide = binary.LittleEndian.AppendUint16(wide, u)
			}
		}

		wideAt := next
		wideSuffixAt := wideAt + len(wide)
		size := wideSuffixAt
		if unicode {
			size += 2
		}

		le(uint32(size))
		le(uint32(header))
		le(uint32(1)) // VolumeIDAndLocalBasePath
		le(uint32(volumeAt))
		le(uint32(baseAt))
		le(uint32(0))
		le(uint32(suffixAt))

		if unicode {
			le(uint32(wideAt))
			le(uint32(wideSuffixAt))
		}

		b.Write(volume)
		b.Write(ansi)
		b.WriteByte(0)

		if unicode {
			b.Write(wide)
			le(uint16(0))
		}
	}

	str := func(s string) {
		u := utf16.Encode([]rune(s))
		le(uint16(len(u)))

		for _, c := range u {
			le(c)
		}
	}

	str("Starts the program")
	str("--new-window")

	if env != "" {
		le(uint32(0x314))
		le(uint32(0xA0000001))

		ansi := make([]byte, 260)
		copy(ansi, env)
		b.Write(ansi)

		wide := make([]byte, 520)
		for i, u := range utf16.Encode([]rune(env)) {
			binary.LittleEndian.PutUint16(wide[2*i:], u)
		}

		b.Write(wide)
	}

	le(uint32(0))

	return b.Bytes()
}

func TestAShortcutsTargetIsRead(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want string
	}{
		{"unicode path", lnk(`C:\Program Files\Mozilla Firefox\firefox.exe`, true, ""), `C:\Program Files\Mozilla Firefox\firefox.exe`},
		{"unicode beyond ANSI", lnk(`C:\Програми\Редактор\редактор.exe`, true, ""), `C:\Програми\Редактор\редактор.exe`},
		{"ANSI path only", lnk(`C:\Windows\notepad.exe`, false, ""), `C:\Windows\notepad.exe`},
		{"environment block wins", lnk(`C:\Windows\System32\cmd.exe`, true, `%windir%\system32\cmd.exe`), `%windir%\system32\cmd.exe`},
		{"only the environment block", lnk("", false, `%ProgramFiles%\Tool\tool.exe`), `%ProgramFiles%\Tool\tool.exe`},
		{"no target at all (an installer's advertised shortcut)", lnk("", false, ""), ""},
	}

	for _, c := range cases {
		got, err := ShortcutTarget(c.b)
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got, err, c.want)
		}
	}
}

func TestABrokenShortcutIsAnErrorNeverAPanic(t *testing.T) {
	good := lnk(`C:\Program Files\App\app.exe`, true, `%ProgramFiles%\App\app.exe`)

	// Every cut short copy, and every copy with one byte changed to 0xFF.
	for n := 0; n < len(good); n++ {
		_, _ = ShortcutTarget(good[:n])

		bad := append([]byte(nil), good...)
		bad[n] = 0xFF
		_, _ = ShortcutTarget(bad)
	}

	if _, err := ShortcutTarget([]byte("not a shortcut at all, but long enough to have a header of its own...........")); err == nil {
		t.Fatal("a file that is not a shortcut must be an error")
	}
}

func TestTheStartMenuIsReadAsTheProgramsItStarts(t *testing.T) {
	root := t.TempDir()
	user := `C:\Users\you\AppData\Roaming\Microsoft\Windows\Start Menu\Programs`
	common := `C:\ProgramData\Microsoft\Windows\Start Menu\Programs`

	put := func(dir, rel string, b []byte) {
		path := filepath.Join(root, filepath.FromSlash(winPath(dir)), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	put(user, "Discord Inc/Discord.lnk", lnk(`C:\Users\you\AppData\Local\Discord\Update.exe`, true, ""))
	put(common, "Firefox.lnk", lnk(`C:\Program Files\Mozilla Firefox\firefox.exe`, true, ""))
	put(common, "7-Zip/7-Zip File Manager.lnk", lnk(`C:\Program Files\7-Zip\7zFM.exe`, true, ""))
	put(common, "7-Zip/7-Zip Help.lnk", lnk(`C:\Program Files\7-Zip\7-zip.chm`, true, ""))
	put(common, "Inno App/Uninstall Inno App.lnk", lnk(`C:\Program Files\Inno App\unins000.exe`, true, ""))
	put(common, "Tool/Uninstall.LNK", lnk(`C:\Program Files\Tool\uninstall.exe`, true, ""))
	put(common, "Microsoft Office/Word.lnk", lnk("", false, "")) // advertised
	put(common, "Website.url", []byte("[InternetShortcut]\r\nURL=https://example.org/\r\n"))
	put(common, "Broken.lnk", []byte("nope"))
	put(common, "Tools/Command Prompt.LNK", lnk(`C:\Windows\System32\cmd.exe`, true, `%windir%\system32\cmd.exe`))
	// The same entry for the user and for everybody: the user's wins.
	put(user, "Firefox.lnk", lnk(`C:\Users\you\AppData\Local\Mozilla Firefox\firefox.exe`, true, ""))

	s := Scanner{Root: root, Windows: true, StartMenus: []string{user, common}}

	got, err := s.Scan(context.Background(), App)
	if err != nil {
		t.Fatal(err)
	}

	at := func(dir, rel string) string {
		return filepath.Join(root, filepath.FromSlash(winPath(dir)), filepath.FromSlash(rel))
	}

	want := []Program{
		{Kind: App, ID: "Discord Inc/Discord.lnk", Name: "Discord", Path: at(user, "Discord Inc/Discord.lnk"), Exec: `C:\Users\you\AppData\Local\Discord\Update.exe`},
		{Kind: App, ID: "Firefox.lnk", Name: "Firefox", Path: at(user, "Firefox.lnk"), Exec: `C:\Users\you\AppData\Local\Mozilla Firefox\firefox.exe`},
		{Kind: App, ID: "7-Zip/7-Zip File Manager.lnk", Name: "7-Zip File Manager", Path: at(common, "7-Zip/7-Zip File Manager.lnk"), Exec: `C:\Program Files\7-Zip\7zFM.exe`},
		{Kind: App, ID: "Microsoft Office/Word.lnk", Name: "Word", Path: at(common, "Microsoft Office/Word.lnk")},
		{Kind: App, ID: "Tools/Command Prompt.LNK", Name: "Command Prompt", Path: at(common, "Tools/Command Prompt.LNK"), Exec: `%windir%\system32\cmd.exe`},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Start menu:\n got %+v\nwant %+v", got, want)
	}

	targets := s.Targets(context.Background(), App)
	if len(targets) != 2 || !targets[0].Tree || targets[0].Registry || targets[0].Dir != at(user, "") {
		t.Fatalf("Start menu targets: %+v", targets)
	}
}

// winPath turns C:\a\b into C/a/b, the shape the tests lay a Windows
// folder out in under a temporary root on any system.
func winPath(p string) string {
	return strings.ReplaceAll(strings.Replace(p, ":", "", 1), `\`, "/")
}

// ---- app packages ----

// What Get-AppxPackage, through the scanner's own command, prints on a
// Windows 11 computer: apps, frameworks, a resource package, a system app.
const appxOutput = `[{"Name":"Microsoft.WindowsCalculator","PackageFamilyName":"Microsoft.WindowsCalculator_8wekyb3d8bbwe","Version":"11.2405.2.0","InstallLocation":"C:\\Program Files\\WindowsApps\\Microsoft.WindowsCalculator_11.2405.2.0_x64__8wekyb3d8bbwe","IsFramework":false,"IsResourcePackage":false,"SignatureKind":"Store"},` +
	`{"Name":"Microsoft.VCLibs.140.00","PackageFamilyName":"Microsoft.VCLibs.140.00_8wekyb3d8bbwe","Version":"14.0.33519.0","InstallLocation":"C:\\Program Files\\WindowsApps\\Microsoft.VCLibs.140.00_14.0.33519.0_x64__8wekyb3d8bbwe","IsFramework":true,"IsResourcePackage":false,"SignatureKind":"Store"},` +
	`{"Name":"Microsoft.LanguageExperiencePackbg-BG","PackageFamilyName":"x_8wekyb3d8bbwe","Version":"1.0.0.0","InstallLocation":"C:\\x","IsFramework":false,"IsResourcePackage":true,"SignatureKind":"Store"},` +
	`{"Name":"Microsoft.Windows.ShellExperienceHost","PackageFamilyName":"Microsoft.Windows.ShellExperienceHost_cw5n1h2txyewy","Version":"10.0.22621.1","InstallLocation":"C:\\Windows\\SystemApps\\ShellExperienceHost_cw5n1h2txyewy","IsFramework":false,"IsResourcePackage":false,"SignatureKind":"System"},` +
	`{"Name":"PNScripts.Sample","PackageFamilyName":"PNScripts.Sample_abc123","Version":"2.1.0.0","InstallLocation":"C:\\Program Files\\WindowsApps\\PNScripts.Sample_2.1.0.0_x64__abc123","IsFramework":false,"IsResourcePackage":false,"SignatureKind":"Developer"},` +
	`{"Name":"5319275A.WhatsAppDesktop","PackageFamilyName":"5319275A.WhatsAppDesktop_cv1g1gvanyjgm","Version":"2.2440.9.0","InstallLocation":"","IsFramework":false,"IsResourcePackage":false,"SignatureKind":"Store"}]`

func TestAppPackagesAreReadWithTheirOwnNames(t *testing.T) {
	root := t.TempDir()

	manifest := func(loc, name string) {
		dir := filepath.Join(root, filepath.FromSlash(winPath(loc)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Properties><DisplayName>%s</DisplayName></Properties></Package>`, name)
		if err := os.WriteFile(filepath.Join(dir, "AppxManifest.xml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manifest(`C:\Program Files\WindowsApps\Microsoft.WindowsCalculator_11.2405.2.0_x64__8wekyb3d8bbwe`, "ms-resource:AppStoreName")
	manifest(`C:\Program Files\WindowsApps\PNScripts.Sample_2.1.0.0_x64__abc123`, "Sample Studio")

	var asked []string

	s := Scanner{Root: root, Windows: true, Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		asked = append([]string{name}, args...)

		return []byte(appxOutput), nil
	}}

	got, err := s.Scan(context.Background(), Msix)
	if err != nil {
		t.Fatal(err)
	}

	want := []Program{
		{Kind: Msix, ID: "Microsoft.WindowsCalculator_8wekyb3d8bbwe", Name: "Windows Calculator", Version: "11.2405.2.0", Path: `C:\Program Files\WindowsApps\Microsoft.WindowsCalculator_11.2405.2.0_x64__8wekyb3d8bbwe`},
		{Kind: Msix, ID: "PNScripts.Sample_abc123", Name: "Sample Studio", Version: "2.1.0.0", Path: `C:\Program Files\WindowsApps\PNScripts.Sample_2.1.0.0_x64__abc123`},
		{Kind: Msix, ID: "5319275A.WhatsAppDesktop_cv1g1gvanyjgm", Name: "Whats App Desktop", Version: "2.2440.9.0"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("app packages:\n got %+v\nwant %+v", got, want)
	}

	// Windows PowerShell, no profile, nothing asked of a person.
	if len(asked) < 4 || asked[0] != "powershell.exe" || asked[1] != "-NoProfile" || asked[2] != "-NonInteractive" || asked[3] != "-Command" {
		t.Fatalf("command %q", asked)
	}
}

func TestOneAppPackageOrNoneIsReadToo(t *testing.T) {
	for out, want := range map[string]int{
		"":   0,
		"[]": 0,
		`{"Name":"A.B","PackageFamilyName":"A.B_1","Version":"1.0.0.0","SignatureKind":"Store"}`: 1,
	} {
		s := Scanner{Windows: true, Run: func(context.Context, string, ...string) ([]byte, error) { return []byte(out), nil }}

		got, err := s.Scan(context.Background(), Msix)
		if err != nil || len(got) != want {
			t.Errorf("%q: %v %v", out, got, err)
		}
	}

	s := Scanner{Windows: true, Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Get-AppxPackage: failed"), nil
	}}
	if _, err := s.Scan(context.Background(), Msix); err == nil {
		t.Fatal("output that is not the list must be an error")
	}

	s.Run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, &os.PathError{Op: "exec", Err: os.ErrNotExist}
	}
	if _, err := s.Scan(context.Background(), Msix); err == nil {
		t.Fatal("a failing PowerShell must be an error, never an empty list")
	}
}

func TestAppPackagesAreWatchedThroughTheUsersPackageList(t *testing.T) {
	s := Scanner{Windows: true, LocalAppData: `C:\Users\you\AppData\Local`}

	got := s.Targets(context.Background(), Msix)
	if len(got) != 2 || !got[0].Registry || got[0].Dir != appxRepository || got[1].Registry || got[1].Dir != filepath.Join(`C:\Users\you\AppData\Local`, "Packages") {
		t.Fatalf("targets %+v", got)
	}
}

func TestWindowsNamesAreReadable(t *testing.T) {
	for in, want := range map[string]string{
		"Microsoft.WindowsCalculator": "Windows Calculator",
		"Microsoft.ZuneMusic":         "Zune Music",
		"Clipchamp.Clipchamp":         "Clipchamp",
		"MSTeams":                     "MSTeams",
		"Microsoft.Paint":             "Paint",
		"x":                           "x",
	} {
		if got := packageName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestSourcesAreKnownAndShownForThisSystem(t *testing.T) {
	for _, k := range AllKinds {
		if !Known(k) {
			t.Errorf("%s not known", k)
		}
	}

	if Known("rpm") || Known("") {
		t.Fatal("unknown kinds are known")
	}

	// This system's first, then those a remembered program came from.
	other := Win
	if Kinds[1] == Win {
		other = Deb
	}

	got := Shown([]Record{{Program: Program{Kind: other}}, {Program: Program{Kind: Kinds[0]}}})
	if !reflect.DeepEqual(got, append(append([]Kind(nil), Kinds...), other)) {
		t.Fatalf("shown %v", got)
	}
}
