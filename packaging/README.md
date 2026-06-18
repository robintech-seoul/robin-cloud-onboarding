# Distributing `rcloud`

Three ways to install, simplest first.

## 1. Build to your PATH (no Homebrew)

```bash
go build -o "$(brew --prefix)/bin/rcloud" .   # or any dir on $PATH
```

## 2. Homebrew tap, build-from-source (simplest distribution)

A tap is a GitHub repo named `homebrew-<name>`. Formula lives at `Formula/rcloud.rb`.

```bash
gh auth login                                   # once

# publish this repo + a release tag
git push origin main
git tag v0.1.0 && git push origin --tags

# fill the formula's sha256
curl -sL https://github.com/robintech-seoul/robin-cloud-onboarding/archive/refs/tags/v0.1.0.tar.gz \
  | shasum -a 256

# create the tap and add the formula
gh repo create robintech-seoul/homebrew-tap --private --clone
cp packaging/homebrew/rcloud.rb homebrew-tap/Formula/rcloud.rb
# (paste the sha256), then commit + push the tap

# users install with:
brew install robintech-seoul/tap/rcloud
# or track main directly:
brew install --HEAD robintech-seoul/tap/rcloud
```

The formula (`packaging/homebrew/rcloud.rb`) installs Go as a build dependency and
compiles from source — no prebuilt binaries to manage.

**Private-repo caveat:** if `robin-cloud-onboarding` is private, Homebrew must authenticate
to fetch the source (set `HOMEBREW_GITHUB_API_TOKEN`, or use SSH). For a frictionless
private install, prefer option 3.

## 3. GoReleaser → prebuilt release binaries (best for non-Go users / private repos)

[GoReleaser](https://goreleaser.com) cross-compiles `rcloud` for macOS/Linux × amd64/arm64,
attaches the binaries to a GitHub Release, **and updates the tap formula automatically** on
every `git tag`. Users then install a prebuilt binary (no Go toolchain needed). This is the
recommended end state; the build-from-source tap (option 2) is the quick start. A
`.goreleaser.yaml` can be added when we're ready to cut versioned releases.
