# Release and Package Publishing

Disbug CLI ships as a single binary named `disbug`.

Package names:

- Homebrew formula: `disbug`
- Homebrew tap repo: `disbug-io/homebrew-tap`
- Homebrew install command: `brew install disbug-io/tap/disbug`
- Scoop manifest: `disbug`
- Scoop bucket repo: `disbug-io/scoop-bucket`
- Scoop install commands:

```powershell
scoop bucket add disbug https://github.com/disbug-io/scoop-bucket
scoop install disbug
```

## First-Time Setup

Create or verify these GitHub repositories:

- `github.com/disbug-io/homebrew-tap`
- `github.com/disbug-io/scoop-bucket`

Add these repository secrets to `github.com/disbug-io/disbug-cli`:

- `HOMEBREW_TAP_TOKEN`: token with write access to `disbug-io/homebrew-tap`
- `SCOOP_BUCKET_TOKEN`: token with write access to `disbug-io/scoop-bucket`

## Validate Locally

Run these before tagging a release:

```bash
make ci
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean
```

The snapshot command builds local artifacts only. It does not publish packages.

## Publish

1. In `CHANGELOG.md`, move the `[Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading and merge that to `main`. This section becomes the GitHub release notes. Preview it with:

   ```bash
   ./scripts/release-notes.sh vX.Y.Z
   ```

2. Push a semver tag:

   ```bash
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

The `release` workflow stops before building if `CHANGELOG.md` has no section for the tag. Otherwise it builds the GitHub release artifacts, publishes that section as the release notes, and updates the Homebrew formula and Scoop manifest.

You can also rerun a release from GitHub Actions using the manual `workflow_dispatch` input with an existing tag, for example `v0.1.0`. A rerun reads the notes from `CHANGELOG.md` on the branch you start it from (normally `main`). GoReleaser keeps an existing release's notes and only fills them in when they are empty.
