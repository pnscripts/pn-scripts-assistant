package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Where a call would act, and what it can prove it did.
 *
 * Two optional answers a tool can give about one call, both read from the
 * call's own arguments and what came back — never from what the model says
 * it did. The first is what confines work on a project to the project: a
 * tool that can say where it would write is held to the project's folders.
 * The second is the record a task's account is built from: which command ran
 * with which arguments and how it ended, which file was written and whether
 * it was new.
 */

// Touching is a tool that can say where a call would write, and where it
// would run a program.
type Touching interface {
	Touches(args json.RawMessage) (writes []string, runsIn []string)
}

// TouchesOf is a call's writes and working folders, and whether the tool
// said at all.
func TouchesOf(t Tool, args json.RawMessage) (writes, runsIn []string, said bool) {
	if touching, ok := t.(Touching); ok {
		writes, runsIn = touching.Touches(args)

		return writes, runsIn, true
	}

	return nil, nil, false
}

// Evidencing is a tool whose evidence follows from its arguments and output.
type Evidencing interface {
	Evidence(args json.RawMessage, output string, err error) []store.Evidence
}

// Showing is a tool whose evidence only it knows — an engine's run, whose
// commands and diagnostics are not in the text it hands the model.
type Showing interface {
	ExecuteShowing(ctx context.Context, args json.RawMessage) (string, []store.Evidence, error)
}

/*
 * Perform runs a call and gathers its evidence.
 *
 * Every place that carries out a call uses this — the loop, and approving a
 * call that waited — so a command run after its owner approved it leaves the
 * same record as one that ran on a standing yes.
 */
func Perform(ctx context.Context, t Tool, args json.RawMessage) (string, []store.Evidence, error) {
	if showing, ok := t.(Showing); ok {
		return showing.ExecuteShowing(ctx, args)
	}

	out, err := t.Execute(ctx, args)

	if evidencing, ok := t.(Evidencing); ok {
		return out, evidencing.Evidence(args, out, err), err
	}

	return out, nil, err
}

/*
 * FileFree is a tool that changes something other than files — an
 * integration's own effects, which its grants govern. Work on a project may
 * use it without saying where it writes; any other tool that changes
 * something must say, or it is not used on a project at all.
 */
type FileFree interface {
	WritesNoFiles()
}

// The file tools write where their path says.

func (WriteFile) Touches(raw json.RawMessage) ([]string, []string) { return pathOf(raw), nil }
func (EditFile) Touches(raw json.RawMessage) ([]string, []string)  { return pathOf(raw), nil }

func (WriteDocument) Touches(raw json.RawMessage) ([]string, []string) { return pathOf(raw), nil }
func (EditDocument) Touches(raw json.RawMessage) ([]string, []string)  { return pathOf(raw), nil }

func pathOf(raw json.RawMessage) []string {
	var a struct {
		Path string `json:"path"`
	}

	json.Unmarshal(raw, &a)

	return []string{a.Path}
}

func (WriteFile) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	return fileEvidence(raw, err, "written")
}

func (EditFile) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	return fileEvidence(raw, err, "edited")
}

func (WriteDocument) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	if err != nil {
		return nil
	}

	path := pathOf(raw)[0]

	return []store.Evidence{{Kind: store.EvidenceDocument, Subject: path, Detail: sizeOf(path), OK: true}}
}

func fileEvidence(raw json.RawMessage, err error, what string) []store.Evidence {
	path := pathOf(raw)[0]

	if err != nil {
		return []store.Evidence{{Kind: store.EvidenceFile, Subject: path, Detail: what + " failed: " + err.Error()}}
	}

	return []store.Evidence{{Kind: store.EvidenceFile, Subject: path, Detail: what + ", " + sizeOf(path), OK: true}}
}

func sizeOf(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "not found afterwards"
	}

	return fmt.Sprintf("%d bytes", info.Size())
}

/*
 * A command runs in its folder, and anything it is handed that looks like a
 * path is somewhere it may act.
 *
 * Not a parser of every program's arguments, and not pretending to be: an
 * absolute path or one starting with ~ among the arguments is taken as a place
 * the command will touch, which is what "rm ~/x" and "cp a /etc/b" are. A
 * command with no folder given runs wherever this program happens to be,
 * which for work on a project is nowhere in particular — so it says so.
 */
func (RunCommand) Touches(raw json.RawMessage) ([]string, []string) {
	var a commandArgs

	json.Unmarshal(raw, &a)

	var writes []string

	home, _ := os.UserHomeDir()

	for i, arg := range a.Argv {
		if i == 0 {
			continue
		}

		// --output=/x and the like, as well as /x on its own.
		if _, value, found := strings.Cut(arg, "="); found && strings.HasPrefix(arg, "-") {
			arg = value
		}

		switch {
		case filepath.IsAbs(arg):
			writes = append(writes, arg)
		case strings.HasPrefix(arg, "~/") && home != "":
			writes = append(writes, filepath.Join(home, arg[2:]))
		case strings.HasPrefix(arg, "~"):
			writes = append(writes, arg)
		case a.Dir != "" && climbs(arg):
			// "../x" and "sub/../../x": where it lands from the folder it
			// runs in, which is what cp and mv will make of it.
			writes = append(writes, filepath.Join(a.Dir, arg))
		}
	}

	return writes, []string{a.Dir}
}

/*
 * Unconfinable is why a command could write where its arguments do not show,
 * when nothing below it can stop that: a shell or an interpreter handed its
 * program as a string. Under the kernel's confinement these are fine — what
 * they write is held to the project either way — so this is only asked when
 * that confinement is not available.
 */
func Unconfinable(t Tool, raw json.RawMessage) string {
	if t.Name() != "run_command" {
		return ""
	}

	var a commandArgs

	json.Unmarshal(raw, &a)

	if len(a.Argv) == 0 {
		return ""
	}

	program := filepath.Base(a.Argv[0])

	inline := map[string][]string{
		"sh": {"-c"}, "bash": {"-c"}, "dash": {"-c"}, "zsh": {"-c"}, "fish": {"-c"},
		"node": {"-e", "--eval", "-p", "--print"}, "perl": {"-e", "-E"}, "ruby": {"-e"},
		"php": {"-r"}, "deno": {"eval"}, "lua": {"-e"},
	}

	flags := inline[program]
	if strings.HasPrefix(program, "python") {
		flags = []string{"-c"}
	}

	for _, arg := range a.Argv[1:] {
		for _, f := range flags {
			if arg == f {
				return program + " " + f + " runs a program written into its argument, and this machine cannot hold what that writes to the project — write it to a file in the project and run that"
			}
		}
	}

	return ""
}

// climbs is a relative path that goes up a folder somewhere in it.
func climbs(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return false
	}

	for _, part := range strings.Split(filepath.ToSlash(arg), "/") {
		if part == ".." {
			return true
		}
	}

	return false
}

var exitedWith = regexp.MustCompile(`^Exited with code (-?\d+)\.`)

// A command's evidence is exactly what ran, where, and how it ended.
func (RunCommand) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	var a commandArgs

	json.Unmarshal(raw, &a)

	e := store.Evidence{Kind: store.EvidenceCommand, Subject: quotedArgv(a.Argv)}

	detail := "exit status 0"

	switch {
	case err != nil:
		detail = "did not complete: " + err.Error()
	case exitedWith.MatchString(out):
		code, _ := strconv.Atoi(exitedWith.FindStringSubmatch(out)[1])
		detail = fmt.Sprintf("exit status %d", code)
	default:
		e.OK = true
	}

	if a.Dir != "" {
		detail += ", in " + a.Dir
	}

	e.Detail = detail + "\n" + lastOf(out, 600)

	return []store.Evidence{e}
}

func quotedArgv(argv []string) string {
	out := make([]string, 0, len(argv))

	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			out = append(out, strconv.Quote(a))

			continue
		}

		out = append(out, a)
	}

	return strings.Join(out, " ")
}

func lastOf(text string, most int) string {
	text = strings.TrimSpace(text)

	if len(text) <= most {
		return text
	}

	return "…" + text[len(text)-most:]
}

// Godot's own build tool runs in its project and writes where it exports.
func (GodotBuild) Touches(raw json.RawMessage) ([]string, []string) {
	var a struct {
		Project string `json:"project"`
		Into    string `json:"into"`
	}

	json.Unmarshal(raw, &a)

	var writes []string

	if a.Into != "" {
		writes = append(writes, a.Into)
	}

	return writes, []string{a.Project}
}

// Where something was read is a source, and research is judged on its sources.

func (FetchURL) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	return sourceEvidence(raw, err)
}

func (ReadAPage) Evidence(raw json.RawMessage, out string, err error) []store.Evidence {
	return sourceEvidence(raw, err)
}

func sourceEvidence(raw json.RawMessage, err error) []store.Evidence {
	var a struct {
		URL string `json:"url"`
	}

	json.Unmarshal(raw, &a)

	if err != nil || a.URL == "" {
		return nil
	}

	return []store.Evidence{{Kind: store.EvidenceSource, Subject: a.URL, Detail: "read", OK: true}}
}
