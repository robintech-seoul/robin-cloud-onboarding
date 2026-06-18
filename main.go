// Command rcloud (module robin-cloud-onboarding) inspects a project repo and generates
// the GitHub Actions workflow that builds its components and deploys them to Robin-Cloud.
//
// Usage:
//
//	rcloud --project <name> [--repo .] [--dry-run]
package main

import (
	"flag"
	"fmt"
	"os"
)

// Options are the CLI inputs (also the override surface that robin-deploy.yaml will
// feed once it's wired up).
type Options struct {
	Root           string
	Project        string
	Region         string
	ConsoleBaseURL string
	OIDCRole       string
	Branch          string
	ActionRef       string
	Config          string
	DryRun          bool
	SkipDockerfiles bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var o Options
	flag.StringVar(&o.Root, "repo", ".", "path to the project repo to inspect")
	flag.StringVar(&o.Project, "project", "", "Robin-Cloud project name (required)")
	flag.StringVar(&o.Region, "region", "ap-northeast-2", "AWS region")
	flag.StringVar(&o.ConsoleBaseURL, "console", "https://console.robintech.cloud", "Robin-Cloud console base URL")
	flag.StringVar(&o.OIDCRole, "oidc-role", "", "IAM role assumed via GitHub OIDC (default: read from the ROBIN_OIDC_ROLE secret; pass a name to bake a literal)")
	flag.StringVar(&o.Branch, "branch", "main", "branch that triggers deploys")
	flag.StringVar(&o.ActionRef, "action-ref", "robintech-seoul/robin-cloud-onboarding/.github/actions/deploy-component@main", "the Robin-Cloud deploy composite action (pin a tag/SHA for production)")
	flag.StringVar(&o.Config, "config", "robin-deploy.yaml", "per-project override file (relative to --repo); used if present")
	flag.BoolVar(&o.DryRun, "dry-run", false, "print the plan and workflow without writing files")
	flag.BoolVar(&o.SkipDockerfiles, "skip-dockerfiles", false, "do not generate Dockerfiles for components missing one")
	flag.Parse()

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	cfg, cfgFound, err := loadConfig(o.Root, o.Config, set["config"])
	if err != nil {
		return err
	}
	applyConfigDefaults(&o, cfg, set)

	if o.Project == "" {
		return fmt.Errorf("--project is required (the Robin-Cloud project name; or set 'project:' in robin-deploy.yaml)")
	}

	rules, err := loadRules()
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	s, err := scan(o.Root)
	if err != nil {
		return fmt.Errorf("scan repo: %w", err)
	}

	var comps []Component
	if len(cfg.Components) > 0 {
		fmt.Printf("using %s: %d component(s) defined explicitly\n", o.Config, len(cfg.Components))
		if comps, err = componentsFromConfig(s, rules, cfg.Components); err != nil {
			return err
		}
	} else {
		if cfgFound {
			fmt.Printf("using %s (no components listed — auto-detecting)\n", o.Config)
		}
		comps = detectComponents(s, rules)
	}

	plan, err := buildPlan(o, comps)
	if err != nil {
		return err
	}
	printPlan(plan)

	wf, err := renderWorkflow(plan)
	if err != nil {
		return fmt.Errorf("render workflow: %w", err)
	}

	const wfPath = ".github/workflows/deploy-robin-cloud.yml"
	if o.DryRun {
		fmt.Printf("\n--- %s (dry-run) ---\n", wfPath)
		fmt.Print(string(wf))
	} else {
		if err := writeRepoFile(o.Root, wfPath, wf); err != nil {
			return fmt.Errorf("write workflow: %w", err)
		}
		fmt.Printf("\nwrote %s\n", wfPath)
	}

	if !o.SkipDockerfiles {
		if err := generateDockerfiles(o, plan); err != nil {
			return err
		}
	}
	return nil
}

// generateDockerfiles writes a Dockerfile for each component that lacks one. It NEVER
// overwrites an existing Dockerfile. In dry-run it prints what it would write.
func generateDockerfiles(o Options, plan Plan) error {
	for _, c := range plan.Components {
		if !c.GenerateDockerfile {
			continue
		}
		content, ok, err := renderDockerfile(c)
		if err != nil {
			return fmt.Errorf("render Dockerfile for %q: %w", c.Module, err)
		}
		rel := dockerfilePath(c.Context)
		if !ok {
			fmt.Printf("note: component %q (stack %q) has no Dockerfile template — add %s manually\n", c.Module, c.DockerTemplate, rel)
			continue
		}
		if o.DryRun {
			fmt.Printf("\n--- %s (dry-run) ---\n%s", rel, content)
			continue
		}
		if repoFileExists(o.Root, rel) {
			fmt.Printf("note: %s already exists — left unchanged\n", rel)
			continue
		}
		if err := writeRepoFile(o.Root, rel, content); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		fmt.Printf("wrote %s\n", rel)
	}
	return nil
}

func printPlan(p Plan) {
	fmt.Printf("Robin-Cloud deploy plan for project %q (region %s)\n", p.Project, p.Region)
	fmt.Printf("  console:   %s\n  oidc role: %s\n  branch:    %s\n", p.ConsoleBaseURL, p.OIDCRole, p.DefaultBranch)
	fmt.Println("  components:")
	for _, c := range p.Components {
		rule := c.RuleID
		if rule == "" {
			rule = "(none)"
		}
		var build string
		switch {
		case c.Builder == "buildpacks":
			build = "buildpacks (no Dockerfile)"
		case c.GenerateDockerfile:
			build = "generate Dockerfile (" + c.DockerTemplate + ")"
		default:
			build = "existing Dockerfile"
		}
		fmt.Printf("    - %-8s ctx=%-12s ecr=%s-%s  port=%d  rule=%-12s [%s]\n",
			c.Module, c.Context, p.Project, c.Module, c.Port, rule, build)
	}
}
