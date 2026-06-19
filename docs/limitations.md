# Known limitations

rcloud builds each component with **its own folder as an isolated Docker build
context**, via the shared `deploy-component` action (`docker build` or buildpacks).
That model is simple and covers most repos, but it breaks for a few real cases.
This catalogs them with today's workaround and the planned fix.

> The workarounds that say "hand-edit the workflow step" are stopgaps: rcloud
> **overwrites** `.github/workflows/deploy-robin-cloud.yml` on every run, so those edits
> don't survive re-generation. Prefer the planned fixes (or stop re-running rcloud once
> you own the workflow).

## 1. Shared local dependencies across folders (monorepo)

**Symptom:** a component's build fails resolving a sibling package — e.g. a shared
`core`/`common` library referenced by a local path (`path = "../core"` in a uv/pip
project, or a `workspace:*` npm dep pointing outside the folder).

**Why:** the build context is the component folder (`./svc`), so `../core` is *outside*
the context and Docker can't `COPY` it. (pip also doesn't honor uv's
`[tool.uv.sources]`, so a path source silently becomes an unresolvable PyPI lookup.)

**Workaround today:**
- Publish the shared library to a package index (PyPI / private index / npm registry)
  and depend on it normally; **or**
- Hand-edit that component's workflow step to build with the **repo root** as context and
  `file: <folder>/Dockerfile`, and rewrite the Dockerfile for a root context
  (`COPY core/ … && COPY svc/ …`).

**Planned fix:** per-component **build context + dockerfile path** in `robin-deploy.yaml`
(e.g. `build_context: .`, `dockerfile: ./svc/Dockerfile`) wired to action inputs.

## 2. Private dependencies needing SSH or build secrets

**Symptom:** a Dockerfile that installs a private git dependency
(`RUN --mount=type=ssh … git+ssh://…`, or a private index needing a token) builds
locally with `docker build --ssh default` but fails in CI.

**Why:** the `deploy-component` action calls `docker/build-push-action` **without** `ssh:`
or `secrets:` forwarding, so no SSH agent socket or build secret reaches the build.

**Workaround today:** switch to an HTTPS-token install via a BuildKit secret
(`RUN --mount=type=secret,id=tok … git+https://…`) and hand-edit the component's step to
pass `secrets:` (with a read-scoped CI secret); or `ssh: default` + an ssh-agent action
holding a deploy key.

**Planned fix:** `build_secrets` / `ssh` passthrough in `robin-deploy.yaml` → action inputs.

## 3. Frontend build-time env vars are not passed

**Symptom:** a frontend (Vite `VITE_*`, Next `NEXT_PUBLIC_*`) builds, but the production
bundle is missing config (e.g. the API base URL is empty) — those values are **inlined at
build time**, not read at runtime.

**Why:** the action passes no `--build-arg`, so build-time env isn't injected. (Rules
already *declare* `buildArgsFromSecrets`, but it isn't wired into the build yet.)

**Workaround today:** add `ARG`/`ENV` before the build in the Dockerfile and hand-edit the
component's step to pass `build-args` from a repo secret/var.

**Planned fix:** wire `buildArgsFromSecrets` → action `build-args` sourced from secrets/vars.

## 4. Generated Dockerfiles have no `.dockerignore`

**Symptom:** generated Dockerfiles `COPY . .`, so without a `.dockerignore` the image can
include `.venv`, `node_modules`, `dist`, dev databases, and caches — bloat (and
occasionally incorrect, e.g. shipping a dev SQLite file).

**Workaround today:** add a `.dockerignore` to the component folder.

**Planned fix:** generate a stack-appropriate `.dockerignore` next to each Dockerfile.

## 5. The workflow is regenerated (hand edits are lost)

rcloud rewrites `deploy-robin-cloud.yml` on every run (Dockerfiles are never overwritten).
Keep persistent changes in flags / `robin-deploy.yaml`, not in hand edits to the workflow.
