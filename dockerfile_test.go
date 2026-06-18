package main

import (
	"strings"
	"testing"
)

func TestRenderDockerfile(t *testing.T) {
	cases := []struct {
		c     Component
		wants []string
	}{
		{
			Component{DockerTemplate: "fastapi", Port: 8000, PyModule: "app.main:app", HasRequirements: true},
			[]string{"FROM python:3.12", "requirements.txt", "EXPOSE 8000", `"uvicorn"`, "app.main:app"},
		},
		{
			Component{DockerTemplate: "fastapi", Port: 8000, PyModule: "main:app", HasRequirements: false},
			[]string{"pip install --no-cache-dir .", "main:app"},
		},
		{
			Component{DockerTemplate: "react-vite", Port: 80, PackageManager: "pnpm"},
			[]string{"FROM node:20", "corepack enable", "pnpm install", "nginx", "EXPOSE 80", "try_files"},
		},
		{
			Component{DockerTemplate: "react-vite", Port: 80, PackageManager: "npm"},
			[]string{"npm ci", "npm run build"},
		},
		{
			Component{DockerTemplate: "go", Port: 8080},
			[]string{"FROM golang", "go build", "distroless", "EXPOSE 8080", `ENTRYPOINT ["/server"]`},
		},
	}
	for _, tc := range cases {
		out, ok, err := renderDockerfile(tc.c)
		if err != nil || !ok {
			t.Fatalf("%s: render ok=%v err=%v", tc.c.DockerTemplate, ok, err)
		}
		for _, w := range tc.wants {
			if !strings.Contains(string(out), w) {
				t.Errorf("%s: missing %q in:\n%s", tc.c.DockerTemplate, w, out)
			}
		}
	}
}

func TestDockerfileDetection(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"web/package.json":     `{"devDependencies":{"vite":"^5"}}`,
		"web/vite.config.ts":   "export default {}",
		"web/pnpm-lock.yaml":   "",
		"api/requirements.txt": "fastapi\nuvicorn\n",
		"api/app/main.py":      "app = 1",
	})
	rules, err := loadRules()
	if err != nil {
		t.Fatal(err)
	}
	s, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	byMod := map[string]Component{}
	for _, c := range detectComponents(s, rules) {
		byMod[c.Module] = c
	}
	if got := byMod["web"].PackageManager; got != "pnpm" {
		t.Errorf("web package manager = %q, want pnpm", got)
	}
	if got := byMod["api"].PyModule; got != "app.main:app" {
		t.Errorf("api PyModule = %q, want app.main:app", got)
	}
	if !byMod["api"].HasRequirements {
		t.Error("api should have HasRequirements=true")
	}
}
