// Package skill installs the gh-merge agent skill for AI coding tools.
package skill

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Content is the SKILL.md shipped with this binary.
//
//go:embed SKILL.md
var Content []byte

// Target is one agent's user-level skills folder.
type Target struct {
	Name   string // claude, codex, opencode, agents
	Config string // agent's config dir; its presence means the agent is installed
	Dir    string // skill folder that will hold SKILL.md
}

// Targets lists the known agents relative to home.
func Targets(home string) []Target {
	return []Target{
		{"claude", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "skills", "gh-merge")},
		{"codex", filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "skills", "gh-merge")},
		{"opencode", filepath.Join(home, ".config", "opencode"), filepath.Join(home, ".config", "opencode", "skills", "gh-merge")},
		{"agents", filepath.Join(home, ".agents"), filepath.Join(home, ".agents", "skills", "gh-merge")},
	}
}

// Select picks targets by name. With no names it picks every agent whose
// config folder exists; "all" picks every target.
func Select(home string, names []string) ([]Target, error) {
	all := Targets(home)
	if len(names) == 0 {
		var found []Target
		for _, t := range all {
			if st, err := os.Stat(t.Config); err == nil && st.IsDir() {
				found = append(found, t)
			}
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no AI coding agent config found in %s; name one: claude, codex, opencode, agents or all", home)
		}
		return found, nil
	}
	var out []Target
	for _, n := range names {
		if n == "all" {
			return all, nil
		}
		ok := false
		for _, t := range all {
			if t.Name == n {
				out = append(out, t)
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (use claude, codex, opencode, agents or all)", n)
		}
	}
	return out, nil
}

// Install writes SKILL.md into each target and returns the paths written.
func Install(targets []Target) ([]string, error) {
	var paths []string
	for _, t := range targets {
		if err := os.MkdirAll(t.Dir, 0o755); err != nil {
			return paths, err
		}
		p := filepath.Join(t.Dir, "SKILL.md")
		if err := os.WriteFile(p, Content, 0o644); err != nil {
			return paths, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}
