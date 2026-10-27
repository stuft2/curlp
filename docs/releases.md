# CI and releases

## Continuous integration

Pull requests run the **CI / Lint, test, build** check on an Ubuntu runner:

1. Require `gofmt` formatting, run `go vet ./...`, and check release-script syntax.
2. Run `go test -race -timeout 120s ./...`.
3. Build the CLI with `go build -trimpath -o httpp ./cmd/httpp`.

Pushes to `main` run the same reusable CI job inside the Release workflow before
Release Please executes. CI can also be run manually using Actions > CI > Run
workflow. Superseded PR runs are canceled; release runs are serialized.

## Release flow

Use Conventional Commit titles when squash-merging PRs:

- `feat: ...` increments the minor version.
- `fix: ...` increments the patch version.
- Breaking changes increment the minor version while the project is below 1.0.
- Documentation, CI, and other maintenance changes do not independently trigger
  a release, but can be included alongside release-worthy changes.

Release Please manages `CHANGELOG.md`, `.release-please-manifest.json`, release
PRs, tags, and GitHub releases. The manifest starts at `0.0.0`, with
`initial-version` explicitly set to `0.1.0` for the first release. Subsequent releases remain in
the `0.x` series until a deliberate transition to 1.0 is configured.

1. Merge feature/fix PRs into `main`; CI must pass before release processing.
2. Review and merge the Release Please PR containing the version/changelog update.
3. On that merge, CI runs again, then Release Please creates the version tag and
   GitHub release.
4. The same workflow checks out the exact release tag, cross-compiles the binaries,
   stamps `httpp --version`, and attaches six archives plus `checksums.txt`.

Artifacts cover macOS (`darwin`), Linux, and Windows on amd64 and arm64. Unix
archives are `.tar.gz`; Windows archives are `.zip`. Each includes the binary,
README, documentation, and example configuration. Windows retains v0's existing
limitation: persistent OAuth token caching is disabled.

The workflow uses the built-in `GITHUB_TOKEN`; no personal access token or external
publishing credentials are required. Repository Settings > Actions > General >
**Allow GitHub Actions to create and approve pull requests** must be enabled.
The workflow creates release PRs but does not approve or merge them.

GitHub suppresses automatic workflow events from changes made using this token.
The release workflow explicitly dispatches CI on each created or updated release
PR branch, using GitHub's supported `workflow_dispatch` exception. A separate job
has only `actions: write` for this dispatch. The post-merge CI gate also runs
before tagging. Artifact publication is in the same workflow so it does not rely
on a suppressed `release` event.

See [Release Please's token documentation](https://github.com/googleapis/release-please-action#other-actions-on-release-please-prs).

## Build archives locally

Requires Go 1.27+, Bash, `tar`, `zip`, and either `sha256sum` or `shasum`:

```sh
bash scripts/build-release.sh v0.1.0
```

This builds six archives under `dist/` and a SHA-256 checksum list. It does not
create tags, change the manifest, or publish anything. Local cross-builds compile
all targets; only the host-compatible binary can be executed locally.

On Linux, verify downloaded files with:

```sh
sha256sum --check checksums.txt
```

On macOS, use `shasum -a 256 --check checksums.txt` instead. Download all archives
listed in the checksum file when checking the full list.

## Retry asset publication

A build/upload failure can leave a GitHub release without all its binaries. Retry
the failed job in Actions, or run the Release workflow manually **from main** and
supply an existing `release_tag`, such as `v0.1.0`. Recovery verifies that the
GitHub release exists, builds from its tag, and replaces assets of the same names.
It does not ask Release Please to create another version.

Release workflows use pinned action commits, deny permissions by default, grant
write permissions only to jobs that create PRs/releases, dispatch CI, or upload
assets, and do
not persist checkout credentials. No privileged fork-PR trigger is used.
