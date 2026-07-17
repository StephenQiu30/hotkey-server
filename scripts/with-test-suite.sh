#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
suite_root="$root/tests/_suite"
go_command=${GO:-go}

if test ! -d "$suite_root"; then
  printf '%s\n' "missing centralized test suite: $suite_root" >&2
  exit 1
fi

if test "$#" -eq 0; then
  set -- test ./... -count=1
fi

source_list=$(mktemp "${TMPDIR:-/tmp}/hotkey-test-sources.XXXXXX")
link_list=$(mktemp "${TMPDIR:-/tmp}/hotkey-test-links.XXXXXX")

cleanup() {
  status=$1
  if test -f "$link_list"; then
    while IFS= read -r target; do
      if test -L "$target"; then
        rm -f "$target"
      fi
    done < "$link_list"
  fi
  rm -f "$source_list" "$link_list"
  exit "$status"
}
trap 'cleanup $?' 0 HUP INT TERM

find "$suite_root" -type f -name '*_test.go' -print | LC_ALL=C sort > "$source_list"

while IFS= read -r source; do
  relative=${source#"$suite_root"/}
  target="$root/$relative"
  if test -e "$target" || test -L "$target"; then
    printf '%s\n' "test materialization conflict: $relative" >&2
    exit 1
  fi
done < "$source_list"

while IFS= read -r source; do
  relative=${source#"$suite_root"/}
  target="$root/$relative"
  mkdir -p "$(dirname "$target")"
  ln -s "$source" "$target"
  printf '%s\n' "$target" >> "$link_list"
done < "$source_list"

HOTKEY_TEST_SUITE_ACTIVE=1 "$go_command" "$@"
