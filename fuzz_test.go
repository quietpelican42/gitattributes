package gitattributes

import (
	"strings"
	"testing"
)

func FuzzParseCheck(f *testing.F) {
	seeds := []struct{ root, sub, info, path string }{
		{"*.go text eol=lf\n[attr]m a -b c=d\n*.x m\n", "*.go -text\n", "*.go diff=go\n", "sub/a.go"},
		{"\"a b\" x\n/a/**/b y\n", "**/z !x\n", "", "a/q/b"},
		{"[[:alpha:]]* v=1\n*[ w\n\\ z\n", "", "* q\n", "sub/Abc"},
		{"\xef\xbb\xbfdocs/ d\r\n*.md md \r\n", "", "", "docs/"},
		{"a\x00b\n", "", "", ""},
	}
	for _, s := range seeds {
		f.Add(s.root, s.sub, s.info, s.path)
	}
	f.Fuzz(func(t *testing.T, root, sub, info, path string) {
		for _, ic := range []bool{false, true} {
			r := New(Options{IgnoreCase: ic})
			r.SetDir("", []byte(root))
			r.SetDir("sub", []byte(sub))
			r.SetInfo([]byte(info))
			r.SetGlobal([]byte(root))
			all := r.CheckAll(path)
			names := make([]string, len(all))
			for i, a := range all {
				names[i] = a.Name
				if a.State == Unspecified {
					t.Fatalf("CheckAll returned unspecified attribute %q", a.Name)
				}
			}
			got, err := r.Check(path, names...)
			if err != nil {
				t.Fatalf("Check on names from CheckAll: %v", err)
			}
			for i := range got {
				if got[i] != all[i] {
					t.Fatalf("Check and CheckAll disagree on %q: %+v vs %+v", names[i], got[i], all[i])
				}
			}
			_ = r.Warnings()
		}
	})
}

func FuzzWildmatch(f *testing.F) {
	for _, s := range [][2]string{{"a/**/b", "a/x/b"}, {"[[:alpha:]-]*", "a-"}, {"*a*a*a*b", "aaaaaaaa"}, {"[!]a]", "b"}, {"**/", "x/"}} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pat, text string) {
		if len(pat) > 64 || len(text) > 256 {
			return
		}
		for _, pathname := range []bool{false, true} {
			for _, fold := range []bool{false, true} {
				plain := matcher{p: pat, t: text, pathname: pathname, fold: fold}
				memo := plain
				memo.memo = make([]uint8, (len(pat)+1)*(len(text)+1))
				got := memo.match(0, 0)
				if strings.Count(pat, "*") > 3 {
					continue // the unmemoized matcher is exponential in the number of stars
				}
				if a, b := plain.match(0, 0), got; a != b {
					t.Fatalf("memoized and plain matchers disagree on %q ~ %q (pathname=%v fold=%v): %v vs %v", pat, text, pathname, fold, a, b)
				}
			}
		}
		if !strings.ContainsAny(pat, "*?[\\") {
			if got := wildmatch(pat, text, true, false); got != (pat == text) {
				t.Fatalf("literal %q ~ %q = %v", pat, text, got)
			}
		}
	})
}
