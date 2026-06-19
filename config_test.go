package main

import (
	"strings"
	"testing"
)

// TestConfigModuleOverride: robin-deploy.yaml names the sub-projects, while the stack
// (and thus Dockerfile/buildpacks choice) is still inferred from each context.
func TestConfigModuleOverride(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"ml-service/pyproject.toml": "[project]\ndependencies = [\"fastapi\"]\n",
		"ml-service/main.py":        "app = 1",
		"web/package.json":          `{"dependencies":{"next":"14"}}`,
		"web/next.config.js":        "module.exports={}",
		"gateway/Dockerfile":        "FROM scratch",
	})
	rules, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}
	s, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}

	entries := []ConfigComponent{
		{Module: "ml", Context: "./ml-service"},
		{Module: "frontend", Context: "./web", Port: 8080},
		{Module: "gw", Context: "./gateway"},
	}
	comps, err := componentsFromConfig(s, rules, entries)
	if err != nil {
		t.Fatal(err)
	}
	byMod := map[string]Component{}
	for _, c := range comps {
		byMod[c.Module] = c
	}

	if _, ok := byMod["ml"]; !ok {
		t.Fatalf("module 'ml' missing; got %v", comps)
	}
	// custom name 'ml' but stack still detected as fastapi → generates a Dockerfile
	if got := byMod["ml"]; got.DockerTemplate != "fastapi" || !got.GenerateDockerfile {
		t.Errorf("ml: template=%q gen=%v, want fastapi/true", got.DockerTemplate, got.GenerateDockerfile)
	}
	if got := byMod["frontend"]; got.Port != 8080 {
		t.Errorf("frontend port = %d, want 8080 (override)", got.Port)
	}
	if got := byMod["frontend"]; got.RuleID != "nextjs" {
		t.Errorf("frontend rule = %q, want nextjs", got.RuleID)
	}
	if got := byMod["gw"]; got.Builder != "dockerfile" || got.GenerateDockerfile {
		t.Errorf("gw: builder=%q gen=%v, want existing Dockerfile (dockerfile/false)", got.Builder, got.GenerateDockerfile)
	}
}

func TestComponentsFromConfigValidation(t *testing.T) {
	s := &Signals{Root: ".", files: map[string]bool{}, dirs: map[string]bool{".": true}}
	if _, err := componentsFromConfig(s, nil, []ConfigComponent{{Module: "x"}}); err == nil {
		t.Error("expected error when context is missing")
	}
}

// TestConfigMonorepoSharedLib: a component can build from a wider context (repo root)
// with its own dockerfile path + watch globs (for a shared sibling) + ssh, while the
// stack is still detected from the component's SOURCE dir (the dockerfile's dir). The
// rendered workflow must wire dockerfile, ssh, the ssh-agent step, and multi-globs.
func TestConfigMonorepoSharedLib(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"core/pyproject.toml": "[project]\nname = \"core\"\n",
		"svc/pyproject.toml":  "[project]\ndependencies = [\"fastapi\"]\n",
		"svc/main.py":         "app = 1",
		"svc/Dockerfile":      "FROM scratch",
		"wk/pyproject.toml":   "[project]\ndependencies = [\"fastapi\"]\n",
		"wk/main.py":          "app = 1",
		"wk/Dockerfile":       "FROM scratch",
	})
	rules, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}
	s, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}

	entries := []ConfigComponent{
		{Module: "server", Context: ".", Dockerfile: "svc/Dockerfile", Watch: []string{"svc/**", "core/**"}},
		{Module: "worker", Context: "./wk", SSH: true},
	}
	comps, err := componentsFromConfig(s, rules, entries)
	if err != nil {
		t.Fatal(err)
	}
	byMod := map[string]Component{}
	for _, c := range comps {
		byMod[c.Module] = c
	}

	server := byMod["server"]
	if server.Context != "." {
		t.Errorf("server context = %q, want '.'", server.Context)
	}
	if server.Dockerfile != "svc/Dockerfile" {
		t.Errorf("server dockerfile = %q, want svc/Dockerfile", server.Dockerfile)
	}
	if len(server.Filters) != 2 || server.Filters[0] != "svc/**" || server.Filters[1] != "core/**" {
		t.Errorf("server filters = %v, want [svc/** core/**]", server.Filters)
	}
	// Stack detected from the dockerfile's dir (svc/), not the root context.
	if server.RuleID != "fastapi" {
		t.Errorf("server rule = %q, want fastapi (from svc/)", server.RuleID)
	}
	if server.Builder != "dockerfile" || server.GenerateDockerfile {
		t.Errorf("server: builder=%q gen=%v, want existing Dockerfile", server.Builder, server.GenerateDockerfile)
	}

	if worker := byMod["worker"]; !worker.SSH || worker.Context != "./wk" {
		t.Errorf("worker: ssh=%v context=%q, want true/./wk", worker.SSH, worker.Context)
	}

	plan, err := buildPlan(Options{Project: "p", Branch: "main", ActionRef: "a"}, comps)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.AnyDockerfile || !plan.AnySSH {
		t.Errorf("plan AnyDockerfile=%v AnySSH=%v, want both true", plan.AnyDockerfile, plan.AnySSH)
	}
	wf, err := renderWorkflow(plan)
	if err != nil {
		t.Fatal(err)
	}
	out := string(wf)
	for _, want := range []string{
		`dockerfile: "svc/Dockerfile"`,
		`ssh: "true"`,
		"- 'svc/**'",
		"- 'core/**'",
		"webfactory/ssh-agent",
		"ssh: ${{ matrix.ssh == 'true' && 'default' || '' }}",
		"dockerfile: ${{ matrix.dockerfile }}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered workflow missing %q", want)
		}
	}
}

// TestSimpleRepoNoMonorepoWiring: a single-component repo (no dockerfile/ssh override)
// must render WITHOUT the dockerfile/ssh matrix fields or ssh-agent step, so simple
// repos keep their clean output shape.
func TestSimpleRepoNoMonorepoWiring(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"Dockerfile": "FROM scratch"})
	rules, _ := loadRules()
	s, _ := scan(root)
	comps := detectComponents(s, rules)
	plan, err := buildPlan(Options{Project: "p", Branch: "main", ActionRef: "a"}, comps)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AnyDockerfile || plan.AnySSH {
		t.Fatalf("simple repo: AnyDockerfile=%v AnySSH=%v, want both false", plan.AnyDockerfile, plan.AnySSH)
	}
	wf, _ := renderWorkflow(plan)
	out := string(wf)
	for _, unwanted := range []string{"dockerfile:", "ssh:", "ssh-agent"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("simple-repo workflow unexpectedly contains %q", unwanted)
		}
	}
}
