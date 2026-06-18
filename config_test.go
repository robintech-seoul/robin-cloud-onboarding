package main

import "testing"

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
