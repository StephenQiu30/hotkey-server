#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
redis_url=${HOTKEY_TEST_REDIS_URL:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required for backend acceptance' >&2
  exit 1
fi
if test -z "$redis_url"; then
  printf '%s\n' 'HOTKEY_TEST_REDIS_URL is required for backend acceptance' >&2
  exit 1
fi

case "$dsn" in
  postgres://*|postgresql://*) ;;
  *)
    printf '%s\n' 'HOTKEY_TEST_DSN must use a PostgreSQL URL' >&2
    exit 1
    ;;
esac
postgres_base=${dsn%%\?*}
postgres_database=${postgres_base##*/}
case "$postgres_database" in
  ''|*[!A-Za-z0-9_]*)
    printf '%s\n' 'HOTKEY_TEST_DSN must name a safe disposable PostgreSQL database ending in _test' >&2
    exit 1
    ;;
  *_test) ;;
  *)
    printf '%s\n' 'HOTKEY_TEST_DSN must target a disposable PostgreSQL database ending in _test' >&2
    exit 1
    ;;
esac

case "$redis_url" in
  redis://*|rediss://*) ;;
  *)
    printf '%s\n' 'HOTKEY_TEST_REDIS_URL must use a Redis URL' >&2
    exit 1
    ;;
esac
redis_base=${redis_url%%\?*}
redis_database=${redis_base##*/}
case "$redis_database" in
  1|2|3|4|5|6|7|8|9|10|11|12|13|14|15) ;;
  *)
    printf '%s\n' 'HOTKEY_TEST_REDIS_URL must target a disposable Redis database from 1 through 15' >&2
    exit 1
    ;;
esac
