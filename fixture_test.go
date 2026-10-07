package gitattributes_test

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"

	"github.com/quietpelican42/gitattributes/internal/difftest"
)

// TestFixture replays answers recorded from git check-attr by cmd/genfixture.
// It needs no git, so it guards the behavior on machines that lack it.
func TestFixture(t *testing.T) {
	f, err := os.Open("testdata/fixture.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		difftest.Case
		Expected difftest.Expected `json:"expected"`
	}
	if err := json.NewDecoder(zr).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("empty fixture")
	}
	total := 0
	for _, e := range entries {
		mm, q := difftest.Compare(e.Case, e.Expected)
		total += q
		for i, m := range mm {
			if i < 3 {
				t.Error(m)
			}
		}
	}
	t.Logf("fixture: %d repositories, %d queries", len(entries), total)
}
