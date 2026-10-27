# Development

## Validation

Requires Go 1.27 or newer. From the repository root:

```sh
gofmt -l cmd internal
go vet ./...
go test -race ./...
go build -o httpp ./cmd/httpp
```

The tests use local HTTP/TLS servers, controlled environment variables, subprocess
helpers, and temporary configuration/cache directories. They cover host selection,
credential isolation, malformed configuration, token endpoint responses, both
OAuth client authentication methods, token cache expiry/concurrency, symlinks,
file modes, redirect boundaries, streaming, and exit codes.

With the Go vulnerability scanner installed:

```sh
govulncheck ./...
```

## Build identity

Unstamped builds report `v0-dev`. A version can be supplied at build time:

```sh
go build -ldflags '-X github.com/stuft2/httpp/internal/cli.Version=v0.0.0' -o httpp ./cmd/httpp
```

This does not create or publish a release. No CI or release workflow is configured
in v0.

## YAML dependency review

The v0 implementation plan explicitly selected `go.yaml.in/yaml/v3 v3.0.5` for
this personal CLI. Upstream is maintained by the YAML organization, with a stable
v3 API and security fixes. Its licenses are MIT and Apache-2.0. It has no transitive
runtime module dependencies and requires no C toolchain, runtime network access,
filesystem access, or subprocess execution for parsing.

Sources:

- [Upstream project and maintenance](https://github.com/yaml/go-yaml).
- [Pinned module metadata](https://github.com/yaml/go-yaml/blob/v3.0.5/go.mod).
- [GoBook's proposed YAML decision](https://github.com/byu-oit/gobook/blob/main/reference/adrs/deps/0018-parse-yaml-in-go-services.md).

The GoBook decision was still proposed at review time; this implementation does
not claim organizational dependency approval. Its parser guidance informed the
bounded input, schema validation, alias/tag rejection, and safe errors used here.
Go's standard library has no YAML parser. JSON would require changing the chosen
format; maintaining a YAML subset parser would add unnecessary grammar and
security maintenance.

The parser is confined to `internal/config`. It decodes application-owned data;
configuration commands run only after validation and host selection. Its primary
risks are resource exhaustion and unexpected interpretation, controlled by input
and nesting limits, rejecting unsupported syntax and fields, and parser tests.
Keep the exact version pinned and rerun vulnerability scanning for changes.
Replacing it requires adapting that parser boundary and retaining its tests; no
cache migration is needed. This review records the existing GoBook proposal
rather than creating a duplicate dependency ADR.
