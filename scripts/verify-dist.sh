#!/usr/bin/env bash
# Checks the output of `make release-local` before it is published (CI runs the same script):
#
#   * exactly one archive per target, SHA256SUMS matches;
#   * every archive holds the binary, README.md and THIRD_PARTY_NOTICES.md;
#   * every binary is built for its own GOOS/GOARCH with CGO_ENABLED=0 -trimpath, and every Go module
#     linked into it (`go version -m`) has its license text in the notices of its archive;
#   * the notices carry the OFL license when the embedded web UI ships fonts;
#   * the binary that runs on this machine reports the release version and serves the embedded web UI.
#
# Usage: scripts/verify-dist.sh DIST VERSION GOOS/GOARCH [GOOS/GOARCH ...]
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"

die() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then echo "::error::verify-dist: $*"; else echo "ERROR: verify-dist: $*" >&2; fi
  exit 1
}

[ $# -ge 3 ] || die "usage: scripts/verify-dist.sh DIST VERSION GOOS/GOARCH [GOOS/GOARCH ...]"
dist="$(cd "$1" && pwd)"
version="$2"
shift 2
targets=("$@")

cd "$dist"
work="$(mktemp -d "$dist/verify.XXXXXX")" # inside dist: /tmp may be mounted noexec
pid=""
cleanup() {
  if [ -n "$pid" ]; then kill "$pid" 2> /dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

# --- archives and checksums ---------------------------------------------------------------------------
shopt -s nullglob
archives=(./*.tar.gz ./*.zip)
[ "${#archives[@]}" -eq "${#targets[@]}" ] \
  || die "expected ${#targets[@]} archives, found ${#archives[@]}: ${archives[*]:-none}"
[ "$(wc -l < SHA256SUMS | tr -d ' ')" -eq "${#targets[@]}" ] || die "SHA256SUMS must list exactly ${#targets[@]} archives"
if command -v sha256sum > /dev/null 2>&1; then sha256sum -c SHA256SUMS; else shasum -a 256 -c SHA256SUMS; fi

# --- fonts in the embedded web UI need the OFL text in the notices ------------------------------------------
fonts=("$repo"/internal/webui/dist/assets/*.woff2)
ships_fonts=0
[ "${#fonts[@]}" -eq 0 ] || ships_fonts=1

# --- every archive -------------------------------------------------------------------------------------
host="$(go env GOOS)/$(go env GOARCH)"
host_bin=""
for t in "${targets[@]}"; do
  os="${t%/*}"
  arch="${t#*/}"
  name="protolab_${version}_${os}_${arch}"
  exe="protolab"
  archive="$name.tar.gz"
  if [ "$os" = windows ]; then exe="protolab.exe"; archive="$name.zip"; fi
  [ -f "$archive" ] || die "missing $archive"

  out="$work/$os-$arch"
  mkdir -p "$out"
  if [ "$os" = windows ]; then unzip -q "$archive" -d "$out"; else tar -xzf "$archive" -C "$out"; fi
  root="$out/$name"
  for f in "$exe" README.md THIRD_PARTY_NOTICES.md; do
    [ -s "$root/$f" ] || die "$archive: $name/$f is missing or empty"
  done
  [ "$os" = windows ] || [ -x "$root/$exe" ] || die "$archive: $name/$exe is not executable"

  info="$(go version -m "$root/$exe")"
  for want in "GOOS=$os" "GOARCH=$arch" "CGO_ENABLED=0" "-trimpath=true"; do
    grep -qxF "$(printf '\tbuild\t%s' "$want")" <<< "$info" || die "$archive: the binary was not built with $want"
  done

  notices="$root/THIRD_PARTY_NOTICES.md"
  deps=0
  while IFS= read -r dep; do
    deps=$((deps + 1))
    grep -qxF "### $dep" "$notices" || die "$archive: no license text for the linked Go module $dep"
  done < <(awk -F'\t' '$2 == "dep" { print $3 " " $4 }' <<< "$info")
  [ "$deps" -gt 0 ] || die "$archive: go version -m lists no Go modules"
  if [ "$ships_fonts" -eq 1 ]; then
    grep -qi 'SIL OPEN FONT LICENSE' "$notices" || die "$archive: the web UI ships fonts but the notices have no OFL text"
  fi
  echo "ok  $t: $deps Go modules covered by the notices"

  if [ "$t" = "$host" ]; then host_bin="$root/$exe"; fi
done

# --- run the binary of this machine ---------------------------------------------------------------------
if [ -z "$host_bin" ]; then
  echo "note: $host is not a release target, the binary was not started"
else
  out="$("$host_bin" version)"
  echo "$out"
  grep -qF "$version" <<< "$out" || die "the binary does not report version $version"

  port=""
  for p in $(seq 17421 17520); do
    if ! (exec 3<> "/dev/tcp/127.0.0.1/$p") 2> /dev/null; then port="$p"; break; fi
  done
  [ -n "$port" ] || die "no free TCP port in 17421-17520"
  mkdir -p "$work/data"
  "$host_bin" --no-tui --data "$work/data" --addr "127.0.0.1:$port" > "$work/server.log" 2>&1 &
  pid=$!
  for _ in $(seq 1 100); do
    curl -fsS "http://127.0.0.1:$port/api/v1/system/info" > /dev/null 2>&1 && break
    sleep 0.2
  done
  page="$(curl -fsS "http://127.0.0.1:$port/" || { cat "$work/server.log"; die "the server does not answer on port $port"; })"
  grep -qF '<div id="root">' <<< "$page" || die "the web UI is not embedded into the binary"
  echo "ok  $host: version $version, the embedded web UI is served"
fi

ls -l ./*.tar.gz ./*.zip
echo "dist OK: ${#targets[@]} archives, checksums, notices, build flags"
