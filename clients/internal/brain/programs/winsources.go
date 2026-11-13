package programs

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
)

/*
 * The Windows sources. Everything that decides is here and plain Go, so it
 * is tested on any system; what only Windows has (the registry reader, the
 * events) is in the _windows files and reaches this through the Scanner.
 */

// RegEntry is one subkey of a registry key: its name and its values, a
// string (REG_SZ, REG_EXPAND_SZ unexpanded) or a number (REG_DWORD,
// REG_QWORD) each.
type RegEntry struct {
	Key    string
	Values map[string]any
}

func (e RegEntry) str(name string) string {
	s, _ := e.Values[name].(string)

	return strings.TrimSpace(s)
}

func (e RegEntry) num(name string) uint64 {
	n, _ := e.Values[name].(uint64)

	return n
}

// uninstallKeys are where Windows lists installed programs: the user's
// own first, then the machine's, its 64-bit and its 32-bit view.
var uninstallKeys = []string{
	`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
}

// appxRepository is the user's list of app packages: a subkey per
// installed package version, so an install, update or removal changes it.
const appxRepository = `HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// notPrograms in an uninstall entry's ReleaseType: updates of something
// else, which Settings lists under updates, not apps.
var updateTypes = map[string]bool{"update": true, "hotfix": true, "security update": true, "service pack": true}

/*
 * installedProgram is an uninstall entry as Settings, Apps lists it: one
 * with a name, that is not a part of the system (SystemComponent), not an
 * update of another (ParentKeyName, ReleaseType), and that can be removed
 * (an UninstallString, or Windows Installer's own entry).
 */
func installedProgram(e RegEntry) (Program, bool) {
	name := e.str("DisplayName")

	switch {
	case name == "", e.num("SystemComponent") == 1, e.str("ParentKeyName") != "",
		updateTypes[strings.ToLower(e.str("ReleaseType"))],
		e.str("UninstallString") == "" && e.num("WindowsInstaller") != 1:
		return Program{}, false
	}

	return Program{
		Kind:    Win,
		ID:      e.Key,
		Name:    name,
		Version: e.str("DisplayVersion"),
		Path:    strings.Trim(e.str("InstallLocation"), `"`),
	}, true
}

func (s Scanner) installed() ([]Program, error) {
	if s.Registry == nil {
		return nil, ErrNotHere
	}

	taken := map[string]bool{}

	var ps []Program

	for _, key := range uninstallKeys {
		entries, err := s.Registry(key)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", key, err)
		}

		for _, e := range entries {
			p, ok := installedProgram(e)
			if !ok || taken[p.ID] {
				continue
			}

			taken[p.ID] = true
			ps = append(ps, p)
		}
	}

	return ps, nil
}

// ---- the Start menu ----

// atWin is a Windows path under Root: a test lays C:\a\b out as
// <Root>/C/a/b.
func (s Scanner) atWin(path string) string {
	if s.Root == "" {
		return path
	}

	p := strings.ReplaceAll(strings.Replace(path, ":", "", 1), `\`, "/")

	return filepath.Join(s.Root, filepath.FromSlash(p))
}

// uninstaller is whether a shortcut starts a program's uninstaller, which
// the Start menu lists beside the program but which is not a program.
func uninstaller(target string) bool {
	base := strings.ToLower(target[strings.LastIndexAny(target, `\/`)+1:])

	return strings.HasPrefix(base, "unins") || strings.HasPrefix(base, "uninstall")
}

/*
 * shortcuts are the Start menu's programs: every .lnk in its folders (four
 * deep), the user's folder first. A shortcut to a document, a help file or
 * an uninstaller is not a program; one whose target cannot be read (an
 * installer's advertised shortcut, Office's) is, with no command shown.
 */
func (s Scanner) shortcuts() ([]Program, error) {
	taken := map[string]bool{}

	var ps []Program

	for _, menu := range s.StartMenus {
		dir := s.atWin(menu)

		for _, file := range filesEndingIn(dir, ".lnk", 4) {
			rel, _ := filepath.Rel(dir, file)
			id := filepath.ToSlash(rel)

			if taken[strings.ToLower(id)] {
				continue
			}

			b, err := readSmall(file)
			if err != nil {
				continue
			}

			target, err := ShortcutTarget(b)
			if err != nil {
				continue
			}

			if target != "" && (!strings.EqualFold(filepath.Ext(strings.ReplaceAll(target, `\`, "/")), ".exe") || uninstaller(target)) {
				continue
			}

			taken[strings.ToLower(id)] = true

			name := filepath.Base(file)
			name = name[:len(name)-len(filepath.Ext(name))]

			ps = append(ps, Program{Kind: App, ID: id, Name: name, Path: file, Exec: target})
		}
	}

	return ps, nil
}

// filesEndingIn is every file in dir and its subfolders (depth deep)
// whose name ends in ext, in any case; sorted.
func filesEndingIn(dir, ext string, depth int) []string {
	var out []string

	var walk func(d string, level int)
	walk = func(d string, level int) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}

		for _, e := range entries {
			path := filepath.Join(d, e.Name())

			switch {
			case e.IsDir() && level < depth:
				walk(path, level+1)
			case !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ext):
				out = append(out, path)
			}
		}
	}

	walk(dir, 0)
	sort.Strings(out)

	return out
}

// Shell link flags ([MS-SHLLINK] 2.1.1).
const (
	lnkHasIDList   = 0x01
	lnkHasLinkInfo = 0x02
	lnkIsUnicode   = 0x80
	lnkHasEnv      = 0x200
	lnkStrings     = 0x04 | 0x08 | 0x10 | 0x20 | 0x40 // name, relative path, working dir, arguments, icon
)

var errNotAShortcut = errors.New("not a shortcut")

/*
 * ShortcutTarget reads the program a Windows shortcut (.lnk, [MS-SHLLINK])
 * starts: the environment block's target when there is one (it keeps
 * %ProgramFiles% and the like, as written), else the link info's local
 * path, Unicode when stored. "" for a shortcut that names no path (an
 * installer's advertised shortcut). Every offset is checked: a broken or
 * hostile file is an error, never a panic.
 */
func ShortcutTarget(b []byte) (string, error) {
	if len(b) < 0x4C || binary.LittleEndian.Uint32(b) != 0x4C ||
		binary.LittleEndian.Uint32(b[4:]) != 0x00021401 {
		return "", errNotAShortcut
	}

	flags := binary.LittleEndian.Uint32(b[0x14:])
	at := 0x4C

	if flags&lnkHasIDList != 0 {
		if at+2 > len(b) {
			return "", errNotAShortcut
		}

		at += 2 + int(binary.LittleEndian.Uint16(b[at:]))
	}

	var local string

	if flags&lnkHasLinkInfo != 0 {
		if at+4 > len(b) {
			return "", errNotAShortcut
		}

		size := int(binary.LittleEndian.Uint32(b[at:]))
		if size < 0x1C || at+size > len(b) || at+size < at {
			return "", errNotAShortcut
		}

		local = linkInfoPath(b[at : at+size])
		at += size
	}

	// The string data: skipped, but walked, to reach the extra data.
	for bit := uint32(0x04); bit <= 0x40; bit <<= 1 {
		if flags&bit == 0 {
			continue
		}

		if at+2 > len(b) {
			return "", errNotAShortcut
		}

		n := int(binary.LittleEndian.Uint16(b[at:]))
		if flags&lnkIsUnicode != 0 {
			n *= 2
		}

		at += 2 + n
	}

	if flags&lnkHasEnv != 0 {
		for at+8 <= len(b) {
			size := int(binary.LittleEndian.Uint32(b[at:]))
			if size < 8 || at+size > len(b) {
				break
			}

			if binary.LittleEndian.Uint32(b[at+4:]) == 0xA0000001 && size >= 8+260+520 {
				if env := wideString(b[at+8+260 : at+8+260+520]); env != "" {
					return env, nil
				}

				if env := ansiString(b[at+8 : at+8+260]); env != "" {
					return env, nil
				}
			}

			at += size
		}
	}

	return local, nil
}

// linkInfoPath is the local base path and common path suffix of a link
// info structure ([MS-SHLLINK] 2.3), Unicode when it carries one.
func linkInfoPath(li []byte) string {
	u32 := func(off int) int {
		if off+4 > len(li) {
			return 0
		}

		return int(binary.LittleEndian.Uint32(li[off:]))
	}

	header, flags := u32(4), u32(8)
	if flags&1 == 0 { // no VolumeIDAndLocalBasePath
		return ""
	}

	if header >= 0x24 {
		base, suffix := u32(0x1C), u32(0x20)
		if base > 0 && base < len(li) {
			p := wideString(li[base:])
			if suffix > 0 && suffix < len(li) {
				p += wideString(li[suffix:])
			}

			if p != "" {
				return p
			}
		}
	}

	base, suffix := u32(0x10), u32(0x18)
	if base <= 0 || base >= len(li) {
		return ""
	}

	p := ansiString(li[base:])
	if suffix > 0 && suffix < len(li) {
		p += ansiString(li[suffix:])
	}

	return p
}

// wideString is a NUL-ended UTF-16LE string.
func wideString(b []byte) string {
	var u []uint16

	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}

		u = append(u, c)
	}

	return string(utf16.Decode(u))
}

// ansiString is a NUL-ended string in the system's code page; read as
// Latin-1, which is right for the ASCII paths it nearly always holds.
func ansiString(b []byte) string {
	var r []rune

	for _, c := range b {
		if c == 0 {
			break
		}

		r = append(r, rune(c))
	}

	return string(r)
}

// ---- app packages ----

/*
 * appxCommand lists the user's app packages as JSON (always an array),
 * in UTF-8 whatever the console's code page. Windows PowerShell, which has
 * the Appx module on every Windows 10 and 11.
 */
const appxCommand = `[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); ` +
	`ConvertTo-Json -Compress -InputObject @(Get-AppxPackage | Select-Object Name, PackageFamilyName, ` +
	`@{n='Version';e={[string]$_.Version}}, InstallLocation, IsFramework, IsResourcePackage, ` +
	`@{n='SignatureKind';e={[string]$_.SignatureKind}})`

type appxPackage struct {
	Name              string
	PackageFamilyName string
	Version           string
	InstallLocation   string
	IsFramework       bool
	IsResourcePackage bool
	SignatureKind     string
}

func (s Scanner) appPackages(ctx context.Context) ([]Program, error) {
	if !s.Windows {
		return nil, ErrNotHere
	}

	out, err := s.run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", appxCommand)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNotHere
		}

		return nil, fmt.Errorf("asking Get-AppxPackage: %w", err)
	}

	out = []byte(strings.TrimSpace(strings.TrimPrefix(string(out), "\ufeff")))
	if len(out) == 0 {
		return nil, nil
	}

	var list []appxPackage

	if out[0] == '{' {
		var one appxPackage
		if err := json.Unmarshal(out, &one); err != nil {
			return nil, fmt.Errorf("reading Get-AppxPackage: %w", err)
		}

		list = []appxPackage{one}
	} else if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("reading Get-AppxPackage: %w", err)
	}

	var ps []Program

	for _, a := range list {
		// Frameworks and resource packages serve apps; System ones are
		// parts of Windows.
		if a.IsFramework || a.IsResourcePackage || strings.EqualFold(a.SignatureKind, "System") ||
			a.PackageFamilyName == "" {
			continue
		}

		name := ""
		if a.InstallLocation != "" {
			name = manifestName(s.atWin(filepath.Join(a.InstallLocation, "AppxManifest.xml")))
		}

		if name == "" {
			name = packageName(a.Name)
		}

		ps = append(ps, Program{Kind: Msix, ID: a.PackageFamilyName, Name: name, Version: a.Version, Path: a.InstallLocation})
	}

	return ps, nil
}

// manifestName is a package's display name from its manifest, unless it
// is a resource reference (ms-resource:…) only the package can resolve.
func manifestName(path string) string {
	b, err := readSmall(path)
	if err != nil {
		return ""
	}

	var doc struct {
		DisplayName string `xml:"Properties>DisplayName"`
	}

	if xml.Unmarshal(b, &doc) != nil {
		return ""
	}

	name := strings.TrimSpace(doc.DisplayName)
	if strings.HasPrefix(strings.ToLower(name), "ms-resource:") {
		return ""
	}

	return name
}

// packageName is a readable name from a package's own: the part after the
// publisher's prefix, split where a word begins (WindowsCalculator →
// Windows Calculator).
func packageName(id string) string {
	name := id[strings.LastIndex(id, ".")+1:]
	if name == "" {
		return id
	}

	var b strings.Builder

	r := []rune(name)
	for i, c := range r {
		if i > 0 && unicode.IsUpper(c) && unicode.IsLower(r[i-1]) {
			b.WriteRune(' ')
		}

		b.WriteRune(c)
	}

	return b.String()
}
