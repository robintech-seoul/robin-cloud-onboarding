package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectComponents(t *testing.T) {
	rules, err := loadRules()
	if err != nil {
		t.Fatalf("loadRules: %v", err)
	}

	cases := []struct {
		name  string
		files map[string]string
		want  map[string]string // module -> ruleID
	}{
		{
			name: "fastapi+vite monorepo",
			files: map[string]string{
				"backend/requirements.txt": "fastapi==0.110\nuvicorn==0.29\n",
				"frontend/package.json":    `{"devDependencies":{"vite":"^5"}}`,
				"frontend/vite.config.ts":  "export default {}",
			},
			want: map[string]string{"api": "fastapi", "web": "react-vite"},
		},
		{
			name:  "go service at root with Dockerfile",
			files: map[string]string{"go.mod": "module x\n", "Dockerfile": "FROM golang"},
			want:  map[string]string{"api": "go-service"},
		},
		{
			// Next.js routes to the nextjs rule (higher priority), not react-vite —
			// react-vite's `none: next.config.js` guard keeps it from matching.
			name: "next.js routes to nextjs, not react-vite",
			files: map[string]string{
				"package.json":   `{"dependencies":{"next":"^14","react":"^18"}}`,
				"next.config.js": "module.exports={}",
			},
			want: map[string]string{"web": "nextjs"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, tc.files)
			s, err := scan(root)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, c := range detectComponents(s, rules) {
				got[c.Module] = c.RuleID
			}
			if len(got) != len(tc.want) {
				t.Fatalf("component set mismatch: got %v want %v", got, tc.want)
			}
			for m, rid := range tc.want {
				if got[m] != rid {
					t.Errorf("module %q: got rule %q want %q", m, got[m], rid)
				}
			}
		})
	}
}

// TestRenderWorkflowValidYAML locks the [[ ]] vs ${{ }} delimiter handling: the
// rendered workflow must parse as YAML with GitHub expressions left intact.
func TestRenderWorkflowValidYAML(t *testing.T) {
	plan := Plan{
		Project: "acme", Region: "ap-northeast-2",
		ConsoleBaseURL: "https://console.robintech.cloud",
		OIDCRole:       "deploy-role", DefaultBranch: "main",
		ActionRef: "robintech-seoul/robin-cloud-onboarding/.github/actions/deploy-component@v0.2.0",
		Components: []Component{
			{Module: "api", Context: ".", FilterGlob: "**", ModuleEnv: "API", Port: 8000},
			{Module: "web", Context: "./web", FilterGlob: "web/**", ModuleEnv: "WEB", Port: 80},
		},
		AnyChangedExpr: "needs.changes.outputs.api == 'true' || needs.changes.outputs.web == 'true'",
	}
	out, err := renderWorkflow(plan)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("rendered workflow is not valid YAML: %v\n%s", err, out)
	}
}
