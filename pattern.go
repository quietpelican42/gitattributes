package gitattributes

import "strings"

// pattern is a parsed attribute pattern.
type pattern struct {
	text      string // pattern without a trailing slash
	negative  bool
	mustBeDir bool
	noDir     bool // no slash inside: matches the base name at any depth
	literal   bool // no glob characters: compare as plain text
}

func parsePattern(s string) *pattern {
	p := &pattern{}
	if strings.HasPrefix(s, "!") {
		p.negative = true
		s = s[1:]
	}
	if strings.HasSuffix(s, "/") {
		p.mustBeDir = true
		s = s[:len(s)-1]
	}
	p.noDir = !strings.Contains(s, "/")
	p.literal = !strings.ContainsAny(s, "*?[\\")
	p.text = s
	return p
}

// hasPrefixFold reports whether s starts with prefix, ignoring ASCII case
// when fold is set.
func hasPrefixFold(s, prefix string, fold bool) bool {
	if len(s) < len(prefix) {
		return false
	}
	return equalFold(s[:len(prefix)], prefix, fold)
}

func equalFold(a, b string, fold bool) bool {
	if !fold {
		return a == b
	}
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

// matches reports whether the pattern applies to path. base is the directory
// of the attributes file that holds the pattern ("" for the top level, the
// global file and info/attributes). A path with a trailing slash names a
// directory.
func (p *pattern) matches(path, base string, fold bool) bool {
	isDir := strings.HasSuffix(path, "/")
	if isDir {
		path = path[:len(path)-1]
	}
	if p.mustBeDir && !isDir {
		return false
	}
	if p.noDir {
		name := path[strings.LastIndexByte(path, '/')+1:]
		if p.literal {
			return equalFold(p.text, name, fold)
		}
		return wildmatch(p.text, name, false, fold)
	}
	pat := p.text
	literal := p.literal
	if strings.HasPrefix(pat, "/") {
		pat = pat[1:]
	}
	name := path
	if base != "" {
		if len(path) < len(base)+1 || path[len(base)] != '/' || !hasPrefixFold(path, base, fold) {
			return false
		}
		name = path[len(base)+1:]
	} else if path == "" {
		return false
	}
	if literal {
		return equalFold(pat, name, fold)
	}
	return wildmatch(pat, name, true, fold)
}
