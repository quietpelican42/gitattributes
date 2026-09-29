package gitattributes

import "strings"

// wildmatch reports whether text matches the glob pattern.
//
// The dialect is the one git uses for attribute patterns: "?" and "*" match
// any run of characters, bracket expressions support ranges, negation with
// "!" or "^" and POSIX classes, and a backslash escapes the next byte. When
// pathname is set, "*", "?" and bracket expressions never match "/", and a
// "**" that stands alone as a path component (at the start of the pattern or
// after "/", and followed by "/" or the end of the pattern) matches across
// slashes. When fold is set, ASCII letters compare case-insensitively.
func wildmatch(pattern, text string, pathname, fold bool) bool {
	m := matcher{p: pattern, t: text, pathname: pathname, fold: fold}
	if strings.Count(pattern, "*") >= 2 {
		m.memo = make([]uint8, (len(pattern)+1)*(len(text)+1))
	}
	return m.match(0, 0)
}

type matcher struct {
	p, t     string
	pathname bool
	fold     bool
	memo     []uint8 // 0 unknown, 1 no match, 2 match
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

func (m *matcher) match(pi, ti int) bool {
	if m.memo == nil {
		return m.walk(pi, ti)
	}
	idx := pi*(len(m.t)+1) + ti
	if v := m.memo[idx]; v != 0 {
		return v == 2
	}
	ok := m.walk(pi, ti)
	if ok {
		m.memo[idx] = 2
	} else {
		m.memo[idx] = 1
	}
	return ok
}

func (m *matcher) walk(pi, ti int) bool {
	p, t := m.p, m.t
	for pi < len(p) {
		c := p[pi]
		if c == '*' {
			return m.star(pi, ti)
		}
		if ti >= len(t) {
			return false
		}
		tc := t[ti]
		if m.fold {
			tc = lower(tc)
		}
		switch c {
		case '?':
			if m.pathname && tc == '/' {
				return false
			}
		case '[':
			end, ok := m.class(pi, tc)
			if !ok || (m.pathname && tc == '/') {
				return false
			}
			pi = end
			ti++
			continue
		case '\\':
			pi++
			if pi >= len(p) || p[pi] != tc {
				// An escaped byte is compared as written, so under
				// case folding an escaped capital never matches.
				return false
			}
		default:
			if m.fold {
				c = lower(c)
			}
			if c != tc {
				return false
			}
		}
		pi++
		ti++
	}
	return ti == len(t)
}

func (m *matcher) star(pi, ti int) bool {
	p, t := m.p, m.t
	j := pi
	for j < len(p) && p[j] == '*' {
		j++
	}
	crossSlash := !m.pathname
	if m.pathname && j-pi >= 2 {
		startOK := pi == 0 || p[pi-1] == '/'
		endOK := j == len(p) || p[j] == '/' || (p[j] == '\\' && j+1 < len(p) && p[j+1] == '/')
		if startOK && endOK {
			if j < len(p) && p[j] == '/' && m.match(j+1, ti) {
				return true
			}
			crossSlash = true
		}
	}
	if j == len(p) {
		return crossSlash || !strings.Contains(t[ti:], "/")
	}
	if !crossSlash && p[j] == '/' {
		i := strings.IndexByte(t[ti:], '/')
		if i < 0 {
			return false
		}
		return m.match(j, ti+i)
	}
	for k := ti; ; k++ {
		if m.match(j, k) {
			return true
		}
		if k >= len(t) || (!crossSlash && t[k] == '/') {
			return false
		}
	}
}

// class evaluates the bracket expression that starts at p[pi] against the
// (already case-folded) text byte tc. It returns the index just past the
// closing bracket. ok is false when the expression does not match or is
// malformed; a malformed expression never matches anything.
func (m *matcher) class(pi int, tc byte) (end int, ok bool) {
	p := m.p
	i := pi + 1
	if i >= len(p) {
		return 0, false
	}
	negated := false
	if p[i] == '!' || p[i] == '^' {
		negated = true
		i++
	}
	matched := false
	var prev byte // previous literal byte, 0 when none (start, after a range or class)
	first := true
	for {
		if i >= len(p) {
			return 0, false
		}
		c := p[i]
		if c == ']' && !first {
			i++
			break
		}
		first = false
		switch {
		case c == '\\':
			i++
			if i >= len(p) {
				return 0, false
			}
			c = p[i]
			if tc == c {
				matched = true
			}
			prev = c
			i++
		case c == '-' && prev != 0 && i+1 < len(p) && p[i+1] != ']':
			i++
			hi := p[i]
			if hi == '\\' {
				i++
				if i >= len(p) {
					return 0, false
				}
				hi = p[i]
			}
			if tc >= prev && tc <= hi {
				matched = true
			} else if m.fold && tc >= 'a' && tc <= 'z' {
				if u := upper(tc); u >= prev && u <= hi {
					matched = true
				}
			}
			prev = 0
			i++
		case c == '[' && i+1 < len(p) && p[i+1] == ':':
			s := i + 2
			e := strings.IndexByte(p[s:], ']')
			if e < 0 {
				return 0, false
			}
			e += s // p[e] is the closing bracket
			if e == s || p[e-1] != ':' {
				// Not a class: the "[" is an ordinary member.
				if tc == '[' {
					matched = true
				}
				prev = '['
				i++
				continue
			}
			cm, known := m.posix(p[s:e-1], tc)
			if !known {
				return 0, false
			}
			if cm {
				matched = true
			}
			prev = 0
			i = e + 1
		default:
			if tc == c {
				matched = true
			}
			prev = c
			i++
		}
	}
	if matched == negated {
		return 0, false
	}
	return i, true
}

func (m *matcher) posix(name string, c byte) (matched, known bool) {
	isLower := c >= 'a' && c <= 'z'
	isUpper := c >= 'A' && c <= 'Z'
	isAlpha := isLower || isUpper
	isDigit := c >= '0' && c <= '9'
	switch name {
	case "alnum":
		return isAlpha || isDigit, true
	case "alpha":
		return isAlpha, true
	case "blank":
		return c == ' ' || c == '\t', true
	case "cntrl":
		return c < 0x20 || c == 0x7f, true
	case "digit":
		return isDigit, true
	case "graph":
		return c > 0x20 && c < 0x7f, true
	case "lower":
		return isLower, true
	case "print":
		return c >= 0x20 && c < 0x7f, true
	case "punct":
		return c > 0x20 && c < 0x7f && !isAlpha && !isDigit, true
	case "space":
		return c == ' ' || (c >= '\t' && c <= '\r'), true
	case "upper":
		return isUpper || (m.fold && isLower), true
	case "xdigit":
		return isDigit || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'), true
	}
	return false, false
}
