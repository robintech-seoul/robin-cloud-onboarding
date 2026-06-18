# Onboarding UX: eliminating manual secret setup

## Problem

A repo onboarded to Robin-Cloud needs GitHub Actions secrets set by hand —
`AWS_ACCOUNT_ID`, `ROBIN_OIDC_ROLE`, `ROBIN_DEPLOY_TOKEN`. That's fine for a
first-party repo, but manual secret entry is a poor experience once Robin-Cloud
opens to the public. This doc is the plan to drive that to zero.

## What is actually secret?

| Value | Secret? | Notes |
|---|---|---|
| `AWS_ACCOUNT_ID` | **No** | Not a credential. AWS's own guidance: account IDs are not secret, but "treat as such by not sharing unnecessarily" (defense in depth). Already appears in every ECR image URI. |
| `ROBIN_OIDC_ROLE` | **No** | A role *name*, an identifier — not a credential. |
| `ROBIN_DEPLOY_TOKEN` | **Yes** | A bearer token authorizing deploys for one project. The only true secret. |

**Load-bearing principle: security must not depend on the secrecy of the account
ID or role name.** With a correctly scoped IAM trust policy (OIDC `sub`/`aud`
conditions), an attacker who knows both still cannot assume the role — the boundary
is the trust policy, not the obscurity of the identifiers.

### Is plaintext acceptable?

Yes, with one rule:

- As GitHub Actions **Variables/Secrets** (plaintext-in-settings, visible only to
  repo collaborators) → fine.
- **Do not hardcode them in a *public* reusable action's source** — a public action
  is world-readable, so that broadcasts them unnecessarily. A public action must
  contain **logic only**; account ID and role are passed as **inputs** from the
  caller's secrets/vars. Region + console URL are genuinely non-sensitive → fine as
  baked defaults.

## The levers (each removes more manual work)

### Lever 1 — reusable composite action (BUILT)

Robin-Cloud publishes `.github/actions/deploy-component`. The generated workflow
becomes a thin caller: a `changes` job (path filter per component) + a matrix
`deploy` job that calls the action per changed component, passing inputs + secrets.

- **Centralizes the deploy logic** — bump action versions / fix bugs once, not in
  every customer repo.
- **Keeps account-specific values out of the action's source** — `aws-account-id`
  and `oidc-role` are inputs (sourced from the caller's secrets/vars), never
  hardcoded. `region` and `console-url` default in the action.

Still requires the three secrets on the caller repo — Levers 2 & 3 remove that.

### Lever 2 — GitHub App (auto-provision)

A Robin-Cloud GitHub App the user installs on their repo (the Vercel/Netlify model):

- **reads the repo** → server-side detection (rcloud's engine moves here),
- **opens a PR** with the caller workflow (+ Dockerfile),
- **writes the deploy token** straight into the repo's Actions secrets (GitHub API,
  encrypted with the repo's public key); can also set `AWS_ACCOUNT_ID` /
  `ROBIN_OIDC_ROLE` as repo **Variables**.

→ "install app + pick repo." No manual secret entry.

### Lever 3 — keyless OIDC (eliminate the last secret)

- **Deploy API accepts the workflow's GitHub OIDC JWT** (validate GitHub's
  signature + the `repository` claim → connected-project lookup) instead of a bearer
  token → **no deploy-token secret**.
- **ECR push via OIDC-brokered short-lived STS creds** — the workflow exchanges its
  OIDC token at a Robin-Cloud endpoint for credentials scoped to that project's
  `<project>-*` repos → **no per-repo IAM trust edits** (which don't scale to public
  customers) and no static keys.

→ zero secrets, zero IAM edits. Depends on a **trustworthy repo↔project mapping**,
which the App install proves (today `github_url` is just a user-typed string).

## Recommended end state

**Lever 2 + Lever 3**: install the App, pick the repo, done — no secrets, no IAM
edits. Ladder: Lever 1 now → GitHub App before going public → keyless OIDC for polish.

## rcloud's place

rcloud's rule-based detection engine is the shared core — invoked locally by the CLI
today, and server-side by the console/App later when it generates and PRs a workflow.
The platform side (App, OIDC-validating deploy endpoint, STS broker) is owned by the
console/backend, consistent with `docs/design.md`.
