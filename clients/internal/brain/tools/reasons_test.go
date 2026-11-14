package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

/*
 * Every reason a call can be always asked for is said in Bulgarian on the
 * card. The reasons are read from the source: what Consent, AlwaysAsks,
 * Spends, Destructive and goal mode return or append, in every package that
 * defines tools.
 */
func TestEveryReasonIsSaidInBulgarian(t *testing.T) {
	reasons := map[string]string{}

	makers := map[string]bool{"Consent": true, "AlwaysAsks": true, "Spends": true, "destructive": true,
		"windowsDestructive": true, "commandAsks": true, "GoalModeAsks": true}

	for _, dir := range cardPackages {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		fset := token.NewFileSet()

		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}

			file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !makers[fn.Name.Name] {
					continue
				}

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					var lits []ast.Expr

					switch x := n.(type) {
					case *ast.ReturnStmt:
						lits = x.Results
					case *ast.CallExpr:
						if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "append" && len(x.Args) > 1 {
							lits = x.Args[1:]
						}
					}

					for _, l := range lits {
						if lit, ok := l.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							s, _ := strconv.Unquote(lit.Value)
							if s != "" {
								reasons[s] = fset.Position(lit.Pos()).String()
							}
						}
					}

					return true
				})
			}
		}
	}

	if len(reasons) < 25 {
		t.Fatalf("only %d reasons found in the source: the reading is broken", len(reasons))
	}

	for _, c := range []string{AskPaying, AskSending, AskDeleting, AskSystemWide, HiddenCommand, TypingInAnotherProgram} {
		reasons[c] = "a constant"
	}

	for why, where := range reasons {
		if bg := ReasonIn(why, "bg"); !SaysBulgarian(bg) {
			t.Errorf("%s: %q has no Bulgarian", where, why)
		}

		if ReasonIn(why, "en") != why {
			t.Errorf("the English reason changed: %q", why)
		}
	}

	for in, want := range map[string]string{
		"full access for a coding agent; held for your quiet hours": "пълен достъп за агент за програмиране; задържано за тихите ви часове",
		AskPaying + " (" + AskSending + ")":                         "плащане или платена услуга (изпращане или публикуване от ваше име)",
		"sending data to Demo Notes, off this machine":              "изпращане на данни към Demo Notes, извън този компютър",
	} {
		if got := ReasonIn(in, "bg"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
