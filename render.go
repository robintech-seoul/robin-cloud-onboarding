package main

import (
	"bytes"
	"os"
	"path/filepath"
	"text/template"
)

// renderWorkflow renders the embedded workflow template. Custom [[ ]] delimiters
// let GitHub's own ${{ ... }} expressions pass through literally.
func renderWorkflow(p Plan) ([]byte, error) {
	t, err := template.New("workflow").Delims("[[", "]]").Parse(workflowTmpl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, p); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// renderDockerfile renders the per-stack Dockerfile template for a component.
// Returns ok=false (no error) when the component's stack has no template, so the
// caller can note it and move on. Executes against the Component (Port, PackageManager,
// PyModule, HasRequirements).
func renderDockerfile(c Component) ([]byte, bool, error) {
	if c.DockerTemplate == "" {
		return nil, false, nil
	}
	raw, err := dockerfileFS.ReadFile("templates/dockerfile/" + c.DockerTemplate + ".Dockerfile.tmpl")
	if err != nil {
		return nil, false, nil // no template for this stack
	}
	t, err := template.New(c.DockerTemplate).Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return nil, false, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, c); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

func writeRepoFile(root, rel string, data []byte) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func repoFileExists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
