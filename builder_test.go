package main

import (
	"strings"
	"testing"
)

// TestBuilderSelection checks the fidelity ladder: existing Dockerfile → generated
// (known stack) → buildpacks (unknown stack / template-less rule).
func TestBuilderSelection(t *testing.T) {
	rules, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		files       map[string]string
		module      string
		wantRule    string
		wantBuilder string
		wantGen     bool
	}{
		{
			name:        "nextjs → generated Dockerfile",
			files:       map[string]string{"package.json": `{"dependencies":{"next":"14"}}`, "next.config.js": "module.exports={}"},
			module:      "web",
			wantRule:    "nextjs",
			wantBuilder: "dockerfile",
			wantGen:     true,
		},
		{
			name:        "spring boot → buildpacks",
			files:       map[string]string{"pom.xml": "<project><dependency>spring-boot-starter-web</dependency></project>"},
			module:      "api",
			wantRule:    "spring-boot",
			wantBuilder: "buildpacks",
			wantGen:     false,
		},
		{
			name:        "rails → buildpacks",
			files:       map[string]string{"Gemfile": `gem "rails"`, "config/application.rb": "module App; end"},
			module:      "web",
			wantRule:    "rails",
			wantBuilder: "buildpacks",
			wantGen:     false,
		},
		{
			name:        "unknown stack (rust) → buildpacks, no rule",
			files:       map[string]string{"Cargo.toml": "[package]\nname=\"x\""},
			module:      "app",
			wantRule:    "",
			wantBuilder: "buildpacks",
			wantGen:     false,
		},
		{
			name:        "BYO Dockerfile, unknown stack → docker build",
			files:       map[string]string{"Cargo.toml": "[package]", "Dockerfile": "FROM rust"},
			module:      "app",
			wantRule:    "",
			wantBuilder: "dockerfile",
			wantGen:     false,
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
			comps := detectComponents(s, rules)
			if len(comps) != 1 {
				t.Fatalf("got %d components, want 1: %+v", len(comps), comps)
			}
			c := comps[0]
			if c.Module != tc.module {
				t.Errorf("module = %q, want %q", c.Module, tc.module)
			}
			if c.RuleID != tc.wantRule {
				t.Errorf("rule = %q, want %q", c.RuleID, tc.wantRule)
			}
			if c.Builder != tc.wantBuilder {
				t.Errorf("builder = %q, want %q", c.Builder, tc.wantBuilder)
			}
			if c.GenerateDockerfile != tc.wantGen {
				t.Errorf("generate = %v, want %v", c.GenerateDockerfile, tc.wantGen)
			}
		})
	}
}

// TestMixedMonorepoDetection guards the union fix: a Dockerfile folder must not
// shadow manifest-only folders in the same repo.
func TestMixedMonorepoDetection(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"ml/pyproject.toml":  "[project]\nname = \"ml\"\n",
		"web/package.json":   `{"devDependencies":{"vite":"^5"}}`,
		"web/pnpm-lock.yaml":  "",
		"web/vite.config.ts":  "export default {}",
		"api/Dockerfile":      "FROM scratch",
	})
	rules, _ := loadRules()
	s, _ := scan(root)
	mods := map[string]bool{}
	for _, c := range detectComponents(s, rules) {
		mods[c.Module] = true
	}
	for _, want := range []string{"ml", "web", "api"} {
		if !mods[want] {
			t.Errorf("missing component %q (got %v)", want, mods)
		}
	}
}

func TestRenderNewDockerfiles(t *testing.T) {
	cases := []struct {
		c     Component
		wants []string
	}{
		{
			Component{DockerTemplate: "nextjs", Port: 3000, PackageManager: "pnpm"},
			[]string{"pnpm install", "pnpm build", "PORT=3000", `CMD ["pnpm", "start"]`},
		},
		{
			Component{DockerTemplate: "django", Port: 8000, DjangoWsgi: "myproj.wsgi:application", HasRequirements: true},
			[]string{"gunicorn", "myproj.wsgi:application", "0.0.0.0:8000", "requirements.txt"},
		},
		{
			Component{DockerTemplate: "flask", Port: 8000, PyModule: "app:app", HasRequirements: false},
			[]string{"gunicorn", "app:app", "pip install --no-cache-dir . gunicorn"},
		},
	}
	for _, tc := range cases {
		out, ok, err := renderDockerfile(tc.c)
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", tc.c.DockerTemplate, ok, err)
		}
		for _, w := range tc.wants {
			if !strings.Contains(string(out), w) {
				t.Errorf("%s: missing %q in:\n%s", tc.c.DockerTemplate, w, out)
			}
		}
	}
}

func TestDjangoWsgiDetection(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"requirements.txt":     "django\n",
		"manage.py":            "",
		"mysite/wsgi.py":       "application = 1",
		"mysite/settings.py":   "",
	})
	rules, _ := loadRules()
	s, _ := scan(root)
	comps := detectComponents(s, rules)
	if len(comps) != 1 {
		t.Fatalf("want 1 component, got %d", len(comps))
	}
	if comps[0].RuleID != "django" {
		t.Fatalf("rule = %q, want django", comps[0].RuleID)
	}
	if comps[0].DjangoWsgi != "mysite.wsgi:application" {
		t.Errorf("DjangoWsgi = %q, want mysite.wsgi:application", comps[0].DjangoWsgi)
	}
}
