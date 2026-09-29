# gitattributes

A Go library that evaluates `.gitattributes` files the way `git check-attr` does, including nested files, macros, `info/attributes` and the global attributes file.

```go
rules := gitattributes.NewFromDirs(map[string][]byte{
	"":     []byte("* text=auto\n*.png binary\n"),
	"docs": []byte("*.md linguist-documentation\n"),
}, gitattributes.Options{})

attrs, _ := rules.Check("docs/guide.md", "text", "linguist-documentation")
// text: auto, linguist-documentation: set
all := rules.CheckAll("logo.png") // binary: set, diff: unset, merge: unset, text: unset
```

## Why

Go programs that need attributes (linguist-style filters, LFS tooling, export rules, EOL handling) either shell out to `git check-attr` or use a partial implementation. The implementations found were measured against `git check-attr -a -z` on generated files:

- go-git's `plumbing/format/gitattributes` panics on trailing-slash patterns such as `docs/`, and its matcher lets the earliest line win when the stack is passed in the documented order.
- git-lfs's `git/gitattr` is written for LFS: it does not accept tabs between the pattern and the attributes, and it cannot tell `-x` from `x=false`.
- The other Go packages found by name search are not matchers: `sigs.k8s.io/prow/pkg/gitattributes` resolves only `linguist-generated` from the top-level file, `grava/pkg/gitattributes` writes one line to the file, and the tree-sitter grammars only parse.

Measured: `go run . -corpus corpus.json` in `bench/incumbents` (300 generated top-level files, 12,000 path lookups, git 2.55.0 as the oracle):

```
implementation                                         panics     paths right     files right
go-git v5.19.2 (stack in documented order)                258     8179  68.2%       75  25.0%
go-git v5.19.2 (stack reversed)                           258     8191  68.3%       76  25.3%
go-git v6.0.0-alpha.5 (documented order)                  258     8179  68.2%       75  25.0%
git-lfs v3.8.0 gitattr                                      0     8305  69.2%       64  21.3%
git-lfs v3.8.0 gitattr (tabs replaced by spaces)            0    10036  83.6%       92  30.7%
this module                                                 0    12000 100.0%      300 100.0%
```

The corpus is deliberately hostile: it includes lines git rejects with a warning, CRLF endings, a BOM, a NUL byte, overlong lines and quoted patterns. On a milder earlier corpus (400 files, 43 paths each, tabs and macros, no malformed lines; a separate throwaway harness, not in this repository) go-git scored 16,472 of 17,200 lookups (95.8%) with 276 of 400 files fully right. Nothing measured reached the point where a new package was unnecessary.

The go-git panic reproduces with `go run ./panic` in `bench/incumbents`:

```
panic: runtime error: index out of range [0] with length 0

goroutine 1 [running]:
github.com/go-git/go-git/v5/plumbing/format/gitattributes.(*pattern).Match(...)
	.../go-git/v5@v5.19.2/plumbing/format/gitattributes/pattern.go:66 +0x300
```

The precedence problem is in go-git's `matcher.go`, not in how it was called. The documented input order is increasing priority, the loop walks from the highest priority down, and it overwrites the result for each attribute it meets, so the lowest-priority line wins. For `*.txt foo=first` followed by `*.txt foo=second`, go-git reports `first` and git reports `second`.

## Installation

```sh
go get github.com/__GH_OWNER__/gitattributes
```

The module has no dependencies outside the standard library. `go.mod` declares Go 1.25.

## Usage

### Build the rules

The package reads no files. Supply the contents.

```go
r := gitattributes.New(gitattributes.Options{IgnoreCase: false})
r.SetDir("", topLevel)          // .gitattributes at the top of the work tree
r.SetDir("docs/api", nested)    // docs/api/.gitattributes
r.SetInfo(info)                 // .git/info/attributes
r.SetGlobal(global)             // the file named by core.attributesFile
```

`SetDir`, `SetInfo` and `SetGlobal` replace what was there; passing `nil` removes it. `NewFromDirs` builds the directory part in one call.

### Ask for attributes

```go
attrs, err := r.Check("src/main.go", "text", "eol")  // one entry per name, in order
all := r.CheckAll("src/main.go")                     // like git check-attr -a
```

A path ending in `/` names a directory. Paths are slash-separated, relative to the top of the work tree, and already clean (no `.` or `..`).

### Read the result

```go
switch a.State {
case gitattributes.Set:         // "name"
case gitattributes.Unset:       // "-name"
case gitattributes.Value:       // "name=value", see a.Value
case gitattributes.Unspecified: // never assigned, or reset with "!name"
}
a.String() // "set", "unset", "unspecified" or the value, as check-attr prints it
```

### Inspect ignored lines

`r.Warnings()` lists the lines git would warn about and ignore: negative patterns, invalid attribute names, overlong lines and macros in nested files.

## Reference

| Function or method | Description |
| --- | --- |
| `New(Options) *Rules` | Empty rules. |
| `NewFromDirs(map[string][]byte, Options) *Rules` | Rules from `.gitattributes` contents keyed by directory (`""` is the top level). |
| `(*Rules) SetDir(dir, content)` | Set or (with `nil`) remove the `.gitattributes` of a directory. |
| `(*Rules) SetInfo(content)` | Set `info/attributes`, the highest-precedence source. |
| `(*Rules) SetGlobal(content)` | Set the `core.attributesFile` content, the lowest-precedence source. |
| `(*Rules) Check(path, names...)` | Resolve named attributes. Returns `ErrInvalidName` for a name git rejects. |
| `(*Rules) CheckAll(path)` | Resolve every attribute that is set, unset or valued, in the order `git check-attr -a` prints for that path alone. |
| `(*Rules) Warnings()` | Ignored lines with source, line number and message. |

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `IgnoreCase` | `bool` | `false` | ASCII case-insensitive patterns and directory names, like `core.ignorecase=true`. |

`Attribute` has `Name`, `State` and `Value`. `State` is one of `Unspecified`, `Set`, `Unset`, `Value`.

## How it works

Sources are stacked from highest to lowest precedence: `info/attributes`, the `.gitattributes` files from the path's deepest directory up to the top, the global file. For each source, lines are read last to first. For each matching line, tokens are read last to first, and the first assignment to an attribute wins. Setting an attribute that has a macro definition expands the macro at that point, which is why a macro token can override an earlier token on the same line. The built-in `binary` macro is `-diff -merge -text`. The highest-precedence definition of a macro wins.

Patterns match as in git: a pattern without a slash matches the base name at any depth, a pattern with a slash is anchored to the directory of its file, a trailing slash restricts it to directory paths, and `**` works as a path component. Glob matching is a from-scratch implementation of the wildmatch dialect (brackets, POSIX classes, escapes, `**`), written from `gitattributes(5)`, `gitignore(5)` and observed `git check-attr` output. No git source is used.

Lines are parsed as git parses them: a quoted pattern is decoded with C escapes (octal and `\a \b \f \n \r \t \v \\ \"`), a malformed quote falls back to an unquoted word, `#` starts a comment, a UTF-8 BOM is skipped, a NUL ends the file, lines of 2048 bytes or more are ignored, and a line with an invalid attribute name is ignored whole. `-name=value` and `!name=value` ignore the value. Names starting with `builtin_` are reserved.

## Limitations

- The system-wide attributes file (`$(prefix)/etc/gitattributes`) is not modeled. Prepend its content to the global content if you need it.
- `builtin_objectmode`, which git computes from the index or file system, is not provided.
- Attribute order in `CheckAll` matches a fresh `git check-attr -a` for one path. In a long-running git process the order also depends on paths queried earlier; that is not modeled.
- Directory names are matched byte-wise (ASCII folding with `IgnoreCase`). Git on a case-insensitive file system also folds non-ASCII letters and Unicode normalization; the differential tests avoid directory-name case differences because the test machine's file system decides them.
- Paths are not normalized. Pass clean paths.
- The `--source` and `--cached` modes of `git check-attr` (attributes read from a tree or the index) are the caller's job: pass the blob contents.

## Compatibility

Verified against git 2.55.0 on macOS (arm64) with Go 1.27.1. Go 1.25 is the declared minimum and was not run. The differential test needs `git` in `PATH` and skips loudly without it; the recorded fixture test runs anywhere. Other git versions were not run. CI runs Linux and macOS, which is not the same as tested here.

## Development

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .
```

Live differential against git (default 80 repositories and 40 pattern-matrix cases; scale up with the environment variables):

```sh
GITATTRIBUTES_REPOS=600 GITATTRIBUTES_MATRIX=300 go test -count=1 -timeout 30m -run Live -v .
```

Regenerate the recorded fixture, and run a fuzz pass:

```sh
go run ./cmd/genfixture -seed 100 -n 50 -work "$TMPDIR/ga" -out testdata/fixture.json.gz
go test -run '^$' -fuzz FuzzParseCheck -fuzztime 60s .
go test -run '^$' -fuzz FuzzWildmatch -fuzztime 40s .
```

Last full run: 600 generated repositories with nested files, macros, quoted paths, `**`, `info/attributes`, a global file and both case modes; 24,000 path queries, each asked with `CheckAll` and with 14 attribute names (360,000 comparisons), 0 mismatches. The pattern matrix (300 files of 120 random glob strings, 60 random paths each) had 0 mismatches. The generator lives in `internal/difftest`.

Benchmark on an Apple M5 Pro (`go test -bench .`): `Check` about 4.8 µs and `CheckAll` about 3.0 µs per call against a 120-line top-level file and a 20-line nested file.

## License

MIT. See [LICENSE](LICENSE).
