# Configuration and authentication

## Location and matching

httpp reads `~/.config/httpp/config.yaml`. An absolute `XDG_CONFIG_HOME` changes
the base directory; `--config FILE` selects a specific file. A missing default
file permits unauthenticated requests. A missing explicit file, malformed file,
or invalid entry is an error, including when `--no-auth` is used.

Configuration version is `1`. Under `hosts`, each key is an exact DNS hostname
or IP address, without a scheme, port, or path. Hostnames are case-insensitive
and a trailing DNS dot is ignored. Matching ignores request ports. Wildcards,
duplicate normalized hostnames, and shared named profiles are unsupported.
Unmatched hosts receive no configured authentication.

See [the full example](../examples/config.yaml). Each host contains an `auth`
mapping with one of these types:

| Type | Required fields | Optional fields |
| --- | --- | --- |
| `bearer` | `token` | None |
| `basic` | `username`, `password` | None |
| `oauth2-client-credentials` | `token_url`, `client_id`, `client_secret` | `scopes`, `token_auth_method` |

The parser rejects unknown fields, mixed auth settings, aliases, anchors, custom
YAML tags, and multiple documents. Configuration is limited to 1 MiB and 32
nesting levels. Quote numeric identifiers so YAML treats them as strings.

## Credential sources

`client_id`, `client_secret`, `token`, and `password` accept either a literal YAML
string or an `env` / `command` reference. Environment variables and commands are
recommended for secrets so their values stay outside the configuration file.
A reference contains exactly one source:

```yaml
token:
  env: EXAMPLE_API_TOKEN
```

```yaml
client_secret:
  command: [op, read, "op://Work/Example/client-secret"]
```

Environment variables are read from httpp's environment. Commands run as an
executable and argument array, inherit the environment, and have no stdin. There
is no shell, variable expansion, or tilde expansion. Use a full executable path
when needed. Only put commands you trust in your configuration file.

Command stdout is the credential. Trailing CR/LF characters are removed; an empty
credential fails. Commands have a ten-second timeout (or the remaining overall
timeout if shorter) and a 64 KiB stdout limit. Captured stdout and stderr are not
included in diagnostics. Literal secrets are supported but not recommended:

```yaml
client_id: my-client
client_secret: "example-only-secret"
```

This stores the secret as plaintext in the configuration file. Keep that file
private and out of source control. Empty strings are rejected; quote numeric
values so YAML treats them as strings.

For example, retrieve both client credentials from a password manager:

```yaml
client_id:
  command: [pass-cli, item, view, --vault-name, Work, --item-title, Example API, --field, API Key, --output, human]
client_secret:
  command: [pass-cli, item, view, --vault-name, Work, --item-title, Example API, --field, Secret, --output, human]
```

Both commands run only when a new OAuth token is needed. Literal client IDs such
as `client_id: my-client` remain supported. Neither resolved client IDs nor client
secrets are written to the token cache. Password-manager fields and executable
names are examples; match them to your local installation and vault entries.

## OAuth2 client credentials

`token_url` must be an HTTPS endpoint without URL userinfo or a fragment. httpp
sends a form-encoded POST with `grant_type=client_credentials` and optional
space-separated `scopes`. Client authentication defaults to
`client_secret_basic`, including OAuth's encoding of the client ID and secret.
Set `token_auth_method: client_secret_post` for providers requiring the credentials
in the form body. Token endpoint redirects are never followed.

Responses must include a nonempty `access_token` and a `token_type` of `Bearer`
(case-insensitive). Responses are limited to 1 MiB. A positive integer
`expires_in` enables caching; omitted or invalid expiry values produce a token
usable only for that invocation. Tokens within 30 seconds of expiry are not reused.
No refresh-token or browser authorization flows are supported.

## Token cache

Tokens are stored under `httpp/tokens` in Go's user cache directory:

- macOS: `~/Library/Caches/httpp/tokens`.
- Linux: `$XDG_CACHE_HOME/httpp/tokens`, defaulting to `~/.cache/httpp/tokens`.

Cache keys cover the resource hostname and the OAuth configuration, including
credential source references. They do not contain credential values. Stored
entries contain only a version, access token, and expiry. Client secrets are not
persisted.

On Unix, application cache directories are created with mode `0700` and entries
with mode `0600`; existing entries with group or other access are rejected.
Application cache directory and entry symlinks are rejected. Writes use temporary
files and atomic replacement. Persistent caching is supported on macOS and Linux. On Windows it is disabled
with a warning because ACL enforcement is not implemented; tokens can still be
acquired for the current invocation.

Cache read errors fall back to acquiring a token. Write errors warn without
failing the resource request. Concurrent processes may acquire multiple tokens,
but cannot publish partially written entries. Cache clearing is not coordinated
with in-flight token acquisition; stop running requests before clearing if you
need the cache to remain empty.

```sh
httpp cache clear
```

Clear the cache after rotating client credentials, including command-backed client
IDs: changing a credential value behind the same environment variable or command does not change its cache key.
A 401 response does not automatically refresh the token or replay the request.

## Request behavior

Configured authentication is only sent over HTTPS. An explicit `Authorization`
header bypasses configured authentication and credential lookup; duplicates are
rejected. `--no-auth` disables configured authentication but does not remove an
explicit header. You control the security of credentials supplied through headers.

Redirects are disabled by default. `--location` permits up to ten redirects within
the same scheme, hostname, and effective port; cross-origin redirects fail before
sending the next request. Standard TLS verification stays enabled. HTTP proxy
settings follow Go's `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` behavior.

The overall timeout defaults to 30 seconds and includes credential resolution,
token acquisition, and response streaming. Use `--timeout 5s` or `--timeout 5` to
change it. There are no application-level retries or OAuth-specific replays;
Go's transport can retry eligible requests on reused connections.

`-d` defaults the method to POST; otherwise it defaults to GET. `-X` overrides
that choice. Data is literal text, `@file`, or `@-` for stdin. Supply headers such
as `Content-Type` explicitly; httpp does not infer JSON or form encoding for
resource requests. Multiple `-d` arguments are rejected.

A literal body can be replayed for a same-origin 307/308 redirect. File and stdin
bodies are streamed and not replayed; Go returns such redirect responses without
following when replay would be required. Output is streamed, so errors can leave
partial stdout or output files. `--include` writes final response headers to the
same destination as the body; `--output -` selects stdout.

Options may appear before or after the URL. Long value options also accept
`--name=value`. Short flags must be separate arguments; curl's combined flags and
full option vocabulary are not supported.
