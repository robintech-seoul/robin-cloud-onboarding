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

func writeRepoFile(root, rel string, data []byte) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}
