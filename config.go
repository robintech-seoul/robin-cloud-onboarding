package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeployConfig is the optional per-project override file (robin-deploy.yaml) checked
// into the repo. Its main job is letting you name the sub-projects of a monorepo and
// pin their build contexts/ports, instead of relying on auto-detected names.
type DeployConfig struct {
	Project        string            `yaml:"project"`
	Region         string            `yaml:"region"`
	ConsoleBaseURL string            `yaml:"console_url"`
	OIDCRole       string            `yaml:"oidc_role"`
	Branch         string            `yaml:"branch"`
	ActionRef      string            `yaml:"action_ref"`
	Components      []ConfigComponent `yaml:"components"`
}

// ConfigComponent pins one deployable unit. module + context are required; port is
// optional (defaults from the detected stack). The stack itself (and thus the
// Dockerfile template / buildpacks choice) is still inferred from the source dir.
//
// For a monorepo where a component depends on a shared sibling package (e.g. a
// service that imports a core/ library), set context to a wider dir (typically "."
// the repo root) so the Dockerfile can COPY the sibling, and point dockerfile at the
// component's own Dockerfile. watch then lists the path globs that should trigger a
// rebuild (the component's dir AND its shared deps). ssh forwards an ssh-agent to the
// build for private deps installed via `RUN --mount=type=ssh`.
type ConfigComponent struct {
	Module     string   `yaml:"module"`
	Context    string   `yaml:"context"`
	Port       int      `yaml:"port"`
	Dockerfile string   `yaml:"dockerfile"` // repo-relative; "" → "<context>/Dockerfile"
	SSH        bool     `yaml:"ssh"`        // forward ssh-agent to the docker build
	Watch      []string `yaml:"watch"`      // path-filter globs that trigger a rebuild
}

// loadConfig reads robin-deploy.yaml (or --config) relative to the repo root.
// Returns found=false (no error) when the default file is simply absent.
func loadConfig(root, configPath string, explicit bool) (DeployConfig, bool, error) {
	p := configPath
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, filepath.FromSlash(p))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return DeployConfig{}, false, nil
		}
		return DeployConfig{}, false, fmt.Errorf("read %s: %w", configPath, err)
	}
	var c DeployConfig
	if err := yaml.Unmarshal(b, &c); err != nil {
		return DeployConfig{}, false, fmt.Errorf("parse %s: %w", configPath, err)
	}
	return c, true, nil
}

// applyConfigDefaults lets the config fill values the user did not pass explicitly on
// the CLI. Explicit flags always win; config wins over built-in defaults.
func applyConfigDefaults(o *Options, c DeployConfig, set map[string]bool) {
	fill := func(flagName string, dst *string, val string) {
		if !set[flagName] && val != "" {
			*dst = val
		}
	}
	fill("project", &o.Project, c.Project)
	fill("region", &o.Region, c.Region)
	fill("console", &o.ConsoleBaseURL, c.ConsoleBaseURL)
	fill("oidc-role", &o.OIDCRole, c.OIDCRole)
	fill("branch", &o.Branch, c.Branch)
	fill("action-ref", &o.ActionRef, c.ActionRef)
}

// normalizeDir converts a config path ("./web/", "." , "web") to a clean repo-relative
// dir ("web" / "."), the form the rest of the tool keys off.
func normalizeDir(p string) string {
	d := strings.TrimPrefix(strings.TrimSuffix(p, "/"), "./")
	if d == "" {
		return "."
	}
	return d
}

// componentsFromConfig builds the component set from explicit config entries. Stack
// detection, module naming, and build-args key off the component's SOURCE dir (the
// dockerfile's dir when given, else the context dir); the build CONTEXT can be wider
// (e.g. "." to reach a shared sibling). dockerfile/ssh/watch are carried through.
func componentsFromConfig(s *Signals, rules []Rule, entries []ConfigComponent) ([]Component, error) {
	used := map[string]bool{}
	comps := make([]Component, 0, len(entries))
	for i, e := range entries {
		if e.Module == "" || e.Context == "" {
			return nil, fmt.Errorf("robin-deploy.yaml component #%d: both 'module' and 'context' are required", i+1)
		}
		contextDir := normalizeDir(e.Context)
		detectDir := contextDir
		if e.Dockerfile != "" {
			detectDir = normalizeDir(path.Dir(e.Dockerfile))
		}
		c := buildComponent(s, rules, detectDir, used, e.Module, e.Port)
		// The build context may be wider than the source dir (monorepo shared-lib).
		c.Context = workflowPath(contextDir)
		c.Dockerfile = e.Dockerfile
		c.SSH = e.SSH
		if len(e.Watch) > 0 {
			c.Filters = e.Watch
		}
		comps = append(comps, c)
	}
	return comps, nil
}
