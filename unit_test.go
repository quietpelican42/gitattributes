package gitattributes

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWildmatch(t *testing.T) {
	tests := []struct {
		pat, text string
		pathname  bool
		fold      bool
		want      bool
	}{
		{"*.js", "a.js", false, false, true},
		{"*.js", "dir/a.js", false, false, true},
		{"*.js", "dir/a.js", true, false, false},
		{"a?c", "abc", true, false, true},
		{"a?c", "a/c", true, false, false},
		{"a?c", "a/c", false, false, true},
		{"**/x", "x", true, false, true},
		{"**/x", "a/b/x", true, false, true},
		{"a/**/b", "a/b", true, false, true},
		{"a/**/b", "a/x/y/b", true, false, true},
		{"a/**", "a/x/y", true, false, true},
		{"a/**", "a/", true, false, true},
		{"a**", "ab/c", true, false, false},
		{"a**b", "a/b", true, false, false},
		{"[abc]x", "bx", true, false, true},
		{"[!abc]x", "bx", true, false, false},
		{"[^abc]x", "dx", true, false, true},
		{"[a-c]x", "bx", true, false, true},
		{"[]]", "]", true, false, true},
		{"[a-]", "-", true, false, true},
		{"[[:digit:]]", "7", true, false, true},
		{"[[:digit:]]", "x", true, false, false},
		{"[[:foo:]]", "x", true, false, false},
		{"[", "[", true, false, false},
		{"[/]", "/", true, false, false},
		{"\\*", "*", true, false, true},
		{"\\*", "a", true, false, false},
		{"a\\", "a", true, false, false},
		{"ABC", "abc", true, true, true},
		{"ABC", "abc", true, false, false},
		{"[A-Z]", "q", true, true, true},
		{"[A]", "a", true, true, false},
		{"\\A", "a", true, true, false},
		{"[[:upper:]]", "q", true, true, true},
		{"*a*a*a*a*a*a*a*a*b", strings.Repeat("a", 60), false, false, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s~%s~%v~%v", tt.pat, tt.text, tt.pathname, tt.fold), func(t *testing.T) {
			if got := wildmatch(tt.pat, tt.text, tt.pathname, tt.fold); got != tt.want {
				t.Errorf("wildmatch(%q, %q, pathname=%v, fold=%v) = %v, want %v", tt.pat, tt.text, tt.pathname, tt.fold, got, tt.want)
			}
		})
	}
}

func TestUnquoteC(t *testing.T) {
	tests := []struct {
		in, out, rest string
		ok            bool
	}{
		{`"a b" x`, "a b", " x", true},
		{`"a\tb"`, "a\tb", "", true},
		{`"\101"`, "A", "", true},
		{`"\"q\""`, `"q"`, "", true},
		{`"a"b`, "a", "b", true},
		{`"\q"`, "", "", false},
		{`"\4"`, "", "", false},
		{`"\400"`, "", "", false},
		{`"open`, "", "", false},
	}
	for _, tt := range tests {
		out, rest, ok := unquoteC(tt.in)
		if ok != tt.ok || (ok && (out != tt.out || rest != tt.rest)) {
			t.Errorf("unquoteC(%q) = %q, %q, %v; want %q, %q, %v", tt.in, out, rest, ok, tt.out, tt.rest, tt.ok)
		}
	}
}

func mustCheck(t *testing.T, r *Rules, path string, names ...string) []string {
	t.Helper()
	got, err := r.Check(path, names...)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(got))
	for i, a := range got {
		out[i] = a.String()
	}
	return out
}

func eq(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func TestPrecedence(t *testing.T) {
	r := New(Options{})
	r.SetGlobal([]byte("*.txt a=global b=global c=global d=global\n"))
	r.SetDir("", []byte("*.txt a=root b=root c=root\n*.txt a=root2\n"))
	r.SetDir("sub", []byte("*.txt a=sub b=sub\n"))
	r.SetInfo([]byte("*.txt a=info\n"))
	got := mustCheck(t, r, "sub/x.txt", "a", "b", "c", "d")
	want := []string{"info", "sub", "root", "global"}
	if !eq(got, want) {
		t.Errorf("sub/x.txt = %v, want %v", got, want)
	}
	got = mustCheck(t, r, "x.txt", "a", "b", "c", "d")
	want = []string{"info", "root", "root", "global"}
	if !eq(got, want) {
		t.Errorf("x.txt = %v, want %v", got, want)
	}
}

func TestStates(t *testing.T) {
	r := NewFromDirs(map[string][]byte{"": []byte("*.c s -u !x v=1 e=\n*.d s\n*.d !s\n*.e u\n*.e -u\n")}, Options{})
	got := mustCheck(t, r, "a.c", "s", "u", "x", "v", "e", "n")
	want := []string{"set", "unset", "unspecified", "1", "", "unspecified"}
	if !eq(got, want) {
		t.Errorf("a.c = %v, want %v", got, want)
	}
	if got := mustCheck(t, r, "a.d", "s"); got[0] != "unspecified" {
		t.Errorf("!s must reset an earlier set, got %v", got)
	}
	if got := mustCheck(t, r, "a.e", "u"); got[0] != "unset" {
		t.Errorf("later line must win, got %v", got)
	}
}

func TestMacros(t *testing.T) {
	r := New(Options{})
	r.SetDir("", []byte("[attr]web text eol=lf\n*.html web\n*.bin binary\n"))
	r.SetDir("sub", []byte("[attr]late x\n*.html late\n"))
	if got := mustCheck(t, r, "a.html", "web", "text", "eol"); !eq(got, []string{"set", "set", "lf"}) {
		t.Errorf("web macro: %v", got)
	}
	if got := mustCheck(t, r, "a.bin", "binary", "diff", "merge", "text"); !eq(got, []string{"set", "unset", "unset", "unset"}) {
		t.Errorf("builtin binary: %v", got)
	}
	if got := mustCheck(t, r, "sub/a.html", "late", "x", "web"); !eq(got, []string{"set", "unspecified", "set"}) {
		t.Errorf("macro in a nested file must be ignored (attribute set, no expansion): %v", got)
	}
	found := false
	for _, w := range r.Warnings() {
		found = found || (w.Source == "sub/.gitattributes" && w.Line == 1)
	}
	if !found {
		t.Errorf("want a warning for the nested macro, got %v", r.Warnings())
	}
	// A macro that mentions an attribute earlier on the same line wins over
	// that attribute, because git fills each line from its last token.
	r = NewFromDirs(map[string][]byte{"": []byte("[attr]m a=fromMacro\n*.x a=line m\n*.y m a=line\n")}, Options{})
	if got := mustCheck(t, r, "f.x", "a"); got[0] != "fromMacro" {
		t.Errorf("f.x a = %v", got)
	}
	if got := mustCheck(t, r, "f.y", "a"); got[0] != "line" {
		t.Errorf("f.y a = %v", got)
	}
}

func TestPatterns(t *testing.T) {
	files := map[string][]byte{
		"":    []byte("/top.txt top\n*.js js\ndocs/ dirs\ndocs/** under\nsrc/*.c srcc\n\"sp ace.txt\" spaced\n!neg negated\n\\!lit lit\n"),
		"pkg": []byte("/only.txt only\ninner/*.go inner\n"),
	}
	r := NewFromDirs(files, Options{})
	tests := []struct {
		path, attr, want string
	}{
		{"top.txt", "top", "set"},
		{"a/top.txt", "top", "unspecified"},
		{"a/b/c.js", "js", "set"},
		{"docs/", "dirs", "set"},
		{"docs", "dirs", "unspecified"},
		{"docs/x", "dirs", "unspecified"},
		{"docs/x", "under", "set"},
		{"docs/x/y", "under", "set"},
		{"docs", "under", "unspecified"},
		{"src/a.c", "srcc", "set"},
		{"src/d/a.c", "srcc", "unspecified"},
		{"sp ace.txt", "spaced", "set"},
		{"neg", "negated", "unspecified"},
		{"!lit", "lit", "set"},
		{"pkg/only.txt", "only", "set"},
		{"pkg/x/only.txt", "only", "unspecified"},
		{"only.txt", "only", "unspecified"},
		{"pkg/inner/a.go", "inner", "set"},
		{"pkg/x/inner/a.go", "inner", "unspecified"},
	}
	for _, tt := range tests {
		if got := mustCheck(t, r, tt.path, tt.attr); got[0] != tt.want {
			t.Errorf("%s %s = %q, want %q", tt.path, tt.attr, got[0], tt.want)
		}
	}
}

func TestIgnoreCase(t *testing.T) {
	files := map[string][]byte{"": []byte("*.TXT up\nMixed.md mixed\nsub/x sub\n")}
	if got := mustCheck(t, NewFromDirs(files, Options{}), "a.txt", "up"); got[0] != "unspecified" {
		t.Errorf("case-sensitive: %v", got)
	}
	r := NewFromDirs(files, Options{IgnoreCase: true})
	for _, p := range []string{"a.txt", "a.TxT", "mixed.MD"} {
		attr := "up"
		if strings.HasPrefix(p, "mixed") {
			attr = "mixed"
		}
		if got := mustCheck(t, r, p, attr); got[0] != "set" {
			t.Errorf("ignorecase %s %s: %v", p, attr, got)
		}
	}
	if got := mustCheck(t, r, "SUB/X", "sub"); got[0] != "set" {
		t.Errorf("ignorecase directory: %v", got)
	}
}

func TestCheckAllOrderAndOmissions(t *testing.T) {
	r := NewFromDirs(map[string][]byte{"": []byte("*.c zeta alpha=1 -mid !gone\n*.c text\n")}, Options{})
	var names []string
	for _, a := range r.CheckAll("a.c") {
		names = append(names, a.Name+"="+a.String())
	}
	want := "zeta=set alpha=1 mid=unset text=set"
	// git interns "binary diff merge text" first, so text comes before the
	// names of the file.
	want = "text=set zeta=set alpha=1 mid=unset"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("CheckAll = %q, want %q", got, want)
	}
}

func TestInvalidNamesAndWarnings(t *testing.T) {
	r := NewFromDirs(map[string][]byte{"": []byte("*.a bad@name ok\n*.b -\n*.c builtin_x\n!neg.d x\n*.e fine\n")}, Options{})
	if _, err := r.Check("x", "bad name"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
	if got := mustCheck(t, r, "f.a", "ok"); got[0] != "unspecified" {
		t.Errorf("a line with an invalid attribute name is dropped whole: %v", got)
	}
	if got := mustCheck(t, r, "f.e", "fine"); got[0] != "set" {
		t.Errorf("valid line after bad ones: %v", got)
	}
	if n := len(r.Warnings()); n != 4 {
		t.Errorf("warnings = %d (%v), want 4", n, r.Warnings())
	}
	r.SetDir("", nil)
	if n := len(r.Warnings()); n != 0 {
		t.Errorf("warnings after removing the file = %d, want 0", n)
	}
}

func TestFileQuirks(t *testing.T) {
	content := "\xef\xbb\xbf*.a bom\r\n" + strings.Repeat("x", 2100) + " long\n*.b crlf \r\n*.c after\x00*.d nul\n"
	r := NewFromDirs(map[string][]byte{"": []byte(content)}, Options{})
	for _, tt := range []struct{ path, attr, want string }{
		{"f.a", "bom", "set"},
		{"f.b", "crlf", "set"},
		{"f.c", "after", "set"},
		{"f.d", "nul", "unspecified"},
	} {
		if got := mustCheck(t, r, tt.path, tt.attr); got[0] != tt.want {
			t.Errorf("%s %s = %v, want %s", tt.path, tt.attr, got, tt.want)
		}
	}
}
