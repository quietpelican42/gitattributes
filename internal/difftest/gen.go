// Package difftest generates random attribute repositories and records what
// git check-attr says about them. It is test support for the gitattributes
// package: the live differential test and cmd/genfixture both use it.
package difftest

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// Case is one generated repository together with the questions to ask.
type Case struct {
	Name       string            `json:"name"`
	IgnoreCase bool              `json:"ignoreCase"`
	Dirs       map[string]string `json:"dirs"`             // directory -> .gitattributes content
	Info       *string           `json:"info,omitempty"`   // $GIT_DIR/info/attributes
	Global     *string           `json:"global,omitempty"` // core.attributesFile
	Paths      []string          `json:"paths"`
	Names      []string          `json:"names"` // attributes for the specific-name queries
}

var (
	dirVocab  = []string{"", "a", "a/b", "docs", "docs/x", "src", "src/lib", "vendor"}
	fileVocab = []string{
		"a.js", "b.txt", "x.min.js", "Makefile", "README.md", "file1.txt", "file10.txt",
		"gen.go", "test-1", "q.x", "v.c", "V.C", "a b.txt", "-dash", "\u00fcn\u00ef.txt",
		"tab\tname", "quo\"te", "back\\slash", "!bang", "[br]", "A.txt", "b", "x", "root.txt",
		"foo.LOG", "x.tar.gz", ".hidden", "a.c", "a.h", "z9",
	}
	attrNames = []string{"text", "eol", "foo", "m1", "m2", "m3", "binary", "diff", "merge", "bin", "filter", "x.y-z_w", "9lives", "unknown_attr"}
)

// Generate returns n reproducible cases derived from seed.
func Generate(seed int64, n int) []Case {
	out := make([]Case, n)
	for i := range out {
		r := rand.New(rand.NewSource(seed*1_000_003 + int64(i)))
		out[i] = genCase(r, fmt.Sprintf("r%04d", i))
	}
	return out
}

// GenerateRootOnly returns n cases that use one top-level .gitattributes file,
// case-sensitive matching and nothing else. Libraries that cannot model nested
// files, info/attributes or the global file can be measured on these.
func GenerateRootOnly(seed int64, n int) []Case {
	cases := Generate(seed, n)
	for i := range cases {
		c := &cases[i]
		root, ok := c.Dirs[""]
		if !ok {
			root = "*.txt text\n"
		}
		c.Dirs = map[string]string{"": root}
		c.Info, c.Global, c.IgnoreCase = nil, nil, false
	}
	return cases
}

func pick[T any](r *rand.Rand, s []T) T { return s[r.Intn(len(s))] }

func genCase(r *rand.Rand, name string) Case {
	c := Case{Name: name, IgnoreCase: r.Intn(4) == 0, Dirs: map[string]string{}, Names: attrNames}
	// Paths.
	seen := map[string]bool{}
	for len(c.Paths) < 40 {
		p := pick(r, fileVocab)
		if d := pick(r, dirVocab); d != "" {
			p = d + "/" + p
		}
		if r.Intn(8) == 0 { // directory query
			p = pick(r, dirVocab[1:]) + "/"
			if r.Intn(2) == 0 {
				p += pick(r, fileVocab) + "/"
			}
		}
		if r.Intn(10) == 0 && !strings.HasSuffix(p, "/") { // odd case in the base name only
			i := strings.LastIndexByte(p, '/') + 1
			if i < len(p) && p[i] < 0x80 {
				p = p[:i] + strings.ToUpper(p[i:i+1]) + p[i+1:]
			}
		}
		if !seen[p] {
			seen[p] = true
			c.Paths = append(c.Paths, p)
		}
	}
	sort.Strings(c.Paths)
	// Files.
	if r.Intn(10) != 0 {
		c.Dirs[""] = genFile(r, c.Paths, true)
	}
	for _, d := range dirVocab[1:] {
		if r.Intn(2) == 0 {
			c.Dirs[d] = genFile(r, c.Paths, false)
		}
	}
	if r.Intn(10) < 3 {
		s := genFile(r, c.Paths, true)
		c.Info = &s
	}
	if r.Intn(10) < 3 {
		s := genFile(r, c.Paths, true)
		c.Global = &s
	}
	return c
}

var fixedPatterns = []string{
	"*", "**", "*.js", "*.min.js", "Makefile", "file?.txt", "file[0-9]*", "[ab].*", "*.[ch]", "test-*",
	"**/gen.go", "a/**", "a/**/b*", "docs/*.md", "/root.txt", "/*.txt", "docs/", "docs", "src/**/*.go",
	"a/b", "*/x.txt", "**/x/*", "a**", "a/**b", "\\*.js", "\\!bang", "[!a]*", "[[:digit:]]*", "[[:upper:]]*",
	"[A-Z]*", "[a-z]*.txt", "\"a b.txt\"", "\"tab\\tname\"", "\"\\101.txt\"", "\"quo\\\"te\"", "a\\", "[", "[]", "[]]",
	"[a-]", "[[:foo:]]", "[[:alpha:]", "[[:]x]", "x/", "/", "a/b/", "**/", "/**", "*/", "foo/**/", "*.LOG", "*.log",
	"[A]*", "\\A*", "a[", "x[b-a]", "[\\]]", "[^a]*", "[[]", "???", "*?*", "**/*.txt", "**/**/x", "a/**/**/b",
	"!neg", "!*.js", "\\#hash", "#comment", "\"unterminated", "\"\\q\"", "\"a\"b", "\"\"", "[attr]notmacro",
	"[[:lower:]]*", "[\\a-c]*", "[a\\-c]*", "[!]a]*", "[]-a]", "[--/]*", "[[:alpha:][:digit:]]*", "[a-z[:digit:]]*", "*[[:space:]]*", "\\[br\\]", "[[]br[]]", "[[:upper:]a-c]*", "[[:^alpha:]]", "*[[:blank:]]*", "*\\\"*", "**/\\*", "a/b**", "docs/x**", "src/l**", "a/b*", "a/**b**", "src/lib/**/a**", "src/lib**/", "src/lib/*", "src/*/lib", "vendor/**", "/docs/**", "/docs/", "docs/**/", "docs/x", "/a/b", "b", "x",
}

var attrForms = []string{
	"text", "-text", "!text", "text=auto", "eol=lf", "eol=crlf", "binary", "-binary", "!binary", "diff", "-diff",
	"merge=ours", "foo", "foo=", "foo=bar", "foo=bar=baz", "-foo=bar", "!foo=x", "m1", "m2", "m3", "-m1", "!m1",
	"m1=x", "filter=lfs", "x.y-z_w", "9lives", "bin", "-bin", "bin=2", "set", "unknown_attr",
}

var badAttrs = []string{"-", "!", "=x", "--x", "builtin_foo", "bad@name", "a/b", "x=y"}

func mutatePath(r *rand.Rand, c []string) string {
	p := strings.TrimSuffix(pick(r, c), "/")
	parts := strings.Split(p, "/")
	for i, part := range parts {
		rs := []rune(part) // splice on rune boundaries so patterns stay valid UTF-8
		switch r.Intn(7) {
		case 0:
			parts[i] = "*"
		case 1:
			parts[i] = "**"
		case 2:
			if len(rs) > 1 {
				j := r.Intn(len(rs))
				parts[i] = string(rs[:j]) + "?" + string(rs[j+1:])
			}
		case 3:
			if len(rs) > 1 {
				j := r.Intn(len(rs))
				parts[i] = string(rs[:j]) + "*" + string(rs[j:])
			}
		case 4:
			if len(rs) > 0 {
				j := r.Intn(len(rs))
				parts[i] = string(rs[:j]) + "[" + string(rs[j:j+1]) + "x]" + string(rs[j+1:])
			}
		}
	}
	// Sometimes drop leading directories, keeping a basename-style pattern.
	if r.Intn(3) == 0 {
		parts = parts[len(parts)-1:]
	}
	s := strings.Join(parts, "/")
	switch r.Intn(6) {
	case 0:
		s = "/" + s
	case 1:
		s += "/"
	case 2:
		s = "**/" + s
	}
	return s
}

func genFile(r *rand.Rand, paths []string, macroOK bool) string {
	n := 1 + r.Intn(9)
	var lines []string
	for i := 0; i < n; i++ {
		switch k := r.Intn(20); {
		case k == 0:
			lines = append(lines, pick(r, []string{"# a comment", "", "   ", "\t", "#* not a rule"}))
		case k <= 2:
			lines = append(lines, genMacro(r))
		default:
			lines = append(lines, genRule(r, paths))
		}
	}
	if r.Intn(30) == 0 {
		lines = append(lines, "*.js "+strings.Repeat("longattr", 300)) // over the line length limit
	}
	sep := "\n"
	if r.Intn(8) == 0 {
		sep = "\r\n"
	}
	s := strings.Join(lines, sep)
	if r.Intn(5) != 0 {
		s += sep
	}
	if r.Intn(40) == 0 {
		s = "\xef\xbb\xbf" + s
	}
	if r.Intn(60) == 0 {
		s += "\x00*.js afternul\n"
	}
	return s
}

func genMacro(r *rand.Rand) string {
	name := pick(r, []string{"m1", "m2", "m3", "m4", "binary", "bin"})
	var attrs []string
	for i := r.Intn(4); i > 0; i-- {
		attrs = append(attrs, pick(r, attrForms))
	}
	return macroPrefix + name + " " + strings.Join(attrs, " ")
}

const macroPrefix = "[attr]"

func genRule(r *rand.Rand, paths []string) string {
	var pat string
	if r.Intn(3) == 0 {
		pat = pick(r, fixedPatterns)
	} else {
		pat = mutatePath(r, paths)
	}
	if r.Intn(25) == 0 {
		pat = "\"" + strings.ReplaceAll(pat, "\"", "\\\"") + "\""
	}
	var attrs []string
	for i := 1 + r.Intn(3); i > 0; i-- {
		if r.Intn(25) == 0 {
			attrs = append(attrs, pick(r, badAttrs))
		} else {
			attrs = append(attrs, pick(r, attrForms))
		}
	}
	sep := pick(r, []string{" ", " ", "\t", "  ", " \t "})
	line := pat + sep + strings.Join(attrs, pick(r, []string{" ", " ", "\t"}))
	if r.Intn(10) == 0 {
		line += pick(r, []string{" ", "\t", "  ", "\r"})
	}
	if r.Intn(15) == 0 {
		line = "  " + line
	}
	return line
}

// GenMatrix returns n cases that stress pattern matching: one top-level file
// of random glob strings, each with its own attribute name p0, p1, ..., asked
// about random short paths. Random strings reach bracket expressions, escapes
// and "**" placements that the hand-written vocabulary in Generate does not.
func GenMatrix(seed int64, n int) []Case {
	out := make([]Case, n)
	for i := range out {
		r := rand.New(rand.NewSource(seed*7_000_003 + int64(i)))
		c := Case{
			Name:       fmt.Sprintf("m%04d", i),
			IgnoreCase: r.Intn(3) == 0,
			Dirs:       map[string]string{},
			Names:      []string{"p0", "p1", "p2"},
		}
		const patAlpha = "abAB/.-*?[]!^\\:x*/[]"
		const pathAlpha = "abAB.-]x:"
		var lines []string
		for k := 0; k < 120; k++ {
			b := make([]byte, 1+r.Intn(9))
			for j := range b {
				b[j] = patAlpha[r.Intn(len(patAlpha))]
			}
			lines = append(lines, fmt.Sprintf("%s p%d", b, k))
		}
		c.Dirs[""] = strings.Join(lines, "\n") + "\n"
		seen := map[string]bool{}
		for len(c.Paths) < 60 {
			var parts []string
			for d := 1 + r.Intn(3); d > 0; d-- {
				b := make([]byte, 1+r.Intn(4))
				for j := range b {
					b[j] = pathAlpha[r.Intn(len(pathAlpha))]
				}
				if bs := string(b); bs != "." && bs != ".." {
					parts = append(parts, bs)
				}
			}
			if len(parts) == 0 {
				continue
			}
			p := strings.Join(parts, "/")
			if r.Intn(6) == 0 {
				p += "/"
			}
			if !seen[p] {
				seen[p] = true
				c.Paths = append(c.Paths, p)
			}
		}
		out[i] = c
	}
	return out
}
