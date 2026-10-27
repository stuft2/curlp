# Installing httpp

## Choose an archive

Prebuilt binaries do not require Go. Download from the
[latest release](https://github.com/stuft2/httpp/releases/latest).

| System | CPU | Archive suffix |
| ------ | --- | -------------- |
| macOS | Apple Silicon | `darwin_arm64.tar.gz` |
| macOS | Intel | `darwin_amd64.tar.gz` |
| Linux | x86-64 / Intel / AMD | `linux_amd64.tar.gz` |
| Linux | ARM64 / aarch64 | `linux_arm64.tar.gz` |
| Windows | x64 / Intel / AMD | `windows_amd64.zip` |
| Windows | ARM64 | `windows_arm64.zip` |

On macOS and Linux, use `uname -m` to check the architecture: `x86_64` means
`amd64`; `arm64` or `aarch64` means `arm64`. On Windows, check **Settings > System >
About > System type**.

## macOS and Linux

This example installs `v0.1.0`. Change `release_version` to another published tag
when upgrading, and set `release_platform` using the table above. For example,
use `darwin_arm64` on Apple Silicon or `linux_amd64` on an x86-64 Linux machine.

```sh
release_version=v0.1.0
release_platform=darwin_arm64
release_archive="httpp_${release_version}_${release_platform}.tar.gz"
release_base="https://github.com/stuft2/httpp/releases/download/$release_version"
install_tmp="$(mktemp -d)"

curl -fL "$release_base/$release_archive" -o "$install_tmp/$release_archive"
curl -fL "$release_base/checksums.txt" -o "$install_tmp/checksums.txt"

# Verify only the downloaded archive against the published SHA-256 list.
(
  cd "$install_tmp" || exit 1
  awk -v file="$release_archive" '$2 == file { print }' checksums.txt > selected-checksum.txt
  test -s selected-checksum.txt || exit 1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum --check selected-checksum.txt
  else
    shasum -a 256 --check selected-checksum.txt
  fi
) &&
tar -xzf "$install_tmp/$release_archive" -C "$install_tmp" &&
mkdir -p "$HOME/.local/bin" &&
install -m 0755 "$install_tmp/httpp" "$HOME/.local/bin/httpp"
```

After the checksum and install commands succeed:

```sh
export PATH="$HOME/.local/bin:$PATH"
httpp --version
```

Add `export PATH="$HOME/.local/bin:$PATH"` to `~/.zshrc` for Zsh or `~/.bashrc`
for interactive Bash sessions, then open a new terminal. If you already keep
`~/.local/bin` on `PATH`, no change is needed. The temporary download directory
can be removed after installation.

## Windows

Download the matching `.zip`, verify its SHA-256 with `Get-FileHash`, and compare
it with the matching filename in the release's `checksums.txt`:

```powershell
Get-FileHash .\httpp_v0.1.0_windows_amd64.zip -Algorithm SHA256
```

Extract the archive. From the extracted directory, install into a user-owned
folder:

```powershell
$HttppInstallDir = Join-Path $env:LOCALAPPDATA 'Programs\httpp'
New-Item -ItemType Directory -Force -Path $HttppInstallDir | Out-Null
Copy-Item .\httpp.exe (Join-Path $HttppInstallDir 'httpp.exe') -Force
$env:Path = "$HttppInstallDir;$env:Path"
httpp --version
```

For future terminals, add `%LOCALAPPDATA%\Programs\httpp` to the **user** `Path`
through **Edit environment variables for your account**, then reopen your
terminal. Persistent OAuth token caching is currently disabled on Windows;
requests can still acquire tokens for the current invocation.

## Install with Go

With Go 1.27 or newer:

```sh
go install github.com/stuft2/httpp/cmd/httpp@latest
```

Find the install directory with `go env GOBIN`. If that is empty, use the `bin`
directory under `go env GOPATH`. Put that directory on `PATH`, then verify the
command is available with `httpp --help`.

Use `@v0.1.0` to pin that version or `@main` for the development branch. Go
installations currently report the default `v0-dev` version label; downloaded
release binaries contain the release version stamp.

## Configure authentication

Installation does not create or overwrite your authentication configuration.
See [configuration and authentication](configuration.md) to set up host entries
and credential sources. An unmatched host can be called without configuration:

```sh
httpp https://example.com
```

To upgrade, repeat the same installation method with a newer release. Existing
configuration and cached tokens are preserved.
