package gollard

import (
	"fmt"
	"strings"
)

type Action struct {
	Migration *Migration
	Test      *Test
}

type ParseError struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *ParseError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("parse error in %s at line %d column %d: %s", e.File, e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("parse error at line %d column %d: %s", e.Line, e.Col, e.Msg)
}

func ParseActions(filename, source string) ([]Action, error) {
	p := &parser{src: source, file: filename}
	return p.parseActions()
}

type fieldValue struct {
	isList bool
	text   string
	list   []string
}

type parser struct {
	src  string
	pos  int
	file string
}

func (p *parser) errf(format string, args ...any) error {
	line, col := p.lineCol()
	return &ParseError{File: p.file, Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) lineCol() (int, int) {
	line, col := 1, 1
	limit := min(p.pos, len(p.src))
	for i := range limit {
		if p.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

// skipWS mirrors megaparsec's spaceConsumer: skips ASCII whitespace AND '-'
// (so that SQL line-comment leaders disappear), plus '@@...@@' block comments
// and '@'-line comments.
func (p *parser) skipWS() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' || c == '-':
			p.pos++
		case c == '@' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '@':
			rest := p.src[p.pos+2:]
			end := strings.Index(rest, "@@")
			if end < 0 {
				p.pos = len(p.src)
			} else {
				p.pos += 2 + end + 2
			}
		case c == '@':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *parser) symbol(s string) bool {
	if p.pos+len(s) > len(p.src) || p.src[p.pos:p.pos+len(s)] != s {
		return false
	}
	p.pos += len(s)
	p.skipWS()
	return true
}

func (p *parser) symbolI(s string) bool {
	if p.pos+len(s) > len(p.src) || !strings.EqualFold(p.src[p.pos:p.pos+len(s)], s) {
		return false
	}
	p.pos += len(s)
	p.skipWS()
	return true
}

func (p *parser) atom() string {
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlnum {
			break
		}
		p.pos++
	}
	name := p.src[start:p.pos]
	p.skipWS()
	return name
}

func (p *parser) parseQuotedText() (string, error) {
	if p.pos >= len(p.src) || p.src[p.pos] != '"' {
		return "", p.errf("expected '\"'")
	}
	p.pos++
	// symbol "\"" consumes trailing whitespace before content begins;
	// faithfully mirror that quirk.
	p.skipWS()
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != '"' {
		p.pos++
	}
	if p.pos >= len(p.src) {
		return "", p.errf("unterminated string literal")
	}
	s := p.src[start:p.pos]
	p.pos++
	p.skipWS()
	return s, nil
}

func (p *parser) parseListValue() (fieldValue, error) {
	if !p.symbol("[") {
		return fieldValue{}, p.errf("expected '['")
	}
	var items []string
	if p.peek() != ']' {
		s, err := p.parseQuotedText()
		if err != nil {
			return fieldValue{}, err
		}
		items = append(items, s)
		for p.symbol(",") {
			s, err := p.parseQuotedText()
			if err != nil {
				return fieldValue{}, err
			}
			items = append(items, s)
		}
	}
	if !p.symbol("]") {
		return fieldValue{}, p.errf("expected ']'")
	}
	return fieldValue{isList: true, list: items}, nil
}

func (p *parser) parseFieldValue() (fieldValue, error) {
	switch p.peek() {
	case '"':
		s, err := p.parseQuotedText()
		if err != nil {
			return fieldValue{}, err
		}
		return fieldValue{isList: false, text: s}, nil
	case '[':
		return p.parseListValue()
	default:
		return fieldValue{}, p.errf("expected field value (quoted text or list)")
	}
}

func (p *parser) parseHeaderFields() (map[string]fieldValue, error) {
	fields := map[string]fieldValue{}
	if p.peek() == ';' {
		return fields, nil
	}
	name, val, err := p.parseField()
	if err != nil {
		return nil, err
	}
	fields[name] = val
	for p.symbol(",") {
		name, val, err = p.parseField()
		if err != nil {
			return nil, err
		}
		fields[name] = val
	}
	return fields, nil
}

func (p *parser) parseField() (string, fieldValue, error) {
	name := p.atom()
	if name == "" {
		return "", fieldValue{}, p.errf("expected field name")
	}
	if !p.symbol(":") {
		return "", fieldValue{}, p.errf("expected ':' after field name %q", name)
	}
	val, err := p.parseFieldValue()
	if err != nil {
		return "", fieldValue{}, err
	}
	return name, val, nil
}

// collectBody consumes characters until "#!" or EOF, returning the body.
// If "#!" is found, it is consumed (and trailing whitespace skipped) so the
// next iteration of parseAction can read the keyword directly.
func (p *parser) collectBody() string {
	start := p.pos
	for p.pos < len(p.src) {
		if p.src[p.pos] == '#' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '!' {
			body := p.src[start:p.pos]
			p.pos += 2
			p.skipWS()
			return body
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) parseActions() ([]Action, error) {
	p.skipWS()
	if !p.symbol("#!") {
		return nil, p.errf("expected '#!' at start of file")
	}
	var actions []Action
	for !p.eof() {
		a, err := p.parseAction()
		if err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func (p *parser) parseAction() (Action, error) {
	save := p.pos
	if p.symbolI("migration") {
		m, err := p.parseMigrationBody()
		if err != nil {
			return Action{}, err
		}
		return Action{Migration: m}, nil
	}
	p.pos = save
	if p.symbolI("test") {
		t, err := p.parseTestBody()
		if err != nil {
			return Action{}, err
		}
		return Action{Test: t}, nil
	}
	return Action{}, p.errf("expected 'migration' or 'test' keyword")
}

// consumeSemi consumes the header-terminating ';' and skips only genuine
// whitespace afterward (spaces, tabs, newlines). Unlike symbol(";"), it does
// NOT call skipWS(), which would eat leading '--' SQL comment markers and strip
// the first comment line from the body before Postgres sees it.
func (p *parser) consumeSemi() bool {
	if p.peek() != ';' {
		return false
	}
	p.pos++
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
			p.pos++
		} else {
			break
		}
	}
	return true
}

func (p *parser) parseMigrationBody() (*Migration, error) {
	fields, err := p.parseHeaderFields()
	if err != nil {
		return nil, err
	}
	if !p.consumeSemi() {
		return nil, p.errf("expected ';' to terminate migration header")
	}
	name, err := requireText(fields, "name")
	if err != nil {
		return nil, err
	}
	desc, err := requireText(fields, "description")
	if err != nil {
		return nil, err
	}
	var requires []MigrationID
	if v, ok := fields["requires"]; ok {
		if v.isList {
			requires = make([]MigrationID, len(v.list))
			for i, r := range v.list {
				requires[i] = MigrationID(r)
			}
		} else {
			requires = []MigrationID{MigrationID(v.text)}
		}
	}

	body := trimRightHeaderWhitespace(p.collectBody())

	return &Migration{
		Name:        MigrationID(name),
		Description: desc,
		Requires:    requires,
		Checksum:    HashScript(body),
		Script:      body,
	}, nil
}

func (p *parser) parseTestBody() (*Test, error) {
	fields, err := p.parseHeaderFields()
	if err != nil {
		return nil, err
	}
	if !p.consumeSemi() {
		return nil, p.errf("expected ';' to terminate test header")
	}
	name, err := requireText(fields, "name")
	if err != nil {
		return nil, err
	}
	desc, err := requireText(fields, "description")
	if err != nil {
		return nil, err
	}
	body := p.collectBody()
	return &Test{
		Name:        TestID(name),
		Description: desc,
		Script:      body,
	}, nil
}

func requireText(fields map[string]fieldValue, key string) (string, error) {
	v, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("the %s field was not provided in the header", key)
	}
	if v.isList {
		return "", fmt.Errorf("the %s field cannot be a list", key)
	}
	return v.text, nil
}

// trimRightHeaderWhitespace mirrors the Haskell `dropWhileEnd isWhiteSpace`
// where whitespace = '\n' | '\r' | ' ' | '-'.
func trimRightHeaderWhitespace(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '-'
	})
}
