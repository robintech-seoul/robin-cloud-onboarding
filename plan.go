package main

import (
	"fmt"
	"strings"
)

// Plan is the fully-resolved input to the workflow template.
type Plan struct {
	Project        string
	Region         string
	ConsoleBaseURL string
	OIDCRole       string
	DefaultBranch  string
	Components     []Component
	AnyChangedExpr string // "needs.changes.outputs.api == 'true' || ..."
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
	return Plan{
		Project:        o.Project,
		Region:         o.Region,
		ConsoleBaseURL: o.ConsoleBaseURL,
		OIDCRole:       role,
		DefaultBranch:  o.Branch,
		Components:     comps,
		AnyChangedExpr: strings.Join(exprs, " || "),
	}, nil
}
