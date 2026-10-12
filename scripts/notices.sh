#!/usr/bin/env bash
# Regenerates THIRD_PARTY_NOTICES.md: licenses of the Go modules linked into the binary and of the
# frontend packages bundled into the embedded web UI (including the OFL fonts).
#
#   scripts/notices.sh            # needs a built bin/protolab and frontend/node_modules
set -euo pipefail
cd "$(dirname "$0")/.."

bin=bin/protolab
[ -x "$bin" ] || { echo "build bin/protolab first (make build)" >&2; exit 1; }
[ -d frontend/node_modules ] || { echo "run npm ci in frontend/ first" >&2; exit 1; }
modcache="$(go env GOMODCACHE)"
out=THIRD_PARTY_NOTICES.md

license_file() {
  for f in LICENSE LICENSE.md LICENSE.txt LICENCE COPYING OFL.txt license license.md; do
    [ -f "$1/$f" ] && { echo "$1/$f"; return; }
  done
}

# Go module paths in the module cache escape upper-case letters as !lower.
escape() { sed -E 's/([A-Z])/!\L\1/g' <<< "$1"; }

{
  echo "# Сторонние компоненты"
  echo
  echo "ProtoLab распространяется вместе со следующими компонентами. Ниже — их лицензии."
  echo "Файл генерируется скриптом \`scripts/notices.sh\`."
  echo
  echo "## Go-модули, встроенные в бинарь"
  echo
  go version -m "$bin" | awk '$1 == "dep" { print $2, $3 }' | while read -r path ver; do
    dir="$modcache/$(escape "$path")@$ver"
    lic="$(license_file "$dir" || true)"
    echo "### $path $ver"
    echo
    if [ -n "$lic" ]; then
      echo '```text'
      cat "$lic"
      echo '```'
    else
      echo "Лицензия: см. https://pkg.go.dev/$path?tab=licenses"
    fi
    echo
  done

  echo "## Пакеты веб-интерфейса"
  echo
  (cd frontend && npm ls --omit=dev --all --parseable 2>/dev/null) | tail -n +2 | sort -u | while read -r dir; do
    name="$(node -p "require('$dir/package.json').name")"
    ver="$(node -p "require('$dir/package.json').version")"
    lic="$(license_file "$dir" || true)"
    echo "### $name $ver"
    echo
    if [ -n "$lic" ]; then
      echo '```text'
      cat "$lic"
      echo '```'
    else
      echo "Лицензия: $(node -p "require('$dir/package.json').license || 'см. пакет'")"
    fi
    echo
  done
} > "$out"

echo "wrote $out ($(wc -c < "$out") bytes)"
