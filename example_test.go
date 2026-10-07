package gitattributes_test

import (
	"fmt"

	"github.com/quietpelican42/gitattributes"
)

func Example() {
	rules := gitattributes.NewFromDirs(map[string][]byte{
		"":     []byte("* text=auto\n*.png binary\n*.sh eol=lf\n"),
		"docs": []byte("*.md linguist-documentation\n"),
	}, gitattributes.Options{})

	attrs, err := rules.Check("docs/guide.md", "text", "linguist-documentation", "eol")
	if err != nil {
		panic(err)
	}
	for _, a := range attrs {
		fmt.Printf("%s: %s\n", a.Name, a)
	}
	for _, a := range rules.CheckAll("logo.png") {
		fmt.Printf("%s: %s\n", a.Name, a)
	}
	// Output:
	// text: auto
	// linguist-documentation: set
	// eol: unspecified
	// binary: set
	// diff: unset
	// merge: unset
	// text: unset
}
