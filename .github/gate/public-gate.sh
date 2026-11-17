#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# The public gate of the KaTyoti CI: a public subset of the release gate, re-run on every
# push. The release gate runs before each export with lists that stay private; this one
# reads only the public lists beside it, which the export writes:
#   hosts.txt     the public hosts a URL may name (each with its sub-domains)
#   waivers.txt   "<rooted path glob> <kind>": a reviewed host or private-address finding
#   binaries.txt  "<path> <sha256>": the reviewed non-text files
#   guids.txt     the GUIDs the published release carries
#   gitleaks.toml the secret scan's configuration (the CI runs gitleaks with it)
# A finding: a URL naming a host not listed or a private address, a GUID not listed, a
# non-text file not listed or changed since reviewed, a document or an archive. Any
# finding fails the gate. A missing list fails it too.
#
# Usage: bash .github/gate/public-gate.sh [<tree>]   (default: the current folder)
# Exit: 0 no finding, 1 a finding, 2 an error.
set -euo pipefail
export LC_ALL=C

tree=${1:-.}
cd "$tree" || { echo "public gate: no folder $tree" >&2; exit 2; }
lists=.github/gate
for f in hosts.txt waivers.txt binaries.txt guids.txt; do
  [ -f "$lists/$f" ] || { echo "public gate: $lists/$f is missing" >&2; exit 2; }
done

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
findings=$work/findings

# The files of the tree, .git left out.
find . -path ./.git -prune -o -type f -print | sed 's|^\./||' | sort > "$work/files"
: > "$findings"

# Documents and archives: never published.
grep -iE '\.(pdf|docx?|xlsx?|xlsm|pptx?|odt|ods|odp|msg|eml|zip|7z|rar|tar|gz|tgz|bz2|xz|jar|war)$' "$work/files" \
  | sed 's|^|binary |; s|$|: a document or an archive|' >> "$findings" || true

# Non-text files (git's test: a NUL byte in the first 8000 bytes): listed, same sha256.
tr '\n' '\0' < "$work/files" | xargs -0 -r grep -L -I -e '' -- > "$work/candidates" || true
: > "$work/binaries"
while IFS= read -r f; do
  [ -s "$f" ] || continue
  if [ "$(head -c 8000 -- "$f" | tr -dc '\000' | wc -c)" -gt 0 ]; then
    printf '%s %s\n' "$f" "$(sha256sum -- "$f" | cut -d' ' -f1)" >> "$work/binaries"
  fi
done < "$work/candidates"
awk 'NR == FNR { if ($0 !~ /^[[:space:]]*(#|$)/) want[$1] = $2; next }
  !($1 in want) { print "binary " $1 ": not on binaries.txt"; next }
  want[$1] != $2 { print "binary " $1 ": changed since reviewed (sha256 differs from binaries.txt)" }' \
  "$lists/binaries.txt" "$work/binaries" >> "$findings"

# The text files: every file not found non-text.
cut -d' ' -f1 "$work/binaries" | sort > "$work/binary-files"
comm -23 "$work/files" "$work/binary-files" | tr '\n' '\0' > "$work/text"

# Hosts: the host of every URL.
xargs -0 -r grep -oiHE '\b(https?|wss?|ftp|nats)://[^][/[:space:]"'"'"'<>(){}\\`,;|]+' -- < "$work/text" > "$work/urls" || true
awk -v hosts="$lists/hosts.txt" -v waivers="$lists/waivers.txt" '
  function under(h,   d) {
    for (;;) { if (h in allow) return 1; d = index(h, "."); if (!d) return 0; h = substr(h, d + 1) }
  }
  function globre(g,   r, i, c) {
    r = "^"
    for (i = 1; i <= length(g); i++) {
      c = substr(g, i, 1)
      if (c == "*") r = r "[^/]*"; else if (c == "?") r = r "[^/]"
      else if (index(".+()|{}^$\\", c)) r = r "\\" c; else r = r c
    }
    return r "$"
  }
  function waived(file, kind,   i) {
    for (i = 1; i <= nw; i++) if (wkind[i] == kind && file ~ wre[i]) return 1
    return 0
  }
  function private4(h,   o) {
    split(h, o, ".")
    return o[1] == 10 || (o[1] == 172 && o[2] >= 16 && o[2] <= 31) || (o[1] == 192 && o[2] == 168) || (o[1] == 169 && o[2] == 254)
  }
  BEGIN {
    while ((getline l < hosts) > 0) { sub(/#.*/, "", l); if (split(l, f, " ") > 0) allow[tolower(f[1])] = 1 }
    n = split("example.com example.org example.net example test invalid localhost", r, " ")
    for (i = 1; i <= n; i++) allow[r[i]] = 1
    while ((getline l < waivers) > 0) {
      if (l ~ /^[[:space:]]*(#|$)/ || split(l, f, " ") < 2) continue
      g = f[1]; sub(/^\//, "", g); nw++; wre[nw] = globre(g); wkind[nw] = f[2]
    }
  }
  {
    low = tolower($0)
    if (!match(low, /:(https?|wss?|ftp|nats):\/\//)) next
    file = substr($0, 1, RSTART - 1); a = substr(low, RSTART + RLENGTH)
    sub(/.*@/, "", a)
    if (a ~ /^\[/) {                       # an IPv6 literal
      if (!sub(/\].*/, "", a)) next
      a = substr(a, 2)
      if (a == "::1" || a == "::") next
      kind = (a ~ /^f[cd]/ || a ~ /^fe[89ab]/) ? "private-address" : "host"
    } else {
      sub(/:.*/, "", a); sub(/\.+$/, "", a)
      if (a !~ /^[a-z0-9.-]+$/ || a !~ /[a-z0-9]/) next   # a template, a variable
      if (a ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) {
        if (a ~ /^127\./ || a == "0.0.0.0") next
        kind = private4(a) ? "private-address" : "host"
      } else {
        if (under(a)) next
        kind = "host"
      }
    }
    if (!waived(file, kind)) print kind " " file ": " a
  }' "$work/urls" | sort -u >> "$findings"

# GUIDs: every GUID of a text file or a path is on guids.txt.
{
  xargs -0 -r grep -oiHE '\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b' -- < "$work/text" || true
  grep -oiE '\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b' "$work/files" | sed 's|^|(a path):|' || true
} | awk -v guids="$lists/guids.txt" '
  BEGIN { while ((getline l < guids) > 0) { sub(/#.*/, "", l); if (split(l, f, " ") > 0) known[tolower(f[1])] = 1 } }
  { g = tolower(substr($0, length($0) - 35)); file = substr($0, 1, length($0) - 37)
    if (!(g in known)) print "guid " file ": " g }' | sort -u >> "$findings"

if [ -s "$findings" ]; then
  cat "$findings"
  echo "public gate: $(wc -l < "$findings") finding(s)" >&2
  exit 1
fi
echo "public gate: no finding ($(wc -l < "$work/files") files)"
