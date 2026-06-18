# Detecting language / framework / build, and the OSS that does it

This is the survey behind the tool's design. Goal: decide *how* to detect a project's
deployable shape, and *what existing work to reuse vs. reimplement*.

The honest summary up front:

- **Detecting the project type is a mature, decade-old, production-proven technique.**
  Every "git push and we deploy" product does it.
- **Nobody achieves "fully automatic for all projects."** The proven systems all do
  **detect → confirm/override → escape hatch**. We should too.
- For Robin-Cloud the surface is small (mostly FastAPI/Python, React/Vite, Go), so a
  registry of a handful of rules + a confirm step gives near-complete coverage for the
  cases that matter — and **buildpacks** are the standard answer for the long tail.

## The detection signals (in increasing fidelity)

1. **Marker / manifest files** — `package.json`, `pyproject.toml` / `requirements.txt`,
   `go.mod`, `pom.xml` / `build.gradle`, `Gemfile`, `Cargo.toml`, `composer.json`.
   → identifies the **language**.
2. **Dependencies inside a manifest** — `react`/`vue`/`next`/`vite` in `package.json`;
   `fastapi`/`flask`/`django` in the Python manifest; `gin`/`echo`/`fiber` in `go.mod`.
   → identifies the **framework**.
3. **Framework config files** — `vite.config.ts`, `next.config.js`, `nuxt.config.ts`,
   `angular.json`, `svelte.config.js`, `manage.py` (Django), `Procfile`, `fly.toml`.
   → disambiguates frameworks that share a language.
4. **Lockfiles** — `pnpm-lock.yaml`/`yarn.lock`/`package-lock.json`, `poetry.lock`/
   `uv.lock`/`Pipfile.lock`. → identifies the **package manager** (affects install/build cmd).
5. **Workspace / monorepo markers** — `pnpm-workspace.yaml`, `turbo.json`, `nx.json`,
   `lerna.json`, `go.work`, `workspaces` in `package.json`. → there are **multiple apps**;
   the user must pick which dirs are deployable (the "root directory" problem).
6. **Existing containerization** — `Dockerfile`(s), `compose.yaml`. → **highest-fidelity
   signal**: each Dockerfile directory is an already-declared deployable unit. When present,
   prefer it over any framework guess.

Fidelity ranking matters: **a present Dockerfile beats a framework guess beats a language
guess.** The engine should resolve in that order.

## OSS landscape — what each does and whether to reuse it

### Framework-detection registries (closest to what we want)

| Project | Lang | What it is | Reuse for us |
|---|---|---|---|
| **`@vercel/frameworks`** | TS | A declarative array of ~40 frameworks; each has `detectors` (file path + content/regex matchers) and default build/output settings. MIT. | **Best schema reference.** Our rule schema is modeled on its `detectors: {some/every: [...]}` shape. Directly usable as a dependency if we go Node. |
| **`@netlify/framework-info`** | TS | Same idea — detect by npm dependency / config file, return build command + dev command + port. MIT. | Reference (or a second Node dep). Confirms the schema shape is the industry norm. |
| **Nixpacks** (Railway) | Rust | `providers` (Node, Python, Go, Deno, …); each detects its stack and emits a build plan, then an OCI image. MIT. | Reference for per-language detection + default build commands. Could also be invoked as a *builder* (see below). |
| **Renovate "managers"** | TS | A large registry of file matchers for every package ecosystem (npm, pip, poetry, gomod, cargo, bundler, …). | Reference for *robust* manifest matching (edge cases in how each ecosystem lays out files). |

### Build-without-a-Dockerfile (covers the long tail)

| Project | Lang | What it is | Reuse for us |
|---|---|---|---|
| **Cloud Native Buildpacks / Paketo / `pack`** | Go | CNCF. A `detect` phase per buildpack ("is there a `go.mod`? a `package.json`?") then builds an OCI image with **no Dockerfile**. Powers Heroku, Google Cloud Run `--source`, etc. Apache-2. | **Strategy option (below).** The workflow can run `pack build` / the Paketo builder to get broad language coverage with zero Dockerfile generation. |
| **Nixpacks** | Rust | As above — also a no-Dockerfile builder, used by Railway. | Same role as buildpacks; lighter, opinionated. |
| **`docker init`** | Go | Docker's official CLI: scans a project and emits a Dockerfile + compose for Go, Python, Node, Rust, .NET, PHP, Java. | **Reference for our Dockerfile templates** (`templates/dockerfile/`). |

### Language detection (coarse)

| Project | Lang | What it is | Reuse for us |
|---|---|---|---|
| **GitHub Linguist** | Ruby | The "language bar" engine: detects languages by extension + heuristics + content (`languages.yml`). | Language-level only, no framework/build. Useful as a *fallback* signal ("repo is 80% Python") when no manifest matches. Don't take it as a hard dependency. |

### Generation / templating engines (the output side)

| Option | Notes |
|---|---|
| Go `text/template` | Stdlib, single-binary friendly. Matches `robin-cli`. |
| `eta` / `handlebars` (Node) | If Node; pairs with `@vercel/frameworks`. |
| `projen`, Yeoman, cookiecutter | Heavier scaffolding frameworks — overkill for "render a few files"; mentioned for completeness. |

## The "git push → deploy" products to study (prior art for the whole flow)

Heroku, Google Cloud Run (`--source`), **Railway** (Nixpacks), **Render**, **Fly.io**
(`fly launch` — scans repo, generates `fly.toml` + Dockerfile), **Vercel** / **Netlify**
(framework registry + confirm screen), **Coolify** / **Zeabur** (open-source PaaS).

Common pattern across all of them, and the one we should copy:

> **detect → present the inferred plan → let the user confirm or override → generate.**
> None are zero-interaction. The confirm step is a *feature*, not the messy interaction
> to avoid.

## Two generation strategies (the key architectural fork)

**Strategy 1 — Dockerfile-based (full control).**
Detect stack → reuse the repo's Dockerfile if present, else render one from a per-stack
template → the workflow does `docker build`. Smaller, controlled images. Cost: we maintain
a Dockerfile template per supported stack (the stack-specific work).

**Strategy 2 — Buildpack-based (broad coverage).**
The workflow runs Cloud Native Buildpacks (`pack build` / Paketo) — **no Dockerfile at
all**. This is *literally how the industry gets "almost all projects"*. Cost: larger,
opinionated images, and a runtime dependency on the builder.

**Recommended: hybrid, resolved in fidelity order.**
1. Repo has a Dockerfile → use it (most reliable).
2. Known stack, no Dockerfile → render the template (Strategy 1).
3. Unknown / long-tail stack → offer buildpacks (Strategy 2) **or** ask the user to add a
   Dockerfile.

This is what addresses the "is it possible for almost all projects?" worry directly:
the **workflow generation is stack-agnostic** (Docker/OCI image is the contract — once a
component produces an image, the build→push→deploy workflow is identical), and only the
**image-building method** is stack-specific — and even that has a universal fallback
(buildpacks).

## What we reuse vs. build

- **Reuse the *model*** from `@vercel/frameworks` (declarative detectors) — and the
  library itself if we build in Node.
- **Reuse Dockerfile templates** patterned on `docker init`.
- **Reuse buildpacks** (`pack`/Paketo GitHub Action) for the long-tail/unknown case.
- **Build ourselves**: the **rule registry** (`rules/*.yaml`) and the **workflow template**,
  because the *output* — Robin-Cloud's ECR naming (`<project>-<module>`), deploy API, and
  deploy-token contract — is bespoke and small. This is the part no OSS gives us.

Net: we are not taking on a research problem. We are assembling a well-trodden pattern
around a small, Robin-Cloud-specific output.
