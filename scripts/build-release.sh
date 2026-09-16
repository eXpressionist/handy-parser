#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 VERSION OUTPUT_DIR" >&2
  exit 2
fi

version="$1"
output_dir="$2"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
package_name="handy-parser-${version}"
stage_dir="${output_dir}/${package_name}"
archive="${output_dir}/${package_name}.tar.gz"

if [[ -e "$stage_dir" || -e "$archive" ]]; then
  echo "release output already exists: $package_name" >&2
  exit 1
fi

mkdir -p "$stage_dir"
cp "$root_dir/compose.yaml" "$stage_dir/compose.yaml"
cp "$root_dir/.env.example" "$stage_dir/.env.example"
cp "$root_dir/.dockerignore" "$root_dir/Dockerfile" "$root_dir/go.mod" "$root_dir/go.sum" "$stage_dir/"
cp -R "$root_dir/cmd" "$root_dir/internal" "$stage_dir/"
cp "$root_dir/docs/INSTALLATION.md" "$stage_dir/INSTALLATION.md"
printf '%s\n' "$version" > "$stage_dir/VERSION"

tar -C "$output_dir" -czf "$archive" "$package_name"
(
  cd "$output_dir"
  sha256sum "${package_name}.tar.gz" > "${package_name}.tar.gz.sha256"
)

printf '%s\n' "$archive"
