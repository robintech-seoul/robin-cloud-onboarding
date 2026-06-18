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
	Branch         string
	DryRun         bool
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
	flag.BoolVar(&o.DryRun, "dry-run", false, "print the plan and workflow without writing files")
	flag.Parse()

	if o.Project == "" {
		return fmt.Errorf("--project is required (the Robin-Cloud project name)")
	}

	rules, err := loadRules()
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	s, err := scan(o.Root)
	if err != nil {
		return fmt.Errorf("scan repo: %w", err)
	}

	plan, err := buildPlan(o, detectComponents(s, rules))
	if err != nil {
		return err
	}
	printPlan(plan)

	wf, err := renderWorkflow(plan)
	if err != nil {
		return fmt.Errorf("render workflow: %w", err)
	}

	if o.DryRun {
		fmt.Println("\n--- .github/workflows/deploy-robin-cloud.yml (dry-run) ---")
		fmt.Print(string(wf))
		return nil
	}
	if err := writeRepoFile(o.Root, ".github/workflows/deploy-robin-cloud.yml", wf); err != nil {
		return fmt.Errorf("write workflow: %w", err)
	}
	fmt.Println("\nwrote .github/workflows/deploy-robin-cloud.yml")
	for _, c := range plan.Components {
		if c.GenerateDockerfile {
			fmt.Printf("note: component %q has no Dockerfile — Dockerfile generation (template %q) is a later slice; add one to build.\n", c.Module, c.DockerTemplate)
		}
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
			rule = "(dockerfile-only)"
		}
		df := "uses existing Dockerfile"
		if c.GenerateDockerfile {
			df = "needs Dockerfile (template: " + c.DockerTemplate + ")"
		}
		fmt.Printf("    - %-8s ctx=%-12s ecr=%s-%s  port=%d  rule=%s  [%s]\n",
			c.Module, c.Context, p.Project, c.Module, c.Port, rule, df)
	}
}
