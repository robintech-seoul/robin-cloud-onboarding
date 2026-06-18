# Homebrew formula for rcloud.
#
# This file belongs in a SEPARATE tap repo: github.com/robintech-seoul/homebrew-tap
# at path  Formula/rcloud.rb. Once there, users install with:
#
#     brew install robintech-seoul/tap/rcloud
#     # or, to track the main branch (no release/sha256 needed):
#     brew install --HEAD robintech-seoul/tap/rcloud
#
# Setup (one time), after `gh auth login`:
#   1. push this repo to GitHub and tag a release:
#        git tag v0.1.0 && git push origin main --tags
#   2. compute the tarball sha256 and paste it below:
#        curl -sL https://github.com/robintech-seoul/robin-cloud-onboarding/archive/refs/tags/v0.2.1.tar.gz | shasum -a 256
#   3. create the tap repo and add this file:
#        gh repo create robintech-seoul/homebrew-tap --private --clone
#        # copy this file to homebrew-tap/Formula/rcloud.rb, commit, push
#
# NOTE: if robin-cloud-onboarding is a PRIVATE repo, Homebrew needs git/API auth to
# fetch the source — set HOMEBREW_GITHUB_API_TOKEN or use SSH. For a fully frictionless
# private install, switch to GoReleaser-built release binaries (see packaging/README.md).
class Rcloud < Formula
  desc "Generate GitHub Actions workflows to deploy a repo to Robin-Cloud"
  homepage "https://github.com/robintech-seoul/robin-cloud-onboarding"
  url "https://github.com/robintech-seoul/robin-cloud-onboarding/archive/refs/tags/v0.2.1.tar.gz"
  sha256 "52e9288db505aa118466e563432cf0fade6687b376fc03e21125b5a182f5a0f2"
  license "MIT"
  head "https://github.com/robintech-seoul/robin-cloud-onboarding.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", "-trimpath", "-o", "rcloud", "."
    bin.install "rcloud"
  end

  test do
    # rcloud with no --project exits 1 and explains itself.
    output = shell_output("#{bin}/rcloud 2>&1", 1)
    assert_match "--project is required", output
  end
end
