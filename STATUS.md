# Project Status

> **Update rule:** whenever a feature is completed/released, overwrite this file as a
> *current-state snapshot* (don't append like a log — delete stale info) and commit.
> New sessions read this first (the project-pulse hook injects it at session start).

**Last updated:** 2026-06-24 · **Version:** v0.3.0

## At a glance

- `rcloud` — a Go CLI that inspects a repo and generates the GitHub Actions workflow
  (+ Dockerfiles) to build each component and deploy it to Robin-Cloud. Rule-based,
  deterministic, no AI.
- **Installable:** `brew install robintech-seoul/tap/rcloud` (tap repo
  `robintech-seoul/homebrew-tap`, source-build formula).
- **Scope:** owns only the GitHub side (repo → buildable + pushable image). The
  Robin-Cloud console owns the platform side (ECR repo create/discover, gitops scaffold,
  deploy token, deploy API).

## Done (shipped, v0.3.0)

- **Detection** — components = Dockerfile dirs ∪ anchor-manifest dirs (monorepo-aware,
  no shadowing). Rules: react-vite, nextjs, fastapi, flask, django, go-service (tailored
  Dockerfile templates); express, spring-boot, rails (→ buildpacks).
- **Generation** — thin caller workflow → published `deploy-component` composite action
  (account id / role from secrets, never hardcoded); Dockerfile generation for components
  missing one (never overwrites existing); Cloud Native Buildpacks fallback. Fidelity
  ladder: existing Dockerfile → generated → buildpacks.
- **`robin-deploy.yaml` override** — name/scope monorepo sub-projects; per-component
  wider build `context` + `dockerfile` path + `watch` globs (shared sibling lib) +
  `ssh: true` (forwards ssh-agent for private deps).
- **Frontend build-args** — auto-detect `VITE_*` / `NEXT_PUBLIC_*` names from `.env*` →
  Dockerfile `ARG`/`ENV` + workflow `build-args` from secrets.
- **Distribution** — Homebrew tap; `go test` covers detection, render, Dockerfile
  generation, build-strategy, config, and monorepo wiring.

## Next

- [ ] Generate a `.dockerignore` next to Dockerfiles (`docs/limitations.md` #4).
- [ ] GoReleaser → prebuilt release binaries + auto formula bump (drop compile-on-install).
- [ ] Bigger UX: GitHub App + keyless OIDC to remove manual secret setup
      (`docs/onboarding-ux.md`).

## Decisions / constraints (why)

- **Go single static binary** (not a Claude plugin, not AI-driven): polyglot audience,
  zero runtime, tool-agnostic (terminal / CI / any agent).
- **Public repo → no Robin-Cloud account identifiers in source.** Account id and role
  come from GitHub secrets `AWS_ACCOUNT_ID` / `ROBIN_OIDC_ROLE`; deploy token from
  `ROBIN_DEPLOY_TOKEN`.
- **rcloud overwrites the workflow** on every run but **never overwrites Dockerfiles.**
- Known limitations + workarounds: `docs/limitations.md`. Architecture: `docs/design.md`.
