package main

import (
	"strings"
	"testing"
)

// TestDetectBuildArgs: only names matching the rule's patterns are picked up from
// .env* files, values are ignored, and non-matching keys are skipped.
func TestDetectBuildArgs(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"web/package.json":   `{"devDependencies":{"vite":"^5"}}`,
		"web/vite.config.ts": "export default {}",
		"web/.env.example":   "# comment\nVITE_API_BASE_URL=\nVITE_FEATURE_X=1\nSECRET_TOKEN=nope\n",
	})
	rules, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}
	s, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	comps := detectComponents(s, rules)
	if len(comps) != 1 {
		t.Fatalf("want 1 component, got %d", len(comps))
	}
	got := strings.Join(comps[0].BuildArgs, ",")
	if got != "VITE_API_BASE_URL,VITE_FEATURE_X" {
		t.Errorf("BuildArgs = %q, want VITE_API_BASE_URL,VITE_FEATURE_X (SECRET_TOKEN excluded)", got)
	}
}

func TestRenderDockerfileWithBuildArgs(t *testing.T) {
	out, ok, err := renderDockerfile(Component{
		DockerTemplate: "react-vite", Port: 80, PackageManager: "pnpm",
		BuildArgs: []string{"VITE_API_BASE_URL"},
	})
	if err != nil || !ok {
		t.Fatalf("render: ok=%v err=%v", ok, err)
	}
	for _, want := range []string{"ARG VITE_API_BASE_URL", "ENV VITE_API_BASE_URL=$VITE_API_BASE_URL"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderWorkflowWithBuildArgs(t *testing.T) {
	plan := Plan{
		Project: "shop", Region: "ap-northeast-2", ConsoleBaseURL: "https://c",
		OIDCRole: "${{ secrets.ROBIN_OIDC_ROLE }}", DefaultBranch: "main",
		ActionRef: "owner/repo/.github/actions/deploy-component@main",
		BuildArgs: []string{"VITE_API_BASE_URL"},
		Components: []Component{
			{Module: "web", Context: "./web", FilterGlob: "web/**", BuildArgs: []string{"VITE_API_BASE_URL"}},
		},
		AnyChangedExpr: "needs.changes.outputs.web == 'true'",
	}
	out, err := renderWorkflow(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"build-args: |", "VITE_API_BASE_URL=${{ secrets.VITE_API_BASE_URL }}"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("workflow missing %q in:\n%s", want, out)
		}
	}
}
