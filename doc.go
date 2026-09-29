// Package gitattributes evaluates .gitattributes files the way git check-attr
// does.
//
// Build a Rules value from the attribute files of a repository (top-level and
// nested .gitattributes, info/attributes and the file named by
// core.attributesFile), then ask for the attributes of a path with Check or
// CheckAll. The package reads no files: the caller supplies the contents, so it
// works on a work tree, on blobs read from a commit, or on test data.
//
// The rules follow gitattributes(5): later lines beat earlier ones, deeper
// directories beat shallower ones, info/attributes beats every .gitattributes,
// and the global file loses to all of them. Macros ([attr] lines) are honored
// only in the top-level file, info/attributes and the global file, and the
// built-in binary macro is defined. Negative patterns are ignored, as in git.
//
// Behavior is verified against git 2.55.0 by a differential test that builds
// random repositories and compares every answer with git check-attr.
package gitattributes
