package main

import "embed"

// The rule registry and output templates are embedded so the binary is fully
// self-contained — it can run in any repo with no external files.

//go:embed rules/*.yaml
var rulesFS embed.FS

//go:embed templates/workflows/deploy-robin-cloud.yml.tmpl
var workflowTmpl string
