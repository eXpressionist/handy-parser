#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 VERSION OUTPUT_DIR IMAGE_REFERENCE" >&2
  exit 2
fi

version="$1"
output_dir="$2"
image_reference="$3"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
package_name="handy-parser-${version}"
stage_dir="${output_dir}/${package_name}"
archive="${output_dir}/${package_name}.tar.gz"

if [[ -e "$stage_dir" || -e "$archive" ]]; then
  echo "release output already exists: $package_name" >&2
  exit 1
fi

mkdir -p "$stage_dir"
sed "s|ghcr.io/expressionist/handy-parser:v0.1.0|${image_reference}|" \
  "$root_dir/deploy/compose.yaml.example" > "$stage_dir/compose.yaml"
cp "$root_dir/deploy/.env.example" "$stage_dir/.env.example"
cp "$root_dir/deploy/handy-parser-check.service.example" "$stage_dir/handy-parser-check.service"
cp "$root_dir/deploy/handy-parser-check.timer.example" "$stage_dir/handy-parser-check.timer"
cp "$root_dir/docs/INSTALLATION.md" "$stage_dir/INSTALLATION.md"
printf '%s\n' "$version" > "$stage_dir/VERSION"

tar -C "$output_dir" -czf "$archive" "$package_name"
(
  cd "$output_dir"
  sha256sum "${package_name}.tar.gz" > "${package_name}.tar.gz.sha256"
)

printf '%s\n' "$archive"
