package main

import (
	"fmt"
	"sort"
	"strings"
)

// Plan is the fully-resolved input to the workflow template.
type Plan struct {
	Project        string
	Region         string
	ConsoleBaseURL string
	OIDCRole       string
	DefaultBranch  string
	ActionRef      string   // the deploy composite action ref (owner/repo/path@ref)
	BuildArgs      []string // union of component build-arg names (→ secrets in the workflow)
	Components     []Component
	AnyChangedExpr string // "needs.changes.outputs.api == 'true' || ..."
	AnyDockerfile  bool   // some component pins an explicit Dockerfile path
	AnySSH         bool   // some component needs ssh-agent forwarding for its build
}

func buildPlan(o Options, comps []Component) (Plan, error) {
	if len(comps) == 0 {
		return Plan{}, fmt.Errorf("no deployable components detected (no Dockerfile and no matching rule)")
	}
	exprs := make([]string, 0, len(comps))
	for _, c := range comps {
		exprs = append(exprs, fmt.Sprintf("needs.changes.outputs.%s == 'true'", c.Module))
	}
	// Default: render the role from a GitHub secret so the tool never bakes a
	// Robin-Cloud IAM role name. An explicit --oidc-role bakes a literal instead.
	role := o.OIDCRole
	if role == "" {
		role = "${{ secrets.ROBIN_OIDC_ROLE }}"
	}
	// Union of every component's detected build-arg names, passed to the build step as
	// secrets. Each Dockerfile only consumes the ARGs it declares.
	seen := map[string]bool{}
	var buildArgs []string
	for _, c := range comps {
		for _, a := range c.BuildArgs {
			if !seen[a] {
				seen[a] = true
				buildArgs = append(buildArgs, a)
			}
		}
	}
	sort.Strings(buildArgs)
	anyDockerfile, anySSH := false, false
	for _, c := range comps {
		if c.Dockerfile != "" {
			anyDockerfile = true
		}
		if c.SSH {
			anySSH = true
		}
	}
	return Plan{
		Project:        o.Project,
		Region:         o.Region,
		ConsoleBaseURL: o.ConsoleBaseURL,
		OIDCRole:       role,
		DefaultBranch:  o.Branch,
		ActionRef:      o.ActionRef,
		BuildArgs:      buildArgs,
		Components:     comps,
		AnyChangedExpr: strings.Join(exprs, " || "),
		AnyDockerfile:  anyDockerfile,
		AnySSH:         anySSH,
	}, nil
}
