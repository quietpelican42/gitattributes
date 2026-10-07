// Command genfixture generates random attribute repositories, asks git
// check-attr about them and writes the questions and answers as JSON, so the
// differential test can run without git.
//
// Usage:
//
//	go run ./cmd/genfixture -seed 100 -n 50 -work "$TMPDIR/ga" -out testdata/fixture.json.gz
//
// An output name ending in .gz is compressed. The -rootonly flag restricts the
// cases to a single top-level file so that libraries without nesting support
// can be measured on them (see bench/incumbents)..gz
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/quietpelican42/gitattributes/internal/difftest"
)

type entry struct {
	difftest.Case
	Expected difftest.Expected `json:"expected"`
}

func main() {
	seed := flag.Int64("seed", 1, "generator seed")
	n := flag.Int("n", 60, "number of repositories")
	rootOnly := flag.Bool("rootonly", false, "generate cases with a single top-level file, no info/global, case-sensitive")
	work := flag.String("work", "", "directory for the temporary repositories (required)")
	out := flag.String("out", "testdata/fixture.json.gz", "output file")
	flag.Parse()
	if *work == "" {
		fmt.Fprintln(os.Stderr, "genfixture: -work is required")
		os.Exit(2)
	}
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "genfixture: git not found in PATH")
		os.Exit(2)
	}
	if err := os.MkdirAll(*work, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "genfixture:", err)
		os.Exit(2)
	}
	ctx := context.Background()
	var entries []entry
	gen := difftest.Generate
	if *rootOnly {
		gen = difftest.GenerateRootOnly
	}
	for _, c := range gen(*seed, *n) {
		exp, err := difftest.Oracle(ctx, *work, c)
		if err != nil {
			fmt.Fprintln(os.Stderr, "genfixture:", err)
			os.Exit(2)
		}
		entries = append(entries, entry{c, exp})
	}
	b, err := json.Marshal(entries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genfixture:", err)
		os.Exit(2)
	}
	b = append(b, '\n')
	if strings.HasSuffix(*out, ".gz") {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(b)
		zw.Close()
		b = buf.Bytes()
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genfixture:", err)
		os.Exit(2)
	}
	fmt.Printf("wrote %d repositories to %s (%d bytes)\n", len(entries), *out, len(b))
}
