package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

/*
 * Every approval card is said in both languages, in every branch.
 *
 * The packages that define tools are read as source: each Summarize must
 * hand over to its SummarizeIn, and every sentence inside a SummarizeIn (or
 * a helper it calls that takes the language) must be one of the two halves
 * of a Say, with the Bulgarian half in Bulgarian. A word left in English on one
 * branch fails here, before anybody sees it on a card.
 */
var cardPackages = []string{".", "../mcp", "../skills"}

var formatVerb = regexp.MustCompile(`%[-+# 0-9.\[\]*]*[a-zA-Z]`)

var latinWord = regexp.MustCompile(`[A-Za-z]{2,}`)

func hasWords(s string) bool {
	return latinWord.MatchString(formatVerb.ReplaceAllString(s, ""))
}

func TestEveryCardIsSaidInBothLanguages(t *testing.T) {
	for _, dir := range cardPackages {
		fset := token.NewFileSet()

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		summarize := map[string]token.Pos{}
		summarizeIn := map[string]bool{}
		withLanguage := map[string][]*ast.FuncDecl{}
		var cards []*ast.FuncDecl

		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}

			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				recv := receiverOf(fn)

				switch {
				case fn.Name.Name == "Summarize" && recv != "":
					summarize[recv] = fn.Pos()

					if !handsOver(fn) {
						t.Errorf("%s: %s.Summarize does not return its SummarizeIn(args, \"en\")", fset.Position(fn.Pos()), recv)
					}

				case fn.Name.Name == "SummarizeIn" && recv != "":
					summarizeIn[recv] = true
					cards = append(cards, fn)

				case takesLanguage(fn):
					withLanguage[fn.Name.Name] = append(withLanguage[fn.Name.Name], fn)
				}
			}
		}

		// The cards, and every helper taking the language that a card
		// calls, however deep.
		checked := map[*ast.FuncDecl]bool{}

		for len(cards) > 0 {
			fn := cards[0]
			cards = cards[1:]

			if checked[fn] {
				continue
			}

			checked[fn] = true
			checkWords(t, fset, fn)

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				var called string

				switch f := call.Fun.(type) {
				case *ast.Ident:
					called = f.Name
				case *ast.SelectorExpr:
					called = f.Sel.Name
				}

				cards = append(cards, withLanguage[called]...)

				return true
			})
		}

		for recv, pos := range summarize {
			if !summarizeIn[recv] {
				t.Errorf("%s: %s has a Summarize and no SummarizeIn: its card has no Bulgarian", fset.Position(pos), recv)
			}
		}
	}
}

func receiverOf(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}

	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}

	return ""
}

// handsOver: the body is `return <recv>.SummarizeIn(<args>, "en")`.
func handsOver(fn *ast.FuncDecl) bool {
	if len(fn.Body.List) != 1 {
		return false
	}

	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}

	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "SummarizeIn" {
		return false
	}

	lit, ok := call.Args[1].(*ast.BasicLit)

	return ok && lit.Value == `"en"`
}

func takesLanguage(fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == "lang" {
				return true
			}
		}
	}

	return false
}

// checkWords fails on every string with words in it that is not a half of
// a Say (or a comparison, a case, an index or a lookup).
func checkWords(t *testing.T, fset *token.FileSet, fn *ast.FuncDecl) {
	t.Helper()

	allowed := map[*ast.BasicLit]bool{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if isSay(x.Fun) && len(x.Args) == 3 {
				en, enOK := x.Args[1].(*ast.BasicLit)
				bg, bgOK := x.Args[2].(*ast.BasicLit)

				if enOK {
					allowed[en] = true
				}

				if bgOK {
					allowed[bg] = true

					b, _ := strconv.Unquote(bg.Value)
					if hasWords(b) && !SaysBulgarian(b) {
						t.Errorf("%s: the Bulgarian half is not Bulgarian: %s", fset.Position(bg.Pos()), bg.Value)
					}
				}

				return true
			}

			// Lookups and tests on data: strings.Contains(x, "word") and the like.
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "strings" || pkg.Name == "filepath" || pkg.Name == "path") {
					for _, a := range x.Args {
						if lit, ok := a.(*ast.BasicLit); ok {
							allowed[lit] = true
						}
					}
				}
			}

		case *ast.BinaryExpr:
			if x.Op == token.EQL || x.Op == token.NEQ {
				for _, side := range []ast.Expr{x.X, x.Y} {
					if lit, ok := side.(*ast.BasicLit); ok {
						allowed[lit] = true
					}
				}
			}

		case *ast.CaseClause:
			for _, e := range x.List {
				if lit, ok := e.(*ast.BasicLit); ok {
					allowed[lit] = true
				}
			}

		case *ast.IndexExpr:
			if lit, ok := x.Index.(*ast.BasicLit); ok {
				allowed[lit] = true
			}

		case *ast.Field:
			if x.Tag != nil {
				allowed[x.Tag] = true
			}
		}

		return true
	})

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || allowed[lit] {
			return true
		}

		s, err := strconv.Unquote(lit.Value)
		if err != nil || s == "en" || s == "bg" || !hasWords(s) {
			return true
		}

		t.Errorf("%s: %s says %s in one language only (write it as Say(lang, english, bulgarian))",
			fset.Position(lit.Pos()), fn.Name.Name, lit.Value)

		return true
	})
}

func isSay(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "Say"
	case *ast.SelectorExpr:
		return f.Sel.Name == "Say"
	}

	return false
}
