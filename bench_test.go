package gitattributes

import (
	"fmt"
	"strings"
	"testing"
)

func benchRules() *Rules {
	var root, sub strings.Builder
	root.WriteString("[attr]vendored linguist-vendored -diff\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&root, "*.ext%d text eol=lf\n", i)
	}
	root.WriteString("vendor/** vendored\n*.png binary\n*.go text diff=golang\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sub, "gen%d/*.go linguist-generated\n", i)
	}
	return NewFromDirs(map[string][]byte{"": []byte(root.String()), "pkg/api": []byte(sub.String())}, Options{})
}

func BenchmarkCheck(b *testing.B) {
	r := benchRules()
	names := []string{"text", "diff", "linguist-generated"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := r.Check("pkg/api/gen3/service.go", names...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckAll(b *testing.B) {
	r := benchRules()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.CheckAll("vendor/github.com/x/y/z.go")
	}
}
