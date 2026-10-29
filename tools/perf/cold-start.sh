#!/bin/sh
# Cold-start measurement for the Lambda path.
#
# Each `sam local invoke` creates a fresh container, so every invocation is
# a cold start by construction. For each invoke the script captures:
#   - Init Duration / Duration from the Lambda-style REPORT line
#   - wall time of the whole invoke (container create + runtime + request)
#
# Usage: cold-start.sh [invocations] [event-file]
# Env:   SAM=sam  FUNCTION=InferFunction
#
# Requires: sam CLI, Docker, a built image (`make sam-build` first).
# POSIX sh + grep/sed/awk/sort — runs under Git Bash on Windows.
set -u

N="${1:-10}"
EVENT="${2:-tools/perf/event-medium.json}"
SAM="${SAM:-sam}"
FUNCTION="${FUNCTION:-InferFunction}"

inits=""
durs=""
walls=""
failures=0

i=0
while [ "$i" -lt "$N" ]; do
	i=$((i + 1))
	start_ms=$(date +%s%3N)
	out="$("$SAM" local invoke "$FUNCTION" -e "$EVENT" 2>&1)"
	wall_ms=$(( $(date +%s%3N) - start_ms ))

	report="$(printf '%s' "$out" | grep 'REPORT RequestId' || true)"
	init_ms="$(printf '%s' "$report" | tr '\t' '\n' | sed -n 's/^Init Duration: \([0-9.]*\) ms/\1/p')"
	dur_ms="$(printf '%s' "$report" | tr '\t' '\n' | sed -n 's/^Duration: \([0-9.]*\) ms/\1/p')"

	if [ -z "$init_ms" ] || [ -z "$dur_ms" ]; then
		failures=$((failures + 1))
		printf 'invoke %d: no REPORT line (wall %d ms)\n' "$i" "$wall_ms" >&2
		continue
	fi

	inits="$inits $init_ms"
	durs="$durs $dur_ms"
	walls="$walls $wall_ms"
	printf 'invoke %d: init %s ms  duration %s ms  wall %d ms\n' "$i" "$init_ms" "$dur_ms" "$wall_ms"
done

stats() {
	printf '%s\n' "$@" | tr ' ' '\n' | grep -v '^$' | sort -n | awk '
		{ a[NR] = $1; sum += $1 }
		END {
			if (NR == 0) { printf "no samples"; exit }
			printf "min %.2f  median %.2f  mean %.2f  max %.2f  n %d", \
				a[1], a[int((NR + 1) / 2)], sum / NR, a[NR], NR
		}'
}

echo "---"
echo "init_duration_ms:      $(stats $inits)"
echo "duration_ms:           $(stats $durs)"
echo "invoke_wall_ms:        $(stats $walls)"
echo "failed_invocations:    $failures"
