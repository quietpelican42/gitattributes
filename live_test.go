package gitattributes_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/__GH_OWNER__/gitattributes/internal/difftest"
)

// TestLiveDifferential compares the library with git check-attr on random
// repositories. GITATTRIBUTES_REPOS sets the repository count (default 80)
// and GITATTRIBUTES_SEED the seed (default 1).
func TestLiveDifferential(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SKIPPED LOUDLY: git is not in PATH, so the live differential against git check-attr did not run")
	}
	n, seed := 80, int64(1)
	if v, err := strconv.Atoi(os.Getenv("GITATTRIBUTES_REPOS")); err == nil && v > 0 {
		n = v
	}
	if v, err := strconv.ParseInt(os.Getenv("GITATTRIBUTES_SEED"), 10, 64); err == nil {
		seed = v
	}
	runLive(t, difftest.Generate(seed, n))
}

// TestLiveMatrix compares pattern matching alone: random glob strings, one
// attribute per pattern. GITATTRIBUTES_MATRIX sets the case count (default 40).
func TestLiveMatrix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SKIPPED LOUDLY: git is not in PATH, so the live pattern matrix against git check-attr did not run")
	}
	n := 40
	if v, err := strconv.Atoi(os.Getenv("GITATTRIBUTES_MATRIX")); err == nil && v > 0 {
		n = v
	}
	runLive(t, difftest.GenMatrix(1, n))
}

func runLive(t *testing.T, cases []difftest.Case) {
	t.Helper()
	root := t.TempDir()
	total, bad, shown := 0, 0, 0
	var st difftest.Stats
	for _, c := range cases {
		exp, err := difftest.Oracle(context.Background(), root, c)
		if err != nil {
			t.Fatalf("oracle for %s: %v", c.Name, err)
		}
		st.Add(c, exp)
		mm, q := difftest.Compare(c, exp)
		total += q
		bad += len(mm)
		for i, m := range mm {
			if i < 2 && shown < 6 {
				shown++
				t.Errorf("%s\nattributes: %.1500q\ninfo: %q global: %q ignorecase: %v", m, fmt.Sprint(c.Dirs), deref(c.Info), deref(c.Global), c.IgnoreCase)
			}
		}
		os.RemoveAll(root + "/" + c.Name)
	}
	if bad > 0 {
		t.Fail()
	}
	t.Logf("repositories=%d paths=%d (with attributes: %d) queries=%d mismatches=%d; check-attr -a lines: set=%d unset=%d valued=%d", len(cases), st.Paths, st.PathsWithAny, total, bad, st.Set, st.Unset, st.Valued)
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
