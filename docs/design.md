# Design

How the tool turns a repo into a Robin-Cloud deployment workflow. Read
[`detection-approaches.md`](detection-approaches.md) first for the *why*; this is the *what*.

## Pipeline

```
  repo ──▶ 1. scan ──▶ 2. find candidate components ──▶ 3. match rules ──▶
          4. resolve project plan ──▶ 5. confirm/override ──▶ 6. render ──▶ files
```

1. **Scan** — walk the repo (respect `.gitignore`), collect: file paths, parsed manifests
   (`package.json`, `pyproject.toml`/`requirements.txt`, `go.mod`, …), lockfiles,
   workspace markers, and every `Dockerfile`.
2. **Find candidate components** — in fidelity order:
   - any directory containing a `Dockerfile` → a candidate (the strongest signal);
   - else, if workspace markers exist → each app/package dir → a candidate;
   - else → the repo root is the single candidate.
3. **Match rules** — for each candidate, evaluate every rule's `detect` block; pick the
   highest-scoring match (ties broken by `priority`). A candidate with a Dockerfile but no
   rule match is still a valid component (Docker is the contract).
4. **Resolve project plan** — assign each component a `module` name (default = rule's
   `suggestedModule`, deduped), build context, port, exposed-or-not, and whether a
   Dockerfile must be generated. Produce a single `DeployPlan`.
5. **Confirm / override** — print the plan; the user accepts, or edits a
   **`robin-deploy.yaml`** override file in the repo (the per-project custom config) and
   re-runs. This is the one interaction, and it is deterministic — not a chat.
6. **Render** — apply templates to the plan → write files.

## The custom rule system

Two layers of "custom rules", matching your two asks:

### a) The stack registry — `rules/*.yaml` (extensible detection → generation)

One file per stack profile. Adding support for a new stack = adding a file, **no engine
code change**. The schema is modeled on `@vercel/frameworks` detectors.

```yaml
id: react-vite                 # unique
name: React (Vite)
kind: web                      # web | service | worker   (web → exposed via ingress)
priority: 50                   # higher wins ties

detect:                        # all/any/none of these conditions
  all:
    - file: package.json
  any:
    - dependency: { manifest: package.json, name: vite }
    - file: vite.config.ts
    - file: vite.config.js
  none:
    - file: next.config.js     # negative guard: Next is a different rule

component:
  suggestedModule: web         # → ECR repo <project>-web, deploy component "web"
  defaultPort: 80

dockerfile:
  ifMissing: react-vite        # template id under templates/dockerfile/ (skip if repo has one)
  buildArgsFromSecrets: ["VITE_*"]   # public envs baked at build time, sourced from GH secrets

build:                         # optional CI quality gate before the image build
  install: "{{pm}} install --frozen-lockfile"
  command: "{{pm}} build"
  test: "{{pm}} test"
```

**Condition types** (each contributes to the match score):
- `file: <path-or-glob>` — exists
- `dependency: { manifest, name }` — name appears in that manifest's deps
- `content: { file, matches }` — regex against a file's contents
- combined under `all` (required), `any` (≥1), `none` (must be absent)

Starter rules in this repo: `react-vite`, `fastapi`, `go-service`. These cover the stacks
visible in the Robin-Cloud codebase itself; grow the registry from real customer repos.

### b) The per-project override — `robin-deploy.yaml` (checked into the customer repo)

When detection is ambiguous (monorepos especially) or the user wants explicit control,
this file pins the plan and the engine skips inference for whatever it specifies:

```yaml
project: acme                  # Robin-Cloud project name (defaults from repo/console)
region: ap-northeast-2
components:
  - module: api                # → ECR repo acme-api, deploy component "api"
    context: ./backend
    dockerfile: ./backend/Dockerfile   # omit to auto-generate from the matched rule
    port: 8000
    expose: false
  - module: web
    context: ./frontend
    port: 80
    expose: true
    buildArgs: { VITE_API_BASE: "https://acme.robintech.cloud" }
```

The same file doubles as the audit of what was generated, and makes re-generation idempotent.

## Robin-Cloud output contract (verified against `robin-cloud` source)

The generated workflow MUST obey these, or the platform rejects the deploy:

- **ECR**: registry `<AWS_ACCOUNT_ID>.dkr.ecr.<region>.amazonaws.com` — the account ID is
  supplied at deploy time via the `AWS_ACCOUNT_ID` GitHub Actions secret, never hardcoded;
  one repo per component named **`<project>-<module>`**. The console's discovery finds
  repos by the `<project>-` prefix and strips it to suggest the module name — so the repo
  name and the module name must agree.
- **Deploy API**: `POST {console}/api/v1/projects/<project>/deployments`
  with header `Authorization: Bearer <ROBIN_DEPLOY_TOKEN>` and body
  `{"component": "<module>", "tag": "<image-tag>"}`. Validated server-side by the deploy API:
  - `component` must match `^[A-Za-z0-9_-]+$` **and** be a top-level key in the project's
    gitops `values.yaml` (else `422 unknown_component`);
  - `tag` must match `^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$` **and** exist in ECR
    (else `422 tag_not_found`);
  - project must be onboarded in gitops (else `404 project_not_onboarded`).
- **Component name = values.yaml key = deploy component = `<module>`.** Keep these
  identical end-to-end. (Some pre-existing projects have a module↔repo-name mismatch that
  predates ECR auto-discovery; new projects should keep the names matching.)
- **404 is expected before the console's deploy-config has run.** The generated `deploy`
  job treats `404` as a warning (push the image, skip the bump) so the first push succeeds;
  once the operator runs deploy-config in the console, later pushes deploy normally.
- **Auth = GitHub OIDC**, no static keys: `permissions: id-token: write` +
  `aws-actions/configure-aws-credentials` assuming
  `arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/${{ secrets.ROBIN_OIDC_ROLE }}`. The
  account ID and the role name are both supplied via GitHub Actions secrets, so neither the
  generated workflow nor this tool hardcodes Robin-Cloud account identifiers. Required
  secrets: `AWS_ACCOUNT_ID`, `ROBIN_OIDC_ROLE`, `ROBIN_DEPLOY_TOKEN` (+ any build-arg
  secrets). (`--oidc-role <name>` can bake a literal role instead, for operator use.)

### One-time platform prerequisites (NOT done by this tool — see `ROBIN_ONBOARDING.md`)

- The OIDC role's **trust policy** must list the customer repo's GitHub org/repo.
  Onboarding a repo from a new GitHub org needs a one-time trust-policy addition by an
  operator (done in AWS, outside this tool).
- The **ECR repos must exist** before the first push (ECR does not auto-create on push)
  and before the console can discover them. Created by `setup-ecr.sh` (now) or a future
  console endpoint.
- **Project + deploy token** created in the console; token stored as the repo secret
  `ROBIN_DEPLOY_TOKEN`.

## Open decisions

1. **Language / runtime of the engine** — the one decision gating implementation:
   - **Go** — single static binary, runs in any CI/repo with zero runtime, matches
     `robin-cli`. Weaker OSS detection ecosystem (we'd port the rule engine ourselves —
     which we're doing anyway). *Best for distribution.*
   - **TypeScript/Node** — `npx`-able, can directly reuse `@vercel/frameworks` /
     `@netlify/framework-info`, familiar to the frontend team. Needs Node at runtime.
     *Best for speed-of-build + OSS reuse.*
   - Either way the rules (`rules/*.yaml`) and templates are language-neutral.
2. **Dockerfile generation vs. buildpacks** for the no-Dockerfile case — start with
   templates for the known stacks; add a buildpack fallback (`pack`/Paketo action) for the
   long tail. (See `detection-approaches.md` § "Two generation strategies".)
3. **ECR repo creation** — `setup-ecr.sh` now (zero backend); optionally a console endpoint
   later. Deliberately decoupled from this tool to avoid a local→console auth dependency.
