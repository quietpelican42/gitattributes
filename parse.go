package gitattributes

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	blank         = " \t\r\n"
	macroPrefix   = "[attr]"
	maxLineLength = 2048
)

// A Warning describes a line that git would report and ignore.
type Warning struct {
	// Source names the file: "global", "info", or the path of the
	// .gitattributes file such as "docs/.gitattributes".
	Source string
	// Line is the 1-based line number within the file.
	Line int
	// Message describes the problem.
	Message string
}

// String formats the warning as "source:line: message".
func (w Warning) String() string {
	return fmt.Sprintf("%s:%d: %s", w.Source, w.Line, w.Message)
}

// State is the kind of value an attribute has for a path.
type State int

const (
	// Unspecified means no rule assigned the attribute, or a rule reset it
	// with "!name".
	Unspecified State = iota
	// Set means the attribute is set ("name").
	Set
	// Unset means the attribute is explicitly unset ("-name").
	Unset
	// Value means the attribute has a string value ("name=value").
	Value
)

// String returns "unspecified", "set", "unset" or "value".
func (s State) String() string {
	switch s {
	case Set:
		return "set"
	case Unset:
		return "unset"
	case Value:
		return "value"
	}
	return "unspecified"
}

// An Attribute is the resolved state of one attribute for one path.
type Attribute struct {
	// Name is the attribute name.
	Name string
	// State is the kind of value.
	State State
	// Value is the string value when State is Value, and empty otherwise.
	Value string
}

// String formats the attribute value the way git check-attr prints it: "set",
// "unset", "unspecified", or the value itself.
func (a Attribute) String() string {
	if a.State == Value {
		return a.Value
	}
	return a.State.String()
}

// attrState is one "name", "-name", "!name" or "name=value" token. The zero
// value of kind is Unspecified, which represents "!name".
type attrState struct {
	name  string
	kind  State
	value string
}

// entry is one parsed line: either a pattern with attributes, or a macro
// definition.
type entry struct {
	macro   string // macro name; empty for pattern lines
	pat     *pattern
	states  []attrState
	isMacro bool
}

// file is a parsed attributes file.
type file struct {
	source  string
	entries []entry
	names   []string // attribute names in the order git first sees them
}

// unquoteC decodes a C-style quoted string that starts at s[0] == '"'. It
// returns the decoded bytes and the remainder after the closing quote. ok is
// false when the quoting is malformed.
func unquoteC(s string) (out string, rest string, ok bool) {
	var b []byte
	i := 1
	for {
		j := strings.IndexAny(s[i:], "\"\\")
		if j < 0 {
			return "", "", false
		}
		b = append(b, s[i:i+j]...)
		i += j
		if s[i] == '"' {
			return string(b), s[i+1:], true
		}
		i++ // backslash
		if i >= len(s) {
			return "", "", false
		}
		c := s[i]
		i++
		switch c {
		case 'a':
			c = '\a'
		case 'b':
			c = '\b'
		case 'f':
			c = '\f'
		case 'n':
			c = '\n'
		case 'r':
			c = '\r'
		case 't':
			c = '\t'
		case 'v':
			c = '\v'
		case '\\', '"':
		case '0', '1', '2', '3':
			if i+1 >= len(s) || s[i] < '0' || s[i] > '7' || s[i+1] < '0' || s[i+1] > '7' {
				return "", "", false
			}
			c = (c-'0')<<6 | (s[i]-'0')<<3 | (s[i+1] - '0')
			i += 2
		default:
			return "", "", false
		}
		b = append(b, c)
	}
}

func validAttrName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '-' || c == '.' || c == '_':
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		default:
			return false
		}
	}
	return true
}

// parseStates parses the attribute tokens of a line. On a bad token it
// returns the token's name for the warning and ok false.
func parseStates(s string) (states []attrState, bad string, ok bool) {
	for {
		s = strings.TrimLeft(s, blank)
		if s == "" {
			return states, "", true
		}
		end := strings.IndexAny(s, blank)
		if end < 0 {
			end = len(s)
		}
		tok := s[:end]
		s = s[end:]
		var st attrState
		body := tok
		switch tok[0] {
		case '-':
			st.kind = Unset
			body = tok[1:]
		case '!':
			st.kind = Unspecified
			body = tok[1:]
		default:
			st.kind = Set
		}
		name := body
		if eq := strings.IndexByte(body, '='); eq >= 0 {
			name = body[:eq]
			if st.kind == Set {
				st.kind = Value
				st.value = body[eq+1:]
			}
		}
		if !validAttrName(name) || strings.HasPrefix(name, "builtin_") {
			return nil, name, false
		}
		st.name = name
		states = append(states, st)
	}
}

// parseFile parses attribute file content. Macro definitions are honored
// only when macroOK is set.
func parseFile(source string, content []byte, macroOK bool) (*file, []Warning) {
	f := &file{source: source}
	var warns []Warning
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	if i := bytes.IndexByte(content, 0); i >= 0 {
		content = content[:i]
	}
	seen := map[string]bool{}
	intern := func(n string) {
		if !seen[n] {
			seen[n] = true
			f.names = append(f.names, n)
		}
	}
	warn := func(line int, format string, args ...any) {
		warns = append(warns, Warning{Source: source, Line: line, Message: fmt.Sprintf(format, args...)})
	}
	text := string(content)
	for lineNo := 1; text != ""; lineNo++ {
		var line string
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			line, text = text, ""
		}
		full := len(line)
		line = strings.TrimLeft(line, blank)
		if line == "" || line[0] == '#' {
			continue
		}
		if full >= maxLineLength {
			warn(lineNo, "ignoring overly long attributes line")
			continue
		}
		var name, states string
		quoted := false
		if line[0] == '"' {
			if n, rest, ok := unquoteC(line); ok {
				name, states, quoted = n, rest, true
			}
		}
		if !quoted {
			// Not a well-formed quoted pattern: the pattern is the
			// first blank-delimited word, quote and all.
			end := strings.IndexAny(line, blank)
			if end < 0 {
				end = len(line)
			}
			name, states = line[:end], line[end:]
		}
		if i := strings.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		isMacro := len(name) > len(macroPrefix) && strings.HasPrefix(name, macroPrefix)
		if isMacro {
			if !macroOK {
				warn(lineNo, "%s not allowed", name)
				continue
			}
			name = name[len(macroPrefix):]
			if !validAttrName(name) || strings.HasPrefix(name, "builtin_") {
				warn(lineNo, "%s is not a valid attribute name", name)
				continue
			}
		}
		sts, bad, ok := parseStates(states)
		if !ok {
			warn(lineNo, "%s is not a valid attribute name", bad)
			continue
		}
		e := entry{states: sts, isMacro: isMacro}
		if isMacro {
			e.macro = name
			intern(name)
		} else {
			p := parsePattern(name)
			if p.negative {
				warn(lineNo, "negative patterns are ignored in git attributes")
				continue
			}
			e.pat = p
		}
		for _, st := range sts {
			intern(st.name)
		}
		f.entries = append(f.entries, e)
	}
	return f, warns
}
