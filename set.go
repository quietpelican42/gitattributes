package gitattributes

import (
	"errors"
	"fmt"
	"strings"
)

// Options configures Rules.
type Options struct {
	// IgnoreCase matches patterns and directory names case-insensitively
	// (ASCII only), as git does when core.ignorecase is true.
	IgnoreCase bool
}

// Rules holds every attributes source that applies to one repository and
// resolves attributes for paths. Build it with New and the Set methods, then
// call Check and CheckAll. Rules may be queried from several goroutines at
// once as long as nobody is calling the Set methods at the same time.
type Rules struct {
	opts     Options
	global   *file
	info     *file
	root     *file
	dirs     map[string]*file // keyed by directory, case-folded when IgnoreCase
	warnings []Warning
}

// New returns empty Rules.
func New(opts Options) *Rules {
	return &Rules{opts: opts, dirs: map[string]*file{}}
}

// NewFromDirs returns Rules built from .gitattributes contents keyed by
// directory. The key "" is the top level of the work tree, "docs/api" is a
// nested directory. Info and global content, when needed, are added with
// SetInfo and SetGlobal.
func NewFromDirs(files map[string][]byte, opts Options) *Rules {
	s := New(opts)
	for dir, content := range files {
		s.SetDir(dir, content)
	}
	return s
}

// SetGlobal sets the content of the file named by core.attributesFile. It has
// the lowest precedence. Passing nil removes it.
func (s *Rules) SetGlobal(content []byte) {
	s.global = s.load("global", content, true, s.global)
}

// SetInfo sets the content of $GIT_DIR/info/attributes. It has the highest
// precedence. Passing nil removes it.
func (s *Rules) SetInfo(content []byte) {
	s.info = s.load("info", content, true, s.info)
}

// SetDir sets the content of the .gitattributes file in dir. The key "" (or
// "." or "/") is the top level of the work tree. Passing nil removes the file.
// Macro definitions are honored only in the top-level file.
func (s *Rules) SetDir(dir string, content []byte) {
	dir = strings.Trim(dir, "/")
	if dir == "." {
		dir = ""
	}
	source := ".gitattributes"
	if dir != "" {
		source = dir + "/.gitattributes"
	}
	key := s.fold(dir)
	if dir == "" {
		s.root = s.load(source, content, true, s.root)
		return
	}
	if old, ok := s.dirs[key]; ok {
		s.dropWarnings(old.source)
		delete(s.dirs, key)
	}
	if content != nil {
		s.dirs[key] = s.load(source, content, false, nil)
	}
}

func (s *Rules) fold(dir string) string {
	if s.opts.IgnoreCase {
		return strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' {
				return r + 'a' - 'A'
			}
			return r
		}, dir)
	}
	return dir
}

func (s *Rules) dropWarnings(source string) {
	kept := s.warnings[:0]
	for _, w := range s.warnings {
		if w.Source != source {
			kept = append(kept, w)
		}
	}
	s.warnings = kept
}

func (s *Rules) load(source string, content []byte, macroOK bool, old *file) *file {
	if old != nil {
		s.dropWarnings(old.source)
	}
	if content == nil {
		return nil
	}
	f, w := parseFile(source, content, macroOK)
	s.warnings = append(s.warnings, w...)
	return f
}

// Warnings returns the lines that were ignored while parsing, in the order the
// files were added. Git prints the same conditions to standard error.
func (s *Rules) Warnings() []Warning {
	return append([]Warning(nil), s.warnings...)
}

// ErrInvalidName is returned by Check for an attribute name git would reject.
var ErrInvalidName = errors.New("gitattributes: invalid attribute name")

// builtinBinary is the macro git defines before reading any file:
// [attr]binary -diff -merge -text.
var builtinBinary = entry{
	isMacro: true,
	macro:   "binary",
	states: []attrState{
		{name: "diff", kind: Unset},
		{name: "merge", kind: Unset},
		{name: "text", kind: Unset},
	},
}

var builtinNames = []string{"binary", "diff", "merge", "text"}

// resolved maps an attribute name to its state. A name that is present with
// State Unspecified was reset with "!name" and stops lower-precedence rules.
type resolved map[string]attrState

// stackFile is one source in precedence order together with its directory.
type stackFile struct {
	f    *file
	base string
}

// stack returns the sources for path from highest to lowest precedence.
func (s *Rules) stack(path string) []stackFile {
	var out []stackFile
	if s.info != nil {
		out = append(out, stackFile{f: s.info})
	}
	// Directories of the path from deepest to shallowest.
	dir := strings.TrimSuffix(path, "/")
	var dirs []string
	for {
		i := strings.LastIndexByte(dir, '/')
		if i < 0 {
			break
		}
		dir = dir[:i]
		dirs = append(dirs, dir)
	}
	for _, d := range dirs {
		if f, ok := s.dirs[s.fold(d)]; ok {
			out = append(out, stackFile{f: f, base: d})
		}
	}
	if s.root != nil {
		out = append(out, stackFile{f: s.root})
	}
	if s.global != nil {
		out = append(out, stackFile{f: s.global})
	}
	return out
}

func (s *Rules) resolve(path string) (resolved, []stackFile) {
	st := s.stack(path)
	// Macro definitions: the highest-precedence definition wins.
	macros := map[string]*entry{}
	for _, sf := range st {
		for i := len(sf.f.entries) - 1; i >= 0; i-- {
			e := &sf.f.entries[i]
			if e.isMacro {
				if _, ok := macros[e.macro]; !ok {
					macros[e.macro] = e
				}
			}
		}
	}
	if _, ok := macros["binary"]; !ok {
		macros["binary"] = &builtinBinary
	}
	res := resolved{}
	var fill func(e *entry)
	fill = func(e *entry) {
		for i := len(e.states) - 1; i >= 0; i-- {
			a := e.states[i]
			if _, known := res[a.name]; known {
				continue
			}
			res[a.name] = a
			if a.kind == Set {
				if m, ok := macros[a.name]; ok {
					fill(m)
				}
			}
		}
	}
	fold := s.opts.IgnoreCase
	for _, sf := range st {
		for i := len(sf.f.entries) - 1; i >= 0; i-- {
			e := &sf.f.entries[i]
			if e.isMacro {
				continue
			}
			if e.pat.matches(path, sf.base, fold) {
				fill(e)
			}
		}
	}
	return res, st
}

func toAttribute(name string, a attrState, known bool) Attribute {
	if !known {
		return Attribute{Name: name}
	}
	out := Attribute{Name: name, State: a.kind}
	if a.kind == Value {
		out.Value = a.value
	}
	return out
}

// Check resolves the named attributes for path, like git check-attr with
// attribute names. The result has one entry per name, in the order given;
// attributes that no rule assigned have State Unspecified.
//
// path is slash-separated and relative to the top of the work tree, with no
// "." or ".." elements. A trailing slash marks a directory, which is the only
// kind of path a pattern with a trailing slash can match.
func (s *Rules) Check(path string, names ...string) ([]Attribute, error) {
	for _, n := range names {
		if !validAttrName(n) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidName, n)
		}
	}
	res, _ := s.resolve(path)
	out := make([]Attribute, len(names))
	for i, n := range names {
		a, ok := res[n]
		out[i] = toAttribute(n, a, ok)
	}
	return out, nil
}

// CheckAll resolves every attribute that is set, unset or valued for path,
// like git check-attr -a. Attributes that are unspecified are omitted. The
// order is the one git prints when it is run for that path alone: the order
// in which attribute names are first met while reading the built-in macro,
// the global file, the top-level file, info/attributes, and then the files of
// each directory from the top down.
func (s *Rules) CheckAll(path string) []Attribute {
	res, st := s.resolve(path)
	var order []string
	seen := map[string]bool{}
	add := func(names []string) {
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				order = append(order, n)
			}
		}
	}
	add(builtinNames)
	if s.global != nil {
		add(s.global.names)
	}
	if s.root != nil {
		add(s.root.names)
	}
	if s.info != nil {
		add(s.info.names)
	}
	for i := len(st) - 1; i >= 0; i-- {
		if st[i].base != "" {
			add(st[i].f.names)
		}
	}
	var out []Attribute
	for _, n := range order {
		a, ok := res[n]
		if !ok || a.kind == Unspecified {
			continue
		}
		out = append(out, toAttribute(n, a, true))
	}
	return out
}
