#!/usr/bin/env bash
set -euo pipefail

release_tag="${1:?Usage: bash scripts/build-release.sh v0.1.0}"
if [[ ! "$release_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Expected a release tag such as v0.1.0' >&2
  exit 1
fi

cd "$(dirname "$0")/.."
mkdir -p dist
staging_dir="$(mktemp -d)"
trap 'rm -rf "$staging_dir"' EXIT
archives=()

for target_os in darwin linux windows; do
  for target_arch in amd64 arm64; do
    archive_base="httpp_${release_tag}_${target_os}_${target_arch}"
    package_dir="$staging_dir/$archive_base"
    mkdir -p "$package_dir"
    binary_name=httpp
    if [[ "$target_os" == windows ]]; then
      binary_name=httpp.exe
    fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath \
        -ldflags "-s -w -X github.com/stuft2/httpp/internal/cli.Version=$release_tag" \
        -o "$package_dir/$binary_name" ./cmd/httpp
    cp LICENSE README.md SECURITY.md "$package_dir/"
    cp -R docs examples "$package_dir/"
    if [[ "$target_os" == windows ]]; then
      archive_path="$PWD/dist/$archive_base.zip"
      # zip updates existing archives; remove only this exact generated file.
      rm -f "$archive_path"
      (cd "$package_dir" && zip -qr "$archive_path" .)
    else
      archive_path="$PWD/dist/$archive_base.tar.gz"
      tar -czf "$archive_path" -C "$package_dir" .
    fi
    archives+=("${archive_path##*/}")
  done
done

(cd dist && if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${archives[@]}" > checksums.txt
else
  shasum -a 256 "${archives[@]}" > checksums.txt
fi)
