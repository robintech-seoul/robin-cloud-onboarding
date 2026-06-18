# robin-cloud-onboarding

> Binary: **`rcloud`** (the repo/module keeps the descriptive `robin-cloud-onboarding` name).

A tool that **inspects a project repo and generates everything it needs to deploy to
Robin-Cloud** — primarily a GitHub Actions workflow that builds container images,
pushes them to Robin-Cloud's ECR, and triggers a deployment.

It is **rule-based and deterministic**: detection and generation are driven by a small,
extensible registry of *stack rules* (`rules/*.yaml`) plus templates (`templates/`).
No AI is required for the supported stacks; the model is the same one Vercel,
Netlify, Railway (Nixpacks), and `docker init` use in production — see
[`docs/detection-approaches.md`](docs/detection-approaches.md).

> Status: **working vertical slice (Go).** The engine detects components (Dockerfile dirs
> or anchor-manifest dirs, incl. monorepos), matches the rule registry, and renders the
> deploy workflow. Verified on a FastAPI+Vite monorepo and a single Go service; output is
> valid YAML; `go test` covers detection + render. **Not yet built:** Dockerfile
> generation, `robin-deploy.yaml` override parsing, `setup-ecr.sh` / `ROBIN_ONBOARDING.md`
> emission, more stack rules, and distribution (Homebrew/releases). See
> [`docs/design.md`](docs/design.md) § "Open decisions".

## Quickstart

```bash
go build -o rcloud .

# inspect a repo and preview the workflow without writing anything
./rcloud --project acme --repo /path/to/repo --dry-run

# write .github/workflows/deploy-robin-cloud.yml into the repo
./rcloud --project acme --repo /path/to/repo
```

Flags: `--project` (required), `--repo` (default `.`), `--region`, `--console`,
`--oidc-role`, `--branch`, `--dry-run`.

The generated workflow reads three GitHub Actions secrets (set them on the target repo):
`AWS_ACCOUNT_ID`, `ROBIN_OIDC_ROLE`, and `ROBIN_DEPLOY_TOKEN` — so no Robin-Cloud account
identifiers are baked into the tool or the workflow.

## What it produces

For a target project named `<project>` on Robin-Cloud, in the **customer's own repo**:

- `.github/workflows/deploy-robin-cloud.yml` — build each detected component → push to
  `<project>-<module>` in Robin-Cloud's ECR (GitHub OIDC, no static keys) → call the
  Robin-Cloud deploy API per changed component.
- A `Dockerfile` for any component that doesn't already have one (from a per-stack
  template; existing Dockerfiles are reused untouched).
- `setup-ecr.sh` (optional) — one-time creation of the `<project>-<module>` ECR repos.
- `ROBIN_ONBOARDING.md` — the human checklist for the steps that live in the
  Robin-Cloud console (create project, deploy-config, issue the deploy token).

## How it fits with the Robin-Cloud console

This tool owns **only the GitHub side** (turning a repo into something that builds and
pushes a deployable image). The console already owns the platform side:

| Step | Owner |
|---|---|
| Detect repo structure → generate workflow + Dockerfiles | **this tool** (local, sees the filesystem) |
| Create the `<project>` project, issue deploy token | console |
| Discover `<project>-*` ECR repos (must have an image) | console (`/deploy-config/proposal`) |
| Map components → repos → scaffold the gitops chart | console (`POST /deploy-config`) |
| Bump image tag on each deploy | console deploy API (called by the workflow) |

The deploy contract the generated workflow must satisfy is documented in
[`docs/design.md`](docs/design.md) § "Robin-Cloud output contract".

## Layout

```
docs/
  detection-approaches.md   # survey: how to detect language/framework + OSS to reuse
  design.md                 # architecture, rule schema, output contract, open decisions
rules/                      # the custom rule registry (one stack profile per file)
  react-vite.yaml
  fastapi.yaml
  go-service.yaml
templates/
  workflows/
    deploy-robin-cloud.yml.tmpl   # the canonical generated workflow
  dockerfile/                     # per-stack Dockerfile templates (TODO as rules grow)
```

## Roadmap (multi-tool)

Claude-first is not required: because the core is rule-based, the natural shape is a
**standalone CLI** that runs from any terminal, CI, Claude, or Codex alike. See the
language decision in [`docs/design.md`](docs/design.md).
