#!/bin/sh
# doctor.sh — check (and optionally install) the ikigenba tool suite.
#
#   sh doctor.sh                    diagnose: which tools are present, versions, what's missing
#   sh doctor.sh --install          install everything missing (asks per tool)
#   sh doctor.sh --install --yes    install everything missing without asking
#   sh doctor.sh --install embed    install a specific tool (repeatable)
#
# Diagnosis is read-only. Installing runs each tool's own installer:
#   curl -fsSL https://raw.githubusercontent.com/ikigenba/<tool>/main/install.sh | sh
# which drops a prebuilt release binary into ${PREFIX:-$HOME/.local}/bin.
set -eu

# tool  repo               version-flag
TOOLS="\
embed:ikigenba/embed:-V
autotune:ikigenba/autotune:-V
oauth:ikigenba/oauth:-V
idgen:ikigenba/idgen:-V
agentrepl:ikigenba/agentrepl:-V
ralph:ikigenba/ralph:-V"

DO_INSTALL=0
ASSUME_YES=0
ONLY=""
for a in "$@"; do
  case "$a" in
    --install) DO_INSTALL=1 ;;
    --yes|-y)  ASSUME_YES=1 ;;
    -h|--help)
      sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    -*) echo "doctor: unknown option: $a" >&2; exit 2 ;;
    *)  ONLY="$ONLY $a" ;;
  esac
done

wanted() {  # wanted <tool> -> 0 if in ONLY (or ONLY empty = all)
  [ -z "$ONLY" ] && return 0
  for t in $ONLY; do [ "$t" = "$1" ] && return 0; done
  return 1
}

install_one() {  # install_one <tool> <repo>
  echo "doctor: installing $1 from $2" >&2
  curl -fsSL "https://raw.githubusercontent.com/$2/main/install.sh" | sh
}

missing_file=$(mktemp)
trap 'rm -f "$missing_file"' EXIT

printf '%-11s %-9s %s\n' TOOL STATUS VERSION
printf '%-11s %-9s %s\n' ---- ------ -------
echo "$TOOLS" | while IFS=: read -r name repo vflag; do
  [ -n "$name" ] || continue
  wanted "$name" || continue
  if command -v "$name" >/dev/null 2>&1; then
    ver=$("$name" "$vflag" 2>/dev/null | head -1 | tr -d '\r' || true)
    printf '%-11s %-9s %s\n' "$name" "ok" "${ver:-?}"
  else
    printf '%-11s %-9s %s\n' "$name" "MISSING" "-"
    printf '%s:%s\n' "$name" "$repo" >>"$missing_file"
  fi
done

[ -s "$missing_file" ] || { echo; echo "doctor: all present."; exit 0; }

echo
n=$(wc -l <"$missing_file" | tr -d ' ')
echo "doctor: $n missing."

if [ "$DO_INSTALL" -ne 1 ]; then
  echo "doctor: re-run with --install to install the missing tools."
  exit 0
fi

# Installing: require an explicit go-ahead when not attached to a terminal.
if [ "$ASSUME_YES" -ne 1 ] && [ ! -t 0 ]; then
  echo "doctor: refusing to install non-interactively without --yes." >&2
  exit 3
fi

while IFS=: read -r name repo; do
  if [ "$ASSUME_YES" -ne 1 ]; then
    printf 'doctor: install %s (%s)? [y/N] ' "$name" "$repo"
    read -r ans </dev/tty || ans=n
    case "$ans" in y|Y|yes|YES) ;; *) echo "doctor: skipped $name"; continue ;; esac
  fi
  install_one "$name" "$repo"
done <"$missing_file"

echo
echo "doctor: re-run 'sh doctor.sh' to confirm."
