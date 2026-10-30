# knok

## Overview

knok is an HTTP command-line client with authentication selected automatically
from exact-host entries in a YAML configuration file. It supports Bearer tokens,
Basic authentication, and OAuth2 client credentials with a persistent token cache.

**Status:** v0 development. The configuration and command interface may change.

## Installation

### Prebuilt binaries (no Go required)

Download the archive matching your operating system and CPU from the
[latest release](https://github.com/stuft2/knok/releases/latest). Releases include
macOS, Linux, and Windows binaries for amd64 (Intel/AMD) and arm64 (including
Apple Silicon).

On macOS or Linux, extract the `.tar.gz` archive, then run these commands from the
extracted directory:

```sh
mkdir -p "$HOME/.local/bin"
install -m 0755 knok "$HOME/.local/bin/knok"
export PATH="$HOME/.local/bin:$PATH"
knok --version
```

Add that `export PATH` line to your shell startup file to keep it available in new
terminals. On Windows, extract the `.zip` and add the folder containing
`knok.exe` to your user `Path`.

See the [installation guide](docs/installation.md) for platform selection,
download commands, checksum verification, and Windows setup.

### With Go

Requires Go 1.27 or newer:

```sh
go install github.com/stuft2/knok/cmd/knok@latest
knok_bin="$(go env GOBIN)"
export PATH="${knok_bin:-$(go env GOPATH)/bin}:$PATH"
knok --help
```

Go installs to `GOBIN` when configured, otherwise to the `bin` directory under
`GOPATH`; make sure that directory is on your `PATH`. Use `@main` instead of
`@latest` to install the current development branch.

## How to Use This

```sh
knok https://api.example.com/users
knok -X POST -H 'Content-Type: application/json' -d '{"name":"Sam"}' https://api.example.com/users
knok --location --include https://example.com
knok -d @payload.json --output response.json https://api.example.com/users
```

Put authentication settings in `~/.config/knok/config.yaml` (or
`$XDG_CONFIG_HOME/knok/config.yaml`):

```yaml
version: 1
hosts:
  api.example.com:
    auth:
      type: bearer
      token:
        env: EXAMPLE_API_TOKEN
```

Secrets can come from environment variables, external commands, or literal YAML
strings. Environment variables and commands are recommended. See
[configuration and authentication](docs/configuration.md)
and the [example configuration](examples/config.yaml) for all three auth methods.

Use `knok --help` for options and `knok cache clear` to discard cached OAuth
tokens. Response bodies go to stdout; diagnostics go to stderr. Completed HTTP
responses, including 4xx and 5xx, exit successfully. Runtime failures exit 1;
invalid arguments or configuration exit 2.

## How to Build Locally

Requires Go 1.27 or newer.

```sh
git clone git@github.com:stuft2/knok.git
cd knok
go build -o knok ./cmd/knok
./knok --help
```

## How to Publish

Release Please opens a version/changelog PR from Conventional Commits on `main`.
Merging that PR creates a GitHub release with binaries and checksums for macOS,
Linux, and Windows (amd64 and arm64). See [release instructions](docs/releases.md).

## Architectural Overview

The CLI composes configuration parsing, credential resolution, OAuth token caching,
and Go's `net/http` client. YAML parsing is the only third-party runtime dependency.
Configured authentication requires HTTPS. Redirect following is opt-in and limited
to the original origin. No curl executable, libcurl, browser login, wildcard host
matching, or automatic request retries are used in v0.

## How to Test

```sh
go test -race ./...
go vet ./...
```

CI runs formatting checks, `go vet`, race-enabled tests, and a build on pull
requests and on `main` before release processing. Tests use temporary directories
and local HTTP/TLS servers; no external API credentials are required. See
[development notes](docs/development.md) for the vulnerability check and dependency
review.

## Security

See the [security policy](SECURITY.md) for supported versions and private
vulnerability reporting.

## License

[MIT](LICENSE) © 2026 Spencer Tuft.
