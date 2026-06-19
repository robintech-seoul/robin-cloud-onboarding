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

> Status: **working (Go).** The engine detects components (Dockerfile dirs or
> anchor-manifest dirs, incl. monorepos), matches the rule registry, renders the deploy
> workflow (a thin caller of the published `deploy-component` action), and **generates a
> Dockerfile for any component missing one** (per-stack templates; existing Dockerfiles
> are never overwritten), **falling back to Cloud Native Buildpacks** for stacks with no
> template. Installable via Homebrew (`brew install robintech-seoul/tap/rcloud`).
> `robin-deploy.yaml` lets you name/scope monorepo sub-projects. `go test` covers
> detection, render, Dockerfile generation, build-strategy selection, and config.
> **Not yet built:** `setup-ecr.sh` / `ROBIN_ONBOARDING.md` emission, frontend build-arg
> wiring, monorepo shared-dep / private-dep build support — see
> [`docs/limitations.md`](docs/limitations.md). Architecture: [`docs/design.md`](docs/design.md).

## Quickstart

```bash
go build -o rcloud .

# inspect a repo and preview the workflow without writing anything
./rcloud --project acme --repo /path/to/repo --dry-run

# write .github/workflows/deploy-robin-cloud.yml into the repo
./rcloud --project acme --repo /path/to/repo
```

Flags: `--project` (required), `--repo` (default `.`), `--region`, `--console`,
`--oidc-role`, `--action-ref`, `--branch`, `--dry-run`, `--skip-dockerfiles`.

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

## Supported stacks

Detection is **open-ended, not a fixed list.** Components build by a fidelity ladder:

1. **Has a Dockerfile** (yours or rcloud-generated) → built with Docker — *any* stack.
2. **Known stack, no Dockerfile** → rcloud generates one: `react-vite`, `nextjs`,
   `fastapi`, `flask`, `django`, `go-service`.
3. **Anything else** (incl. `express`, `spring-boot`, `rails`, or an unrecognized repo) →
   built with **Cloud Native Buildpacks** — no Dockerfile, no rule needed.

So coverage is effectively universal; named rules just yield leaner, tailored images.
Add a stack by dropping a `rules/*.yaml` (+ optional `templates/dockerfile/*.tmpl`).

## Naming sub-projects (`robin-deploy.yaml`)

For a monorepo, drop a `robin-deploy.yaml` at the repo root to name each component
(and pin its context/port) instead of relying on auto-detected names:

```yaml
project: myproj            # optional — replaces the --project flag
components:
  - { module: ml,       context: ./ml-service }
  - { module: frontend, context: ./web, port: 8080 }
  - { module: gateway,  context: ./gateway }
```

When present, these define the deployable set with **your** module names (→ ECR repos
`myproj-ml`, `myproj-frontend`, `myproj-gateway`); the stack of each `context` is still
auto-detected, so Dockerfile generation / buildpacks selection still apply. CLI flags
(`--project`, `--region`, …) override the file; the file overrides built-in defaults.
Without it, rcloud auto-detects components and names. Point elsewhere with `--config`.

## Layout

```
docs/                       # detection-approaches.md, design.md, onboarding-ux.md
rules/                      # custom rule registry (one stack profile per file)
templates/
  workflows/                # the generated caller workflow
  dockerfile/               # per-stack Dockerfile templates
.github/actions/
  deploy-component/         # the published composite action (Docker or buildpacks build)
packaging/homebrew/         # the rcloud formula
```

## Roadmap (multi-tool)

Claude-first is not required: because the core is rule-based, the natural shape is a
**standalone CLI** that runs from any terminal, CI, Claude, or Codex alike. See the
language decision in [`docs/design.md`](docs/design.md).
