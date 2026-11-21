package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// changing is a tool that changes something.
type changing struct{ counted }

func (changing) Risk() tools.Risk                 { return tools.Mutating }
func (changing) Summarize(json.RawMessage) string { return "Write notes.txt" }

/*
 * What another program asked for is put to the person at the desk, with
 * the program's name on the question, whatever is allowed there: the
 * person allowed their own requests, not everybody's.
 */
func TestWhatAnotherProgramAsksForIsAlwaysPutToThePerson(t *testing.T) {
	always := func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	for name, allow := range map[string]func(*Loop){
		"always allowed": func(l *Loop) { l.MayI = always },
		"nothing asks": func(l *Loop) {
			l.MayI = always
			l.NothingAsks = func() bool { return true }
		},
		"everything allowed": func(l *Loop) {
			l.MayI = always
			l.NothingAsks = func() bool { return true }
			l.Unbounded = func() bool { return true }
		},
	} {
		t.Run(name, func(t *testing.T) {
			ran := 0
			loop, db := newLoop(t, changing{counted{name: "write_file", ran: &ran}})
			allow(loop)

			var deeds []Deed
			loop.Did = func(d Deed) { deeds = append(deeds, d) }

			conv, _ := db.NewConversation("t")
			ctx := tools.AskedBy(context.Background(), "claude-code 2.1.0")

			res, err := loop.RunBrief(ctx, conv, askingFor("write_file"), nil, Brief{WithTools: true})
			if err != nil {
				t.Fatal(err)
			}

			if ran != 0 || len(res.Pending) != 1 {
				t.Fatalf("ran %d times, %d questions", ran, len(res.Pending))
			}

			q, _ := db.Invocation(res.Pending[0].ID)
			if q.AskedBy != "claude-code 2.1.0" {
				t.Errorf("the question does not say who asked: %q", q.AskedBy)
			}

			if !strings.HasPrefix(q.Summary, "Always asked (another program asked for it): ") ||
				!strings.HasPrefix(q.SummaryBG, "Винаги се пита (поискано от друга програма): ") {
				t.Errorf("the card: %q / %q", q.Summary, q.SummaryBG)
			}

			if len(deeds) != 1 || deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Outcome != DeedWaiting {
				t.Errorf("the record: %+v", deeds)
			}
		})
	}
}

// The same request from the desk runs as it always did, and says nobody
// else asked.
func TestTheSameRequestFromTheDeskIsNotAskedTwice(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, changing{counted{name: "write_file", ran: &ran}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("write_file"), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Pending) != 0 {
		t.Fatalf("ran %d times, %d questions", ran, len(res.Pending))
	}

	if len(deeds) != 1 || deeds[0].AskedBy != "" {
		t.Errorf("the record: %+v", deeds)
	}
}

// open is a tool that says other programs may use it without a question.
type open struct{ counted }

func (open) OpenToOtherPrograms() bool { return true }

/*
 * A turn another program asked for runs nothing unasked that does not say
 * it is open to other programs. "Only reads" is not enough: reading a
 * file, the mail or the memory is reading what is the person's, and some
 * tools that count as safe at the desk write (a to-do, a reminder, a
 * hire). So the rule is by name, not by kind: closed unless opened.
 */
func TestATurnForAnotherProgramRunsOnlyWhatIsOpenToOtherPrograms(t *testing.T) {
	readRan, openRan := 0, 0
	loop, db := newLoop(t, counted{name: "read_file", ran: &readRan}, open{counted{name: "list_programs", ran: &openRan}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.NothingAsks = func() bool { return true }
	loop.Unbounded = func() bool { return true }

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	model := &scripted{replies: []llm.Response{{
		Content: "Looking.",
		ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "list_programs", Arguments: json.RawMessage(`{}`)},
			{ID: "c2", Name: "read_file", Arguments: json.RawMessage(`{}`)},
		},
	}}}

	// Named in the brief, as the door will name it.
	res, err := loop.RunBrief(context.Background(), conv, model, nil, Brief{WithTools: true, AskedBy: "Cursor 3.0"})
	if err != nil {
		t.Fatal(err)
	}

	if openRan != 1 {
		t.Errorf("the tool open to other programs ran %d times", openRan)
	}

	if readRan != 0 || len(res.Pending) != 1 || res.Pending[0].Tool != "read_file" {
		t.Fatalf("a tool that is not open to other programs ran %d times, %d questions", readRan, len(res.Pending))
	}

	q, _ := db.Invocation(res.Pending[0].ID)
	if q.AskedBy != "Cursor 3.0" || !strings.HasPrefix(q.Summary, "Always asked (another program asked for it): ") {
		t.Errorf("the question: %+v", q)
	}

	if len(deeds) != 2 || deeds[0].AskedBy != "Cursor 3.0" || deeds[0].Outcome != DeedDone ||
		deeds[1].AskedBy != "Cursor 3.0" || deeds[1].Outcome != DeedWaiting {
		t.Errorf("the record: %+v", deeds)
	}

	// The same safe tool from the desk runs without a question.
	readRan = 0
	conv, _ = db.NewConversation("t2")

	if res, err = loop.RunBrief(context.Background(), conv, askingFor("read_file"), nil, Brief{WithTools: true}); err != nil ||
		readRan != 1 || len(res.Pending) != 0 {
		t.Errorf("from the desk: ran %d times, %d questions (%v)", readRan, len(res.Pending), err)
	}
}

// A tool reached through another tool still knows who asked: the name
// travels with the call.
type asksWho struct {
	open
	heard *string
}

func (a asksWho) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	*a.heard = tools.AskedByOf(ctx)

	return a.open.Execute(ctx, args)
}

func TestAToolIsToldWhoAsked(t *testing.T) {
	ran, heard := 0, ""
	loop, db := newLoop(t, asksWho{open{counted{name: "list_programs", ran: &ran}}, &heard})

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("list_programs"), nil,
		Brief{WithTools: true, AskedBy: "Cursor 3.0"}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 || heard != "Cursor 3.0" {
		t.Errorf("ran %d, heard %q", ran, heard)
	}
}

/*
 * Another program cannot say yes for the person. The tool that carries out
 * what is waiting is the person's own (said out loud at the desk it needs
 * no approval, since the sentence is the approval); a turn another program
 * asked for is refused it under every setting, "everything allowed" too,
 * and is not offered it. Otherwise a program could ask for something, be
 * put to the person, and approve its own question.
 */
func TestAnotherProgramCannotApproveWhatIsWaiting(t *testing.T) {
	for name, allow := range map[string]func(*Loop){
		"as it comes": func(*Loop) {},
		"everything allowed": func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
			l.NothingAsks = func() bool { return true }
			l.Unbounded = func() bool { return true }
		},
	} {
		t.Run(name, func(t *testing.T) {
			ran := 0
			loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})
			allow(loop)

			conv, _ := db.NewConversation("t")
			model := askingFor("decide_waiting")

			res, err := loop.RunBrief(tools.AskedBy(context.Background(), "claude-code 2.1.0"), conv, model, nil, Brief{WithTools: true})
			if err != nil {
				t.Fatal(err)
			}

			if ran != 0 || len(res.Pending) != 0 {
				t.Fatalf("it decided what was waiting (%d times, %d questions)", ran, len(res.Pending))
			}

			// Not offered, and told why when it names it anyway.
			for _, req := range model.seen {
				for _, spec := range req.Tools {
					if spec.Name == "decide_waiting" {
						t.Error("a turn another program asked for was offered decide_waiting")
					}
				}
			}

			msgs, _ := db.History(conv)

			var told string
			for _, m := range model.seen[len(model.seen)-1].Messages {
				if m.Role == llm.RoleTool {
					told += m.Content
				}
			}

			if !strings.Contains(told, "Only the person at this computer decides what is waiting") {
				t.Errorf("the refusal does not say whose decision it is: %q (%d messages kept)", told, len(msgs))
			}
		})
	}

	// From the desk the same tool runs, as it always did.
	ran := 0
	loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})
	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("decide_waiting"), nil, Brief{WithTools: true}); err != nil || ran != 1 {
		t.Errorf("from the desk: ran %d times (%v)", ran, err)
	}

	if len(NotForAnotherProgram) != 1 || NotForAnotherProgram[0] != "decide_waiting" {
		t.Errorf("what another program may never use: %v", NotForAnotherProgram)
	}
}

/*
 * Where a turn for another program could begin (the review of 1.2.0,
 * note 4: "reached only by tests").
 *
 * True, and on purpose: nothing other programs may ask today runs the
 * loop. The rules above are the gate for the day something does, and a
 * gate nobody walks through is easy to leave open when the first caller is
 * written. So the callers are counted here. Today there are two places
 * that set the mark, and neither starts anything: the loop passing its own
 * brief's name on to the tools, and an approved question carrying the name
 * its row has. Nothing builds a Brief with a name in it.
 *
 * Whoever adds the first real caller changes this list, and with it: the
 * paragraph at tools.AskedBy, a test of that caller through the real door
 * (the switch, the record, the card answered in the window only), and the
 * known limits in both languages.
 */
func TestNothingStartsATurnForAnotherProgramYet(t *testing.T) {
	var found []string

	// Every Go file of the program that is not a test, read as the
	// compiler reads it: a name that is spelt another way, a brief built
	// over several lines or with lists inside it, is still found.
	clients := filepath.Join("..", "..", "..")
	files := 0

	err := filepath.WalkDir(clients, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}

		files++

		rel, _ := filepath.Rel(clients, path)
		isTools := strings.HasSuffix(filepath.ToSlash(filepath.Dir(path)), "/brain/tools")

		for _, where := range forAnotherProgram(parsed, isTools) {
			found = append(found, filepath.ToSlash(rel)+": "+where)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if files < 400 {
		t.Fatalf("only %d files of the program were read: the walk is not looking at all of it", files)
	}

	sort.Strings(found)

	// Each place by its file and the function it is in, once for each
	// time it is written there: a second one in the same file shows too.
	want := []string{
		// The loop takes the name from the context when the brief has
		// none, and hands its brief's name on to the tools.
		"internal/brain/agent/agent.go: marks a call, in RunBrief",
		"internal/brain/agent/agent.go: sets a name, in RunBrief",
		// An approved question keeps the name of its row.
		"internal/brain/brain/approvals.go: marks a call, in decide",
	}

	if !reflect.DeepEqual(found, want) {
		t.Errorf("the places that could start or mark a turn for another program changed:\n got %q\nwant %q\n"+
			"A new one is the first way in for other programs to have something done: read the comment above this test.", found, want)
	}

	// And the list a turn for another program is held to is still the
	// short one the tests above prove.
	if !reflect.DeepEqual(NotForAnotherProgram, []string{"decide_waiting"}) {
		t.Errorf("what a turn for another program may never use changed: %v", NotForAnotherProgram)
	}
}

/*
 * forAnotherProgram is every place in one file that marks a call as asked
 * for by another program, builds a brief that names one, or sets that
 * name: "<what>, in <function>", once for each time it is written.
 * isTools says the file is of package tools itself, where the mark is
 * made (its two own functions and the key's type are not counted).
 */
func forAnotherProgram(file *ast.File, isTools bool) []string {
	const toolsPath = "pn-scripts-assistant/internal/brain/tools"

	// How this file names package tools: its own name, another, or a dot
	// (its names used bare, as inside the package itself).
	toolsAs, bare := "", isTools && file.Name.Name == "tools"

	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != toolsPath {
			continue
		}

		switch {
		case imp.Name == nil:
			toolsAs = "tools"
		case imp.Name.Name == ".":
			bare = true
		default:
			toolsAs = imp.Name.Name
		}
	}

	var out []string

	within := "no function"

	look := func(n ast.Node) bool {
		switch at := n.(type) {
		case *ast.TypeSpec:
			// The type of the key the name is kept under.
			if bare && at.Name.Name == "askedByKey" {
				return false
			}

		case *ast.SelectorExpr:
			if x, ok := at.X.(*ast.Ident); ok && toolsAs != "" && x.Name == toolsAs && at.Sel.Name == "AskedBy" {
				out = append(out, "marks a call, in "+within)
			}

		case *ast.Ident:
			// With the package's names used bare: the function by its
			// own name, and the key it keeps the name under.
			if bare && (at.Name == "AskedBy" || at.Name == "askedByKey") {
				out = append(out, "marks a call, in "+within)
			}

		case *ast.CompositeLit:
			brief := false

			switch typed := at.Type.(type) {
			case *ast.Ident:
				brief = typed.Name == "Brief"
			case *ast.SelectorExpr:
				brief = typed.Sel.Name == "Brief"
			}

			if !brief {
				return true
			}

			for _, el := range at.Elts {
				kv, keyed := el.(*ast.KeyValueExpr)
				if !keyed {
					// Fields given by place: any of them may be the name.
					out = append(out, "builds a brief with a name, in "+within)

					break
				}

				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "AskedBy" {
					out = append(out, "builds a brief with a name, in "+within)
				}
			}

		case *ast.AssignStmt:
			for _, left := range at.Lhs {
				if sel, ok := left.(*ast.SelectorExpr); ok && sel.Sel.Name == "AskedBy" {
					out = append(out, "sets a name, in "+within)
				}
			}
		}

		return true
	}

	for _, decl := range file.Decls {
		within = "no function"

		if fn, ok := decl.(*ast.FuncDecl); ok {
			// The two functions that are the mark itself.
			if isTools && fn.Recv == nil && (fn.Name.Name == "AskedBy" || fn.Name.Name == "AskedByOf") {
				continue
			}

			within = fn.Name.Name
		}

		ast.Inspect(decl, look)
	}

	return out
}

/*
 * The watch is tested on what it must not miss: each of these is a way to
 * write the first caller that a search for one spelling would pass.
 */
func TestTheWatchSeesEveryWayToWriteIt(t *testing.T) {
	const head = "package x\nimport (\n\"context\"\n\"pn-scripts-assistant/internal/brain/agent\"\n%s \"pn-scripts-assistant/internal/brain/tools\"\n)\nvar _ = agent.Brief{}\nvar _ context.Context\n"

	for name, c := range map[string]struct {
		src  string
		want []string
	}{
		"the plain call":      {fmt.Sprintf(head, "") + "func f(ctx context.Context) { tools.AskedBy(ctx, \"x\") }", []string{"marks a call, in f"}},
		"under another name":  {fmt.Sprintf(head, "t") + "func f(ctx context.Context) { t.AskedBy(ctx, \"x\") }", []string{"marks a call, in f"}},
		"its names used bare": {fmt.Sprintf(head, ".") + "func f(ctx context.Context) { AskedBy(ctx, \"x\") }", []string{"marks a call, in f"}},
		"kept as a value":     {fmt.Sprintf(head, "") + "var mark = tools.AskedBy", []string{"marks a call, in no function"}},
		"twice in one function": {fmt.Sprintf(head, "") + "func f(ctx context.Context) { tools.AskedBy(ctx, \"x\"); tools.AskedBy(ctx, \"y\") }",
			[]string{"marks a call, in f", "marks a call, in f"}},
		"in a method": {fmt.Sprintf(head, "") + "type l struct{}\nfunc (l) RunForProgram(ctx context.Context, by string) { tools.AskedBy(ctx, by) }",
			[]string{"marks a call, in RunForProgram"}},
		"a brief with lists in it": {fmt.Sprintf(head, "") + "var b = agent.Brief{Only: []string{\"a\"}, Never: []string{\"b\"},\n AskedBy: \"x\"}; var _ = tools.AskedByOf",
			[]string{"builds a brief with a name, in no function"}},
		"a pointer to a brief":  {fmt.Sprintf(head, "") + "func g() any { return &agent.Brief{AskedBy: \"x\"} }; var _ = tools.AskedByOf", []string{"builds a brief with a name, in g"}},
		"fields given by place": {fmt.Sprintf(head, "") + "var b = agent.Brief{\"a\", \"b\"}; var _ = tools.AskedByOf", []string{"builds a brief with a name, in no function"}},
		"a name set afterwards": {fmt.Sprintf(head, "") + "func h(b *agent.Brief, by string) { b.AskedBy = by }; var _ = tools.AskedByOf", []string{"sets a name, in h"}},
		"a brief with no name":  {fmt.Sprintf(head, "") + "var b = agent.Brief{Only: []string{\"a\"}}; var _ = tools.AskedByOf", nil},
		"reading the name only": {fmt.Sprintf(head, "") + "func r(b agent.Brief) bool { return b.AskedBy == \"\" && tools.AskedByOf(nil) == \"\" }", nil},
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "x.go", c.src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, c.src)
		}

		if got := forAnotherProgram(parsed, false); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: saw %q, want %q", name, got, c.want)
		}
	}

	// Inside package tools itself: the mark's own two functions and the
	// key's type are not places, anything else that uses them is.
	inside := "package tools\nimport \"context\"\ntype askedByKey struct{}\n" +
		"func AskedBy(ctx context.Context, by string) context.Context { return context.WithValue(ctx, askedByKey{}, by) }\n" +
		"func AskedByOf(ctx context.Context) string { by, _ := ctx.Value(askedByKey{}).(string); return by }\n"

	for name, c := range map[string]struct {
		more string
		want []string
	}{
		"as it is":                  {"", nil},
		"another function calls it": {"func Sneak(ctx context.Context) context.Context { return AskedBy(ctx, \"x\") }", []string{"marks a call, in Sneak"}},
		"another function sets the key": {"func Sneak(ctx context.Context) context.Context { return context.WithValue(ctx, askedByKey{}, \"x\") }",
			[]string{"marks a call, in Sneak"}},
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "sensitive.go", inside+c.more, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if got := forAnotherProgram(parsed, true); !reflect.DeepEqual(got, c.want) {
			t.Errorf("inside package tools, %s: saw %q, want %q", name, got, c.want)
		}
	}
}
