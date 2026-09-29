// Command incumbents measures Go gitattributes implementations against the
// answers git check-attr gave for a corpus of top-level .gitattributes files.
//
// Generate the corpus with git installed:
//
//	go run ../../cmd/genfixture -rootonly -seed 200 -n 300 -work "$TMPDIR/ga" -out corpus.json
//
// then run:
//
//	go run . -corpus corpus.json
//
// Each implementation gets the same file and is asked for every attribute of
// every path. A path counts as right when the set of attributes and their
// values equals what git check-attr -a printed (order ignored). A file counts
// as fully right when all of its paths are right. Panics are recovered and
// counted; a path whose lookup panicked counts as wrong.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	lfs "github.com/git-lfs/git-lfs/v3/git/gitattr"
	gg5 "github.com/go-git/go-git/v5/plumbing/format/gitattributes"
	gg6 "github.com/go-git/go-git/v6/plumbing/format/gitattributes"

	ours "github.com/__GH_OWNER__/gitattributes"
)

type triple struct {
	Attr  string `json:"a"`
	Value string `json:"v"`
}

type entry struct {
	Name  string            `json:"name"`
	Dirs  map[string]string `json:"dirs"`
	Paths []string          `json:"paths"`
	Exp   struct {
		All map[string][]triple `json:"all"`
	} `json:"expected"`
}

type impl struct {
	name   string
	lookup func(root string) func(path string) map[string]string
}

func main() {
	corpus := flag.String("corpus", "corpus.json", "corpus written by genfixture -rootonly")
	flag.Parse()
	b, err := os.ReadFile(*corpus)
	if err != nil {
		fmt.Fprintln(os.Stderr, "incumbents:", err)
		os.Exit(2)
	}
	var entries []entry
	if err := json.Unmarshal(b, &entries); err != nil {
		fmt.Fprintln(os.Stderr, "incumbents:", err)
		os.Exit(2)
	}
	impls := []impl{
		{"go-git v5.19.2 (stack in documented order)", gg5Impl(false)},
		{"go-git v5.19.2 (stack reversed)", gg5Impl(true)},
		{"go-git v6.0.0-alpha.5 (documented order)", gg6Impl(false)},
		{"git-lfs v3.8.0 gitattr", lfsImpl(false)},
		{"git-lfs v3.8.0 gitattr (tabs replaced by spaces)", lfsImpl(true)},
		{"this module", oursImpl},
	}
	fmt.Printf("corpus: %d files, %d path lookups\n\n", len(entries), countPaths(entries))
	fmt.Printf("%-52s %8s %15s %15s\n", "implementation", "panics", "paths right", "files right")
	for _, im := range impls {
		panics, okPaths, totPaths, okFiles := 0, 0, 0, 0
		for _, e := range entries {
			look := im.lookup(e.Dirs[""])
			allOK := true
			for _, p := range e.Paths {
				want := map[string]string{}
				for _, t := range e.Exp.All[p] {
					want[t.Attr] = t.Value
				}
				got, panicked := safe(look, p)
				if panicked {
					panics++
				}
				totPaths++
				if !panicked && equal(got, want) {
					okPaths++
				} else {
					allOK = false
				}
			}
			if allOK {
				okFiles++
			}
		}
		fmt.Printf("%-52s %8d %8d %5.1f%% %8d %5.1f%%\n", im.name, panics, okPaths, pct(okPaths, totPaths), okFiles, pct(okFiles, len(entries)))
	}
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }

func countPaths(es []entry) int {
	n := 0
	for _, e := range es {
		n += len(e.Paths)
	}
	return n
}

func safe(look func(string) map[string]string, p string) (res map[string]string, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	return look(p), false
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v, ok := b[k]; !ok || v != a[k] {
			return false
		}
	}
	return true
}

func gg5Impl(reverse bool) func(string) func(string) map[string]string {
	return func(root string) func(string) map[string]string {
		attrs, err := gg5.ReadAttributes(strings.NewReader(root), nil, true)
		if err != nil {
			return func(string) map[string]string { return map[string]string{} }
		}
		if reverse {
			for i, j := 0, len(attrs)-1; i < j; i, j = i+1, j-1 {
				attrs[i], attrs[j] = attrs[j], attrs[i]
			}
		}
		m := gg5.NewMatcher(attrs)
		return func(p string) map[string]string {
			res, _ := m.Match(strings.Split(p, "/"), nil)
			out := map[string]string{}
			for k, a := range res {
				switch {
				case a.IsSet():
					out[k] = "set"
				case a.IsUnset():
					out[k] = "unset"
				case a.IsValueSet():
					out[k] = a.Value()
				}
			}
			return out
		}
	}
}

func gg6Impl(reverse bool) func(string) func(string) map[string]string {
	return func(root string) func(string) map[string]string {
		attrs, err := gg6.ReadAttributes(strings.NewReader(root), nil, true)
		if err != nil {
			return func(string) map[string]string { return map[string]string{} }
		}
		if reverse {
			for i, j := 0, len(attrs)-1; i < j; i, j = i+1, j-1 {
				attrs[i], attrs[j] = attrs[j], attrs[i]
			}
		}
		m := gg6.NewMatcher(attrs)
		return func(p string) map[string]string {
			res, _ := m.Match(strings.Split(p, "/"), nil)
			out := map[string]string{}
			for k, a := range res {
				switch {
				case a.IsSet():
					out[k] = "set"
				case a.IsUnset():
					out[k] = "unset"
				case a.IsValueSet():
					out[k] = a.Value()
				}
			}
			return out
		}
	}
}

// lfsImpl adapts git-lfs, whose Tree.Applied returns every matching attribute
// in file order and leaves the last-one-wins resolution to the caller. Its
// Attr type cannot tell "-x" from "x=false"; both are reported as "unset".
func lfsImpl(tabsToSpaces bool) func(string) func(string) map[string]string {
	return func(root string) func(string) map[string]string {
		data := []byte(root)
		if tabsToSpaces {
			data = bytes.ReplaceAll(data, []byte("\t"), []byte(" "))
		}
		t, err := lfs.NewFromReader(lfs.NewMacroProcessor(), bytes.NewReader(data))
		if err != nil {
			return func(string) map[string]string { return map[string]string{} }
		}
		return func(p string) map[string]string {
			out := map[string]string{}
			for _, a := range t.Applied(p) {
				switch {
				case a.Unspecified:
					delete(out, a.K)
				case a.V == "true":
					out[a.K] = "set"
				case a.V == "false":
					out[a.K] = "unset"
				default:
					out[a.K] = a.V
				}
			}
			return out
		}
	}
}

func oursImpl(root string) func(string) map[string]string {
	r := ours.New(ours.Options{})
	r.SetDir("", []byte(root))
	return func(p string) map[string]string {
		out := map[string]string{}
		for _, a := range r.CheckAll(p) {
			out[a.Name] = a.String()
		}
		return out
	}
}
