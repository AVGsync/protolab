#!/usr/bin/env bash
# Generates THIRD_PARTY_NOTICES.md: the licenses of everything that ships inside a ProtoLab archive.
#
#   * Go modules linked into the binaries. The list is the union of `go list -deps` over every target,
#     because it differs per GOOS/GOARCH (x/termios is linked on linux/darwin only, and so on);
#   * the Go standard library and runtime (BSD-3-Clause, the notice must travel with binaries);
#   * npm packages bundled into the embedded web UI: the production dependency tree (React, TanStack,
#     Phosphor icons, the IBM Plex fonts under the SIL OFL, ...) plus Tailwind CSS, a build tool whose
#     preflight stylesheet is copied into the bundle.
#
# Usage:
#   scripts/notices.sh OUTPUT [GOOS/GOARCH ...]    # no targets = the host platform only
#
# Needs go, node and frontend/node_modules (`npm ci` in frontend/). `make notices` and
# `make release-local` call it with the six release targets.
#
# Fails (instead of writing a partial file) if a module or package has no license text at all.
set -euo pipefail
cd "$(dirname "$0")/.."

die() { echo "notices.sh: $*" >&2; exit 1; }

[ $# -ge 1 ] || die "usage: scripts/notices.sh OUTPUT [GOOS/GOARCH ...]"
out="$1"
shift
[ $# -gt 0 ] || set -- "$(go env GOOS)/$(go env GOARCH)"

[ -d frontend/node_modules ] || die "frontend/node_modules is missing: run npm ci in frontend/ first"
command -v node > /dev/null || die "node is required"

# Build tools (devDependencies) whose code is copied into the bundle.
# Not vite: its LICENSE.md carries ~700 lines for the dependencies bundled into vite itself, and only a
# few-line preload helper reaches the output.
bundled_tools="tailwindcss"

# emit_licenses DIR: prints every license-like file of DIR (non-recursive) as a text block.
# Returns 1 if there is none.
emit_licenses() {
  local dir="$1" file found=1
  while IFS= read -r file; do
    found=0
    echo "**$(basename "$file")**"
    echo
    echo '```text'
    cat "$file"
    # keep the closing fence on its own line even if the file has no trailing newline
    [ -z "$(tail -c 1 "$file")" ] || echo
    echo '```'
    echo
  done < <(find "$dir" -maxdepth 1 -type f \( -iname 'LICEN[CS]E*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \
    -o -iname 'OFL*' -o -iname 'PATENTS*' \) | LC_ALL=C sort)
  return "$found"
}

# One tab-separated line per module: path, version, directory in the module cache.
go_modules() {
  local target
  for target in "$@"; do
    CGO_ENABLED=0 GOOS="${target%/*}" GOARCH="${target#*/}" \
      go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}{{end}}' ./cmd/protolab
  done | sed '/^$/d' | LC_ALL=C sort -u
}

emit_npm_package() {
  local dir="$1" meta name version license
  meta="$(node -p "const p = require(process.argv[1]);
    [p.name, p.version, typeof p.license === 'string' ? p.license : ((p.license && p.license.type) || '')].join('\t')" \
    "$dir/package.json")"
  IFS=$'\t' read -r name version license <<< "$meta"
  echo "### $name $version"
  echo
  if ! emit_licenses "$dir"; then
    [ -n "$license" ] || die "npm package $name $version has neither a license file nor a license field ($dir)"
    echo "Файла лицензии в пакете нет; лицензия по package.json: $license"
    echo
  fi
}

tmp="$out.tmp.$$"
mkdir -p "$(dirname "$out")"
trap 'rm -f "$tmp"' EXIT

modules="$(go_modules "$@")"
[ -n "$modules" ] || die "go list found no third-party Go modules"

{
  echo "# Сторонние компоненты"
  echo
  echo "ProtoLab распространяется вместе со следующими компонентами. Ниже — их лицензии."
  echo "Файл генерируется скриптом \`scripts/notices.sh\` при сборке релиза."
  echo
  echo "## Go-модули, встроенные в бинари"
  echo
  echo "Объединение зависимостей для платформ: $*."
  echo
  while IFS=$'\t' read -r path version dir; do
    [ -n "$dir" ] && [ -d "$dir" ] || die "module $path $version is not in the module cache: run go mod download"
    echo "### $path $version"
    echo
    emit_licenses "$dir" || die "Go module $path $version has no license file ($dir)"
  done <<< "$modules"

  echo "## Go: стандартная библиотека и рантайм"
  echo
  echo "### Go $(go env GOVERSION)"
  echo
  emit_licenses "$(go env GOROOT)" || die "no LICENSE in GOROOT"

  echo "## Пакеты веб-интерфейса"
  echo
  # Production tree of frontend/ (parseable output = one directory per line, the project itself first),
  # plus the bundled build tools.
  dirs="$( (cd frontend && npm ls --omit=dev --all --parseable) | tail -n +2)"
  for tool in $bundled_tools; do
    [ -d "frontend/node_modules/$tool" ] || die "build tool $tool is not installed (npm ci installs devDependencies)"
    dirs="$dirs"$'\n'"$PWD/frontend/node_modules/$tool"
  done
  [ -n "$dirs" ] || die "npm ls found no production packages"
  while IFS= read -r dir; do
    emit_npm_package "$dir"
  done < <(sort -u <<< "$dirs")
} > "$tmp"

mv "$tmp" "$out"
trap - EXIT
echo "wrote $out ($(wc -c < "$out" | tr -d ' ') bytes: $(grep -c '^### ' "$out") components)"
