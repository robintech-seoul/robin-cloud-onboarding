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
	Builder              string   // "dockerfile" | "buildpacks"
	GenerateDockerfile   bool     // missing Dockerfile AND a stack template exists → generate one
	DockerTemplate       string   // rule's dockerfile.ifMissing template id
	BuildArgsFromSecrets []string // rule's glob patterns (e.g. VITE_*)
	BuildArgs            []string // concrete build-arg names detected from .env* files
	ModuleEnv            string   // module uppercased for shell env var names

	// Stack-specific inputs for Dockerfile generation.
	PackageManager  string // node: npm | pnpm | yarn
	PyModule        string // python: module:callable (fastapi/flask), e.g. "app.main:app"
	DjangoWsgi      string // django: gunicorn WSGI target, e.g. "config.wsgi:application"
	HasRequirements bool   // python: requirements.txt present (vs pyproject.toml)
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

// candidateDirs returns the directories to treat as deployable components: every
// directory that contains a Dockerfile OR an anchor manifest (unioned, so a mixed
// monorepo where some folders have Dockerfiles and others only manifests is fully
// covered); the repo root as a fallback when nothing matches.
func candidateDirs(s *Signals) []string {
	seen := map[string]bool{}
	var dirs []string
	for f := range s.files {
		base := path.Base(f)
		if base != "Dockerfile" && !anchorManifests[base] {
			continue
		}
		if d := path.Dir(f); !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return []string{"."}
	}
	sort.Strings(dirs)
	return dirs
}

// detectComponents matches rules against each auto-discovered candidate dir.
func detectComponents(s *Signals, rules []Rule) []Component {
	used := map[string]bool{}
	var comps []Component
	for _, dir := range candidateDirs(s) {
		comps = append(comps, buildComponent(s, rules, dir, used, "", 0))
	}
	return comps
}

// bestRule returns the highest-scoring rule that matches dir, or nil.
func bestRule(s *Signals, dir string, rules []Rule) *Rule {
	var best *Rule
	bestScore := -1
	for i := range rules {
		if ok, score := matchRule(s, dir, rules[i]); ok && score > bestScore {
			bestScore, best = score, &rules[i]
		}
	}
	return best
}

// buildComponent resolves one component at dir. moduleOverride/portOverride (from
// robin-deploy.yaml) win over the matched rule's suggestions; everything else —
// stack detection, build strategy, Dockerfile inputs — is still inferred from dir.
func buildComponent(s *Signals, rules []Rule, dir string, used map[string]bool, moduleOverride string, portOverride int) Component {
	best := bestRule(s, dir, rules)
	hasDocker := s.files[relKey(dir, "Dockerfile")]

	module, port, kind, ruleID := dirModule(dir), 8080, "service", ""
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
	if moduleOverride != "" {
		module = moduleOverride
	}
	if portOverride != 0 {
		port = portOverride
	}
	module = dedupeModule(module, used)
	used[module] = true

	filter := "**"
	if dir != "." {
		filter = dir + "/**"
	}

	// Build strategy (fidelity order): existing Dockerfile → generate one from a
	// stack template → Cloud Native Buildpacks for everything else.
	willGenerate := !hasDocker && hasDockerfileTemplate(tmpl)
	builder := "buildpacks"
	if hasDocker || willGenerate {
		builder = "dockerfile"
	}

	// Stack-specific inputs for Dockerfile generation.
	var pm, pyModule, djangoWsgi string
	var hasReq bool
	switch tmpl {
	case "react-vite", "nextjs":
		pm = detectPackageManager(s, dir)
	case "fastapi", "flask":
		pyModule = detectPyModule(s, dir)
		hasReq = s.hasFile(dir, "requirements.txt")
	case "django":
		djangoWsgi = detectDjangoWsgi(s, dir)
		hasReq = s.hasFile(dir, "requirements.txt")
	}

	return Component{
		Module:               module,
		Context:              workflowPath(dir),
		FilterGlob:           filter,
		Port:                 port,
		Kind:                 kind,
		RuleID:               ruleID,
		Builder:              builder,
		GenerateDockerfile:   willGenerate,
		DockerTemplate:       tmpl,
		BuildArgsFromSecrets: buildArgs,
		BuildArgs:            detectBuildArgs(s, dir, buildArgs),
		ModuleEnv:            envName(module),
		PackageManager:       pm,
		PyModule:             pyModule,
		DjangoWsgi:           djangoWsgi,
		HasRequirements:      hasReq,
	}
}

// detectBuildArgs reads the component's .env* files and returns the variable names
// that match the rule's buildArgsFromSecrets glob patterns (e.g. VITE_*). Only names
// are read, never values; the values come from CI secrets at build time.
func detectBuildArgs(s *Signals, dir string, patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	prefix := ""
	if dir != "." {
		prefix = dir + "/"
	}
	seen := map[string]bool{}
	var names []string
	for f := range s.files {
		rest, ok := strings.CutPrefix(f, prefix)
		if !ok || strings.Contains(rest, "/") {
			continue // only files directly in the component dir
		}
		if !strings.HasPrefix(rest, ".env") {
			continue
		}
		content, ok := s.readFile(dir, rest)
		if !ok {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key := line
			if i := strings.IndexByte(line, '='); i >= 0 {
				key = strings.TrimSpace(line[:i])
			}
			if key == "" || seen[key] {
				continue
			}
			for _, p := range patterns {
				if matched, _ := path.Match(p, key); matched {
					seen[key] = true
					names = append(names, key)
					break
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

// dirModule names a component from its directory ("." → "app", else the basename).
func dirModule(dir string) string {
	if dir == "." || dir == "" {
		return "app"
	}
	return path.Base(dir)
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

// detectPackageManager picks the node package manager from the lockfile present.
func detectPackageManager(s *Signals, dir string) string {
	switch {
	case s.hasFile(dir, "pnpm-lock.yaml"):
		return "pnpm"
	case s.hasFile(dir, "yarn.lock"):
		return "yarn"
	default:
		return "npm"
	}
}

// detectPyModule guesses the uvicorn ASGI target from the entrypoint file present.
// The app object name is assumed to be "app" (the overwhelming convention); the
// generated Dockerfile carries a comment telling the user to adjust if it differs.
func detectPyModule(s *Signals, dir string) string {
	switch {
	case s.hasFile(dir, "app/main.py"):
		return "app.main:app"
	case s.hasFile(dir, "main.py"):
		return "main:app"
	case s.hasFile(dir, "app.py"):
		return "app:app"
	case s.hasFile(dir, "server.py"):
		return "server:app"
	default:
		return "app.main:app"
	}
}

// hasDockerfileTemplate reports whether a Dockerfile template exists for a stack id.
func hasDockerfileTemplate(id string) bool {
	if id == "" {
		return false
	}
	_, err := dockerfileFS.ReadFile("templates/dockerfile/" + id + ".Dockerfile.tmpl")
	return err == nil
}

// detectDjangoWsgi finds the Django WSGI target ("<project>.wsgi:application") by
// locating a top-level "<project>/wsgi.py" under the context; defaults if none found.
func detectDjangoWsgi(s *Signals, dir string) string {
	prefix := ""
	if dir != "." {
		prefix = dir + "/"
	}
	for f := range s.files {
		rest, ok := strings.CutPrefix(f, prefix)
		if !ok {
			continue
		}
		if proj := path.Dir(rest); strings.HasSuffix(rest, "/wsgi.py") && !strings.Contains(proj, "/") {
			return proj + ".wsgi:application"
		}
	}
	return "config.wsgi:application"
}

// dockerfilePath returns the repo-relative path of the Dockerfile for a component's
// build context ("." → "Dockerfile", "./web" → "web/Dockerfile").
func dockerfilePath(ctx string) string {
	ctx = strings.TrimPrefix(ctx, "./")
	if ctx == "" || ctx == "." {
		return "Dockerfile"
	}
	return ctx + "/Dockerfile"
}
