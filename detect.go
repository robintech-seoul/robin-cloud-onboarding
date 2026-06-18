package main

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// Component is one resolved deployable unit in the plan.
type Component struct {
	Module               string // values key, deploy component, ECR repo suffix
	Context              string // docker build context, workflow path form ("." / "./web")
	FilterGlob           string // dorny/paths-filter glob, repo-relative ("**" / "web/**")
	Port                 int
	Kind                 string
	RuleID               string
	GenerateDockerfile   bool     // no Dockerfile present → must be generated
	DockerTemplate       string   // rule's dockerfile.ifMissing template id
	BuildArgsFromSecrets []string
	ModuleEnv            string // module uppercased for shell env var names
}

func evalCondition(s *Signals, dir string, c Condition) bool {
	switch {
	case c.File != "":
		return s.hasFile(dir, c.File)
	case c.Dependency != nil:
		return s.dependencyPresent(dir, c.Dependency.Manifest, c.Dependency.Name)
	case c.Content != nil:
		return s.contentMatches(dir, c.Content.File, c.Content.Matches)
	}
	return false
}

// matchRule evaluates a rule against a candidate dir. score = number of passing
// All+Any conditions, used to rank competing rules (ties broken by Rule.Priority).
func matchRule(s *Signals, dir string, r Rule) (bool, int) {
	score := 0
	for _, c := range r.Detect.All {
		if !evalCondition(s, dir, c) {
			return false, 0
		}
		score++
	}
	if len(r.Detect.Any) > 0 {
		anyPass := 0
		for _, c := range r.Detect.Any {
			if evalCondition(s, dir, c) {
				anyPass++
			}
		}
		if anyPass == 0 {
			return false, 0
		}
		score += anyPass
	}
	for _, c := range r.Detect.None {
		if evalCondition(s, dir, c) {
			return false, 0
		}
	}
	return true, score
}

// anchorManifests mark a directory as a likely deployable unit when no Dockerfile
// is present (the monorepo case — components live in subdirectories).
var anchorManifests = map[string]bool{
	"package.json": true, "go.mod": true,
	"pyproject.toml": true, "requirements.txt": true,
	"Cargo.toml": true, "pom.xml": true, "build.gradle": true,
	"composer.json": true, "Gemfile": true,
}

// candidateDirs returns the directories to treat as deployable components, in
// fidelity order: every directory containing a Dockerfile; else every directory
// containing an anchor manifest; else the repo root.
func candidateDirs(s *Signals) []string {
	dockerSeen, manifestSeen := map[string]bool{}, map[string]bool{}
	var withDocker, withManifest []string
	for f := range s.files {
		base, d := path.Base(f), path.Dir(f)
		if base == "Dockerfile" && !dockerSeen[d] {
			dockerSeen[d] = true
			withDocker = append(withDocker, d)
		}
		if anchorManifests[base] && !manifestSeen[d] {
			manifestSeen[d] = true
			withManifest = append(withManifest, d)
		}
	}
	switch {
	case len(withDocker) > 0:
		sort.Strings(withDocker)
		return withDocker
	case len(withManifest) > 0:
		sort.Strings(withManifest)
		return withManifest
	default:
		return []string{"."}
	}
}

// detectComponents matches rules against each candidate dir and resolves modules.
func detectComponents(s *Signals, rules []Rule) []Component {
	used := map[string]bool{}
	var comps []Component
	for _, dir := range candidateDirs(s) {
		var best *Rule
		bestScore := -1
		for i := range rules {
			if ok, score := matchRule(s, dir, rules[i]); ok && score > bestScore {
				bestScore, best = score, &rules[i]
			}
		}
		hasDocker := s.files[relKey(dir, "Dockerfile")]
		if best == nil && !hasDocker {
			continue // no rule and no Dockerfile → not a deployable unit
		}

		module, port, kind, ruleID := "app", 8080, "service", ""
		var tmpl string
		var buildArgs []string
		if best != nil {
			module = best.Component.SuggestedModule
			if module == "" {
				module = best.ID
			}
			if best.Component.DefaultPort != 0 {
				port = best.Component.DefaultPort
			}
			kind, ruleID = best.Kind, best.ID
			tmpl, buildArgs = best.Dockerfile.IfMissing, best.Dockerfile.BuildArgsFromSecrets
		}
		module = dedupeModule(module, used)
		used[module] = true

		filter := "**"
		if dir != "." {
			filter = dir + "/**"
		}

		comps = append(comps, Component{
			Module:               module,
			Context:              workflowPath(dir),
			FilterGlob:           filter,
			Port:                 port,
			Kind:                 kind,
			RuleID:               ruleID,
			GenerateDockerfile:   !hasDocker,
			DockerTemplate:       tmpl,
			BuildArgsFromSecrets: buildArgs,
			ModuleEnv:            envName(module),
		})
	}
	return comps
}

func workflowPath(p string) string {
	if p == "." || p == "" {
		return "."
	}
	return "./" + p
}

func envName(m string) string {
	return strings.ToUpper(strings.ReplaceAll(m, "-", "_"))
}

func dedupeModule(m string, used map[string]bool) string {
	if !used[m] {
		return m
	}
	for i := 2; ; i++ {
		c := m + "-" + strconv.Itoa(i)
		if !used[c] {
			return c
		}
	}
}
