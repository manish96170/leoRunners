#!/bin/sh
set -eu

test_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if ! "$test_dir/validate.sh"; then
    printf 'github-validation tests: FAIL\n' >&2
    exit 1
fi

for fixture in "$test_dir/fixtures/scope.safe" "$test_dir/fixtures/scope.fork" "$test_dir/fixtures/scope.repository" "$test_dir/fixtures/scope.label"; do
    case "$(sed -n 's/^expected=//p' "$fixture")" in
        PASS|DENY) ;;
        *) printf 'github-validation tests: invalid fixture %s\n' "$fixture" >&2; exit 1 ;;
    esac
done

printf 'github-validation tests: PASS\n'
