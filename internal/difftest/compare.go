package difftest

import (
	"fmt"

	"github.com/__GH_OWNER__/gitattributes"
)

// Rules builds the library's view of c.
func Rules(c Case) *gitattributes.Rules {
	r := gitattributes.New(gitattributes.Options{IgnoreCase: c.IgnoreCase})
	for dir, content := range c.Dirs {
		r.SetDir(dir, []byte(content))
	}
	if c.Info != nil {
		r.SetInfo([]byte(*c.Info))
	}
	if c.Global != nil {
		r.SetGlobal([]byte(*c.Global))
	}
	return r
}

// Compare checks the library against git's answers for c. It returns one
// message per disagreement and the number of queries made (each path counts
// once for CheckAll and once per name for Check).
func Compare(c Case, exp Expected) (mismatches []string, queries int) {
	r := Rules(c)
	for _, p := range c.Paths {
		queries++
		got := r.CheckAll(p)
		want := exp.All[p]
		same := len(got) == len(want)
		for i := 0; same && i < len(got); i++ {
			same = got[i].Name == want[i].Attr && got[i].String() == want[i].Value
		}
		if !same {
			mismatches = append(mismatches, fmt.Sprintf("%s: CheckAll(%q)\n  git: %v\n  got: %v", c.Name, p, want, got))
		}
		res, err := r.Check(p, c.Names...)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: Check(%q): %v", c.Name, p, err))
			continue
		}
		for i, a := range res {
			queries++
			if a.String() != exp.Specific[p][i] {
				mismatches = append(mismatches, fmt.Sprintf("%s: Check(%q, %q): git %q, got %q", c.Name, p, a.Name, exp.Specific[p][i], a.String()))
			}
		}
	}
	return mismatches, queries
}

// Stats summarizes how much of git's behavior a set of answers exercises.
type Stats struct {
	Paths        int // paths asked about
	PathsWithAny int // paths for which check-attr -a printed something
	Set, Unset   int // -a lines by value
	Valued       int
}

// Add folds one Case and its Expected into s.
func (s *Stats) Add(c Case, exp Expected) {
	s.Paths += len(c.Paths)
	for _, ts := range exp.All {
		if len(ts) > 0 {
			s.PathsWithAny++
		}
		for _, t := range ts {
			switch t.Value {
			case "set":
				s.Set++
			case "unset":
				s.Unset++
			default:
				s.Valued++
			}
		}
	}
}
