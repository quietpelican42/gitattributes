// Command panic shows go-git's gitattributes matcher panicking on a pattern
// with a trailing slash.
package main

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitattributes"
)

func main() {
	attrs, err := gitattributes.ReadAttributes(strings.NewReader("docs/ text\n"), nil, true)
	fmt.Println("parse error:", err)
	m := gitattributes.NewMatcher(attrs)
	res, _ := m.Match([]string{"docs", "a.md"}, nil)
	fmt.Println("docs/a.md ->", res)
}
