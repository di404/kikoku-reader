package script

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// call is one function call statement found in script code. Arguments are
// kept as raw tokens: strings without quotes, everything else verbatim.
type call struct {
	name string
	args []arg
}

type arg struct {
	s     string
	str   bool // was a quoted string
	isVar bool // $var / #var: value unknown to us
}

func (a arg) num() (float64, bool) {
	if a.str || a.isVar {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimPrefix(a.s, "@"), 64)
	return f, err == nil
}

func (a arg) rel() bool { return strings.HasPrefix(a.s, "@") && !a.str }

func (c call) arg(i int) arg {
	if i < len(c.args) {
		return c.args[i]
	}
	return arg{}
}

// calls extracts top-level calls in source order. Control flow keywords
// are skipped, so calls inside if/else bodies are still returned; scene
// files only use if for skip handling, which we never take anyway.
func calls(src string) []call {
	var out []call
	l := &lexer{src: src}
	for {
		t := l.next()
		if t.kind == tEOF {
			return out
		}
		if t.kind != tIdent {
			continue
		}
		save := l.pos
		if p := l.next(); p.kind != tPunct || p.s != "(" {
			l.pos = save
			continue
		}
		args, ok := l.args()
		if !ok {
			return out
		}
		switch t.s {
		case "if", "while", "for", "switch", "return":
			continue
		}
		out = append(out, call{name: t.s, args: args})
	}
}

const (
	tEOF = iota
	tIdent
	tString
	tNumber
	tPunct
	tVar
)

type token struct {
	kind int
	s    string
}

type lexer struct {
	src string
	pos int
}

func (l *lexer) next() token {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == '/' && strings.HasPrefix(l.src[l.pos:], "//"):
			if j := strings.IndexByte(l.src[l.pos:], '\n'); j >= 0 {
				l.pos += j
			} else {
				l.pos = len(l.src)
			}
		case c == '/' && strings.HasPrefix(l.src[l.pos:], "/*"):
			if j := strings.Index(l.src[l.pos+2:], "*/"); j >= 0 {
				l.pos += j + 4
			} else {
				l.pos = len(l.src)
			}
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.pos++
		default:
			return l.token()
		}
	}
	return token{kind: tEOF}
}

func identRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || r > 0x7f
}

func (l *lexer) token() token {
	c := l.src[l.pos]
	start := l.pos
	switch {
	case c == '"':
		j := strings.IndexByte(l.src[l.pos+1:], '"')
		if j < 0 {
			l.pos = len(l.src)
			return token{kind: tString, s: l.src[start+1:]}
		}
		l.pos += j + 2
		return token{kind: tString, s: l.src[start+1 : l.pos-1]}
	case c == '$' || c == '#':
		l.pos++
		l.eatIdent()
		return token{kind: tVar, s: l.src[start:l.pos]}
	case c == '@' || c == '-' || (c >= '0' && c <= '9'):
		l.pos++
		for l.pos < len(l.src) {
			d := l.src[l.pos]
			if (d >= '0' && d <= '9') || d == '.' || (d == '-' && l.pos == start+1) {
				l.pos++
				continue
			}
			break
		}
		if l.pos == start+1 && c != '@' && (c < '0' || c > '9') {
			return token{kind: tPunct, s: string(c)}
		}
		return token{kind: tNumber, s: l.src[start:l.pos]}
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	if identRune(r) {
		l.eatIdent()
		return token{kind: tIdent, s: l.src[start:l.pos]}
	}
	l.pos++
	return token{kind: tPunct, s: string(c)}
}

func (l *lexer) eatIdent() {
	for l.pos < len(l.src) {
		r, n := utf8.DecodeRuneInString(l.src[l.pos:])
		if !identRune(r) {
			return
		}
		l.pos += n
	}
}

// args reads a comma separated list up to the matching ')'. A complex
// argument (expression, nested call) collapses to a variable arg.
func (l *lexer) args() ([]arg, bool) {
	var out []arg
	var cur []token
	depth := 0
	emit := func() {
		if len(cur) == 0 {
			return
		}
		if len(cur) == 1 {
			t := cur[0]
			out = append(out, arg{s: t.s, str: t.kind == tString, isVar: t.kind == tVar})
		} else {
			var b strings.Builder
			for _, t := range cur {
				b.WriteString(t.s)
			}
			out = append(out, arg{s: b.String(), isVar: true})
		}
		cur = nil
	}
	for {
		t := l.next()
		switch {
		case t.kind == tEOF:
			return out, false
		case t.kind == tPunct && t.s == "(":
			depth++
		case t.kind == tPunct && t.s == ")":
			if depth == 0 {
				emit()
				return out, true
			}
			depth--
		case t.kind == tPunct && t.s == "," && depth == 0:
			emit()
			continue
		}
		cur = append(cur, t)
	}
}
