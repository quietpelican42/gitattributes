package difftest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Triple is one line of git check-attr output.
type Triple struct {
	Attr  string `json:"a"`
	Value string `json:"v"`
}

// Expected is what git said for a Case.
type Expected struct {
	// All is git check-attr -a -z for each path, run as a fresh process per
	// path, so the attribute order is the one git prints for that path alone.
	All map[string][]Triple `json:"all"`
	// Specific is git check-attr -z --stdin with Case.Names: for each path,
	// the value of each name in order.
	Specific map[string][]string `json:"specific"`
}

// GitEnv returns an environment that isolates git from the user's and the
// system's configuration.
func GitEnv(home string) []string {
	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_ATTR_NOSYSTEM=1",
		"LC_ALL=C",
		"TZ=UTC",
		"PATH=" + os.Getenv("PATH"),
	}
	return env
}

func run(ctx context.Context, dir string, env []string, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin) // never inherit stdin
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// Build writes the repository for c under root and returns its directory.
func Build(ctx context.Context, root string, c Case) (string, error) {
	repo := filepath.Join(root, c.Name)
	if err := os.RemoveAll(repo); err != nil {
		return "", err
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return "", err
	}
	env := GitEnv(root)
	if _, err := run(ctx, repo, env, nil, "init", "-q"); err != nil {
		return "", err
	}
	ic := "false"
	if c.IgnoreCase {
		ic = "true"
	}
	if _, err := run(ctx, repo, env, nil, "config", "core.ignorecase", ic); err != nil {
		return "", err
	}
	for dir, content := range c.Dirs {
		d := filepath.Join(repo, filepath.FromSlash(dir))
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(d, ".gitattributes"), []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	if c.Info != nil {
		p := filepath.Join(repo, ".git", "info", "attributes")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, []byte(*c.Info), 0o644); err != nil {
			return "", err
		}
	}
	if c.Global != nil {
		p := filepath.Join(root, c.Name+".global-attributes")
		if err := os.WriteFile(p, []byte(*c.Global), 0o644); err != nil {
			return "", err
		}
		if _, err := run(ctx, repo, env, nil, "config", "core.attributesFile", p); err != nil {
			return "", err
		}
	}
	return repo, nil
}

func parseTriples(b []byte) ([]struct{ Path, Attr, Value string }, error) {
	parts := bytes.Split(b, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	if len(parts)%3 != 0 {
		return nil, errors.New("check-attr output is not a multiple of three fields")
	}
	var out []struct{ Path, Attr, Value string }
	for i := 0; i < len(parts); i += 3 {
		out = append(out, struct{ Path, Attr, Value string }{string(parts[i]), string(parts[i+1]), string(parts[i+2])})
	}
	return out, nil
}

// Oracle builds c under root and asks git about every path.
func Oracle(ctx context.Context, root string, c Case) (Expected, error) {
	repo, err := Build(ctx, root, c)
	if err != nil {
		return Expected{}, err
	}
	env := GitEnv(root)
	exp := Expected{All: map[string][]Triple{}, Specific: map[string][]string{}}
	for _, p := range c.Paths {
		out, err := run(ctx, repo, env, nil, "check-attr", "-a", "-z", "--", p)
		if err != nil {
			return Expected{}, err
		}
		ts, err := parseTriples(out)
		if err != nil {
			return Expected{}, err
		}
		for _, t := range ts {
			exp.All[p] = append(exp.All[p], Triple{t.Attr, t.Value})
		}
	}
	var stdin bytes.Buffer
	for _, p := range c.Paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	args := append([]string{"check-attr", "-z", "--stdin"}, c.Names...)
	out, err := run(ctx, repo, env, stdin.Bytes(), args...)
	if err != nil {
		return Expected{}, err
	}
	ts, err := parseTriples(out)
	if err != nil {
		return Expected{}, err
	}
	if len(ts) != len(c.Paths)*len(c.Names) {
		return Expected{}, fmt.Errorf("specific query returned %d results, want %d", len(ts), len(c.Paths)*len(c.Names))
	}
	for _, t := range ts {
		exp.Specific[t.Path] = append(exp.Specific[t.Path], t.Value)
	}
	return exp, nil
}
