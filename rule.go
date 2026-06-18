package main

import (
	"fmt"
	"io/fs"
	"sort"

	"gopkg.in/yaml.v3"
)

// Dep is a "dependency" detection condition: name appears in the named manifest.
type Dep struct {
	Manifest string `yaml:"manifest"`
	Name     string `yaml:"name"`
}

// Content is a "content" detection condition: regex matches the file's contents.
type Content struct {
	File    string `yaml:"file"`
	Matches string `yaml:"matches"`
}

// Condition is exactly one of file / dependency / content.
type Condition struct {
	File       string   `yaml:"file"`
	Dependency *Dep     `yaml:"dependency"`
	Content    *Content `yaml:"content"`
}

// Detect groups conditions: every All must pass, ≥1 Any must pass (when present),
// and every None must fail.
type Detect struct {
	All  []Condition `yaml:"all"`
	Any  []Condition `yaml:"any"`
	None []Condition `yaml:"none"`
}

// Rule is one stack profile from rules/*.yaml. The schema mirrors @vercel/frameworks'
// declarative detector model; see docs/design.md.
type Rule struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Kind      string `yaml:"kind"` // web | service | worker
	Priority  int    `yaml:"priority"`
	Detect    Detect `yaml:"detect"`
	Component struct {
		SuggestedModule string `yaml:"suggestedModule"`
		DefaultPort     int    `yaml:"defaultPort"`
	} `yaml:"component"`
	Dockerfile struct {
		IfMissing            string   `yaml:"ifMissing"`
		BuildArgsFromSecrets []string `yaml:"buildArgsFromSecrets"`
	} `yaml:"dockerfile"`
}

// loadRules reads the embedded rule registry, highest priority first.
func loadRules() ([]Rule, error) {
	entries, err := fs.ReadDir(rulesFS, "rules")
	if err != nil {
		return nil, err
	}
	var rules []Rule
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := rulesFS.ReadFile("rules/" + e.Name())
		if err != nil {
			return nil, err
		}
		var r Rule
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("rule %s: %w", e.Name(), err)
		}
		if r.ID == "" {
			return nil, fmt.Errorf("rule %s: missing id", e.Name())
		}
		rules = append(rules, r)
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority > rules[j].Priority })
	return rules, nil
}
