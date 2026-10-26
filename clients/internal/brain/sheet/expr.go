package sheet

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

/*
 * Formulas for a new column: numbers, column names, + - * / and brackets,
 * and four functions — round(x, n), abs(x), min(a, b), max(a, b). A
 * column name is written in double quotes, or plainly when it is one word.
 * "20%" is a fifth. Nothing else: a formula can only do arithmetic on the
 * row it is given, exactly.
 */

// Expr is a parsed formula.
type Expr struct {
	node node
	cols []string
}

type node interface {
	eval(func(string) (*big.Rat, bool)) (*big.Rat, error)
}

type num struct{ v *big.Rat }

type col struct{ name string }

type unary struct{ x node }

type binary struct {
	op   byte
	a, b node
}

type call struct {
	fn   string
	args []node
}

// ErrNotANumber is a formula that met a cell that is not a number.
var ErrNotANumber = errors.New("a cell is not a number")

func (n num) eval(func(string) (*big.Rat, bool)) (*big.Rat, error) { return new(big.Rat).Set(n.v), nil }

func (c col) eval(get func(string) (*big.Rat, bool)) (*big.Rat, error) {
	v, ok := get(c.name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotANumber, c.name)
	}

	return v, nil
}

func (u unary) eval(get func(string) (*big.Rat, bool)) (*big.Rat, error) {
	v, err := u.x.eval(get)
	if err != nil {
		return nil, err
	}

	return v.Neg(v), nil
}

func (b binary) eval(get func(string) (*big.Rat, bool)) (*big.Rat, error) {
	x, err := b.a.eval(get)
	if err != nil {
		return nil, err
	}

	y, err := b.b.eval(get)
	if err != nil {
		return nil, err
	}

	switch b.op {
	case '+':
		return x.Add(x, y), nil
	case '-':
		return x.Sub(x, y), nil
	case '*':
		return x.Mul(x, y), nil
	case '/':
		if y.Sign() == 0 {
			return nil, errors.New("division by zero")
		}

		return x.Quo(x, y), nil
	}

	return nil, fmt.Errorf("unknown operation %c", b.op)
}

func (c call) eval(get func(string) (*big.Rat, bool)) (*big.Rat, error) {
	var args []*big.Rat

	for _, a := range c.args {
		v, err := a.eval(get)
		if err != nil {
			return nil, err
		}

		args = append(args, v)
	}

	switch c.fn {
	case "abs":
		return args[0].Abs(args[0]), nil
	case "min":
		if args[0].Cmp(args[1]) <= 0 {
			return args[0], nil
		}

		return args[1], nil
	case "max":
		if args[0].Cmp(args[1]) >= 0 {
			return args[0], nil
		}

		return args[1], nil
	case "round":
		places := 0
		if len(args) > 1 {
			p, _ := args[1].Float64()
			places = int(p)
		}

		return Round(args[0], places), nil
	}

	return nil, fmt.Errorf("unknown function %s", c.fn)
}

// Round is a number rounded half away from zero to places decimals, as
// people and tax offices round.
func Round(r *big.Rat, places int) *big.Rat {
	if places < 0 {
		places = 0
	}

	if places > 8 {
		places = 8
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	x := new(big.Rat).Mul(r, new(big.Rat).SetInt(scale))

	n, d := new(big.Int).Set(x.Num()), x.Denom()
	neg := n.Sign() < 0
	n.Abs(n)

	q, m := new(big.Int).QuoRem(n, d, new(big.Int))
	if new(big.Int).Mul(m, big.NewInt(2)).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}

	if neg {
		q.Neg(q)
	}

	return new(big.Rat).SetFrac(q, scale)
}

var arity = map[string][2]int{"abs": {1, 1}, "min": {2, 2}, "max": {2, 2}, "round": {1, 2}}

type parser struct {
	s    []rune
	at   int
	cols []string
}

// Parse reads a formula.
func Parse(s string) (*Expr, error) {
	p := &parser{s: []rune(strings.TrimSpace(s))}
	if len(p.s) == 0 {
		return nil, errors.New("the formula is empty")
	}

	if len(p.s) > 500 {
		return nil, errors.New("the formula is too long")
	}

	n, err := p.sum()
	if err != nil {
		return nil, err
	}

	p.space()

	if p.at < len(p.s) {
		return nil, fmt.Errorf("unexpected %q", string(p.s[p.at:]))
	}

	return &Expr{node: n, cols: p.cols}, nil
}

// Columns are the column names a formula uses.
func (e *Expr) Columns() []string { return e.cols }

// Eval works a formula out with a row's numbers.
func (e *Expr) Eval(get func(string) (*big.Rat, bool)) (*big.Rat, error) { return e.node.eval(get) }

func (p *parser) space() {
	for p.at < len(p.s) && unicode.IsSpace(p.s[p.at]) {
		p.at++
	}
}

func (p *parser) peek() rune {
	p.space()

	if p.at < len(p.s) {
		return p.s[p.at]
	}

	return 0
}

func (p *parser) sum() (node, error) {
	left, err := p.product()
	if err != nil {
		return nil, err
	}

	for {
		op := p.peek()
		if op != '+' && op != '-' {
			return left, nil
		}

		p.at++

		right, err := p.product()
		if err != nil {
			return nil, err
		}

		left = binary{byte(op), left, right}
	}
}

func (p *parser) product() (node, error) {
	left, err := p.factor()
	if err != nil {
		return nil, err
	}

	for {
		op := p.peek()
		if op == '×' {
			op = '*'
		}

		if op != '*' && op != '/' {
			return left, nil
		}

		p.at++

		right, err := p.factor()
		if err != nil {
			return nil, err
		}

		left = binary{byte(op), left, right}
	}
}

func (p *parser) factor() (node, error) {
	switch r := p.peek(); {
	case r == '-':
		p.at++

		x, err := p.factor()
		if err != nil {
			return nil, err
		}

		return unary{x}, nil
	case r == '(':
		p.at++

		x, err := p.sum()
		if err != nil {
			return nil, err
		}

		if p.peek() != ')' {
			return nil, errors.New("a bracket is not closed")
		}

		p.at++

		return x, nil
	case r == '"' || r == '\'' || r == '`' || r == '[':
		end := map[rune]rune{'"': '"', '\'': '\'', '`': '`', '[': ']'}[r]
		p.at++
		start := p.at

		for p.at < len(p.s) && p.s[p.at] != end {
			p.at++
		}

		if p.at >= len(p.s) {
			return nil, errors.New("a column name is not closed")
		}

		name := strings.TrimSpace(string(p.s[start:p.at]))
		p.at++
		p.cols = append(p.cols, name)

		return col{name}, nil
	case unicode.IsDigit(r) || r == '.':
		start := p.at

		for p.at < len(p.s) && (unicode.IsDigit(p.s[p.at]) || p.s[p.at] == '.') {
			p.at++
		}

		v, ok := new(big.Rat).SetString(string(p.s[start:p.at]))
		if !ok {
			return nil, fmt.Errorf("%q is not a number", string(p.s[start:p.at]))
		}

		if p.peek() == '%' {
			p.at++
			v.Quo(v, big.NewRat(100, 1))
		}

		return num{v}, nil
	case unicode.IsLetter(r) || r == '_':
		start := p.at

		for p.at < len(p.s) && (unicode.IsLetter(p.s[p.at]) || unicode.IsDigit(p.s[p.at]) || p.s[p.at] == '_') {
			p.at++
		}

		word := string(p.s[start:p.at])

		if a, isFn := arity[strings.ToLower(word)]; isFn && p.peek() == '(' {
			p.at++

			var args []node

			for {
				x, err := p.sum()
				if err != nil {
					return nil, err
				}

				args = append(args, x)

				if p.peek() == ',' || p.peek() == ';' {
					p.at++

					continue
				}

				break
			}

			if p.peek() != ')' {
				return nil, fmt.Errorf("%s( is not closed", word)
			}

			p.at++

			if len(args) < a[0] || len(args) > a[1] {
				return nil, fmt.Errorf("%s takes %d to %d numbers", word, a[0], a[1])
			}

			return call{strings.ToLower(word), args}, nil
		}

		p.cols = append(p.cols, word)

		return col{word}, nil
	case r == 0:
		return nil, errors.New("the formula ends too soon")
	default:
		return nil, fmt.Errorf("unexpected %q", string(r))
	}
}
