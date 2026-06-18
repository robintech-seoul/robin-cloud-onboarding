package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Signals is the result of walking a repo: which files/dirs exist (relative,
// slash-separated paths). File contents are read lazily from disk on demand.
type Signals struct {
	Root  string
	files map[string]bool
	dirs  map[string]bool
}

// skipDirs are never descended into during the scan.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, ".venv": true, "venv": true, "__pycache__": true,
	".next": true, "target": true, ".idea": true, ".vscode": true,
}

func scan(root string) (*Signals, error) {
	s := &Signals{Root: root, files: map[string]bool{}, dirs: map[string]bool{".": true}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			s.dirs[rel] = true
			return nil
		}
		s.files[rel] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// relKey is the slash-path of <dir>/<name>, with "." treated as the repo root.
func relKey(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return path.Join(dir, name)
}

// hasFile reports whether <dir>/<name> exists. name may be a basename glob.
func (s *Signals) hasFile(dir, name string) bool {
	if s.files[relKey(dir, name)] {
		return true
	}
	if strings.ContainsAny(name, "*?[") {
		base := dir
		if base == "" {
			base = "."
		}
		for f := range s.files {
			if path.Dir(f) == base {
				if ok, _ := path.Match(name, path.Base(f)); ok {
					return true
				}
			}
		}
	}
	return false
}

func (s *Signals) readFile(dir, name string) (string, bool) {
	key := relKey(dir, name)
	if !s.files[key] {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(key)))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// dependencyPresent reports whether name is declared in <dir>/<manifest>.
// package.json is parsed as JSON (deps/devDeps/peerDeps); other manifests use a
// word-boundary token match against the raw text (good enough for requirements.txt,
// pyproject.toml, go.mod, etc.).
func (s *Signals) dependencyPresent(dir, manifest, name string) bool {
	content, ok := s.readFile(dir, manifest)
	if !ok {
		return false
	}
	if manifest == "package.json" {
		var pj struct {
			Dependencies    map[string]any `json:"dependencies"`
			DevDependencies map[string]any `json:"devDependencies"`
			PeerDependencies map[string]any `json:"peerDependencies"`
		}
		if json.Unmarshal([]byte(content), &pj) == nil {
			_, a := pj.Dependencies[name]
			_, b := pj.DevDependencies[name]
			_, c := pj.PeerDependencies[name]
			return a || b || c
		}
		// malformed JSON → fall through to token match
	}
	re := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_./-])` + regexp.QuoteMeta(name) + `($|[^A-Za-z0-9_./-])`)
	return re.MatchString(content)
}

func (s *Signals) contentMatches(dir, file, pattern string) bool {
	content, ok := s.readFile(dir, file)
	if !ok {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(content)
}
