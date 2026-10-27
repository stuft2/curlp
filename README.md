# httpp

## Overview

httpp is an HTTP command-line client with authentication selected automatically
from exact-host entries in a YAML configuration file. It supports Bearer tokens,
Basic authentication, and OAuth2 client credentials with a persistent token cache.

**Status:** v0 development. The configuration and command interface may change.

## How to Use This

```sh
httpp https://api.example.com/users
httpp -X POST -H 'Content-Type: application/json' -d '{"name":"Sam"}' https://api.example.com/users
httpp --location --include https://example.com
httpp -d @payload.json --output response.json https://api.example.com/users
```

Put authentication settings in `~/.config/httpp/config.yaml` (or
`$XDG_CONFIG_HOME/httpp/config.yaml`):

```yaml
version: 1
hosts:
  api.example.com:
    auth:
      type: bearer
      token:
        env: EXAMPLE_API_TOKEN
```

Secrets come from environment variables or external commands, rather than literal
values in the file. See [configuration and authentication](docs/configuration.md)
and the [example configuration](examples/config.yaml) for all three auth methods.

Use `httpp --help` for options and `httpp cache clear` to discard cached OAuth
tokens. Response bodies go to stdout; diagnostics go to stderr. Completed HTTP
responses, including 4xx and 5xx, exit successfully. Runtime failures exit 1;
invalid arguments or configuration exit 2.

## How to Build Locally

Requires Go 1.27 or newer.

```sh
git clone git@github.com:stuft2/httpp.git
cd httpp
go build -o httpp ./cmd/httpp
./httpp --help
```

Alternatively, install the current main branch:

```sh
go install github.com/stuft2/httpp/cmd/httpp@main
```

## How to Publish

No tagged release or automated distribution process is established yet. See
[development notes](docs/development.md) for validation and version stamping.

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

Tests use temporary directories and local HTTP/TLS servers; no external API
credentials are required. See [development notes](docs/development.md) for the
vulnerability check and dependency review.
