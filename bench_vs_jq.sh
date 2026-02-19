#!/usr/bin/env bash
set -euo pipefail

# Benchmark fastjq vs jq CLI on JSONL throughput
# Usage: ./bench_vs_jq.sh

ITERATIONS=3
TMPDIR_PREFIX="/tmp/fastjq_bench"
SMALL_FILE="${TMPDIR_PREFIX}_small.jsonl"
LARGE_FILE="${TMPDIR_PREFIX}_large.jsonl"
FASTJQ_BIN="/tmp/fastjq-bench"
DATAGEN_BIN="/tmp/fastjq-datagen"
DATAGEN_SRC="/tmp/fastjq-datagen.go"

# --- Check prerequisites ---
if ! command -v jq &>/dev/null; then
    echo "ERROR: jq not found in PATH" >&2
    exit 1
fi
if ! command -v go &>/dev/null; then
    echo "ERROR: go not found in PATH" >&2
    exit 1
fi
if ! perl -MTime::HiRes -e '1' 2>/dev/null; then
    echo "ERROR: perl Time::HiRes not available" >&2
    exit 1
fi

echo "jq version: $(jq --version)"
echo ""

# --- Generate test data ---
echo "Generating test data..."

cat > "$DATAGEN_SRC" << 'GOEOF'
package main

import (
	"fmt"
	"os"
	"strings"
)

func generateJSON(n int, valueSize int) string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"field_%d":"%s"`, i, strings.Repeat("x", valueSize))
	}
	b.WriteString("}")
	return b.String()
}

func generateNestedJSON(topKeys, nestedKeys, valueSize int) string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < topKeys; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		if i%3 == 0 && nestedKeys > 0 {
			fmt.Fprintf(&b, `"field_%d":{`, i)
			for j := 0; j < nestedKeys; j++ {
				if j > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `"sub_%d":"%s"`, j, strings.Repeat("y", valueSize))
			}
			b.WriteString("}")
		} else {
			fmt.Fprintf(&b, `"field_%d":"%s"`, i, strings.Repeat("x", valueSize))
		}
	}
	b.WriteString("}")
	return b.String()
}

func main() {
	kind := os.Args[1]
	outFile := os.Args[2]

	f, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	switch kind {
	case "small":
		line := generateJSON(5, 10)
		for i := 0; i < 100000; i++ {
			fmt.Fprintln(f, line)
		}
	case "large":
		line := generateNestedJSON(200, 10, 200)
		for i := 0; i < 100; i++ {
			fmt.Fprintln(f, line)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown kind: %s\n", kind)
		os.Exit(1)
	}
}
GOEOF

go build -o "$DATAGEN_BIN" "$DATAGEN_SRC"
"$DATAGEN_BIN" small "$SMALL_FILE"
"$DATAGEN_BIN" large "$LARGE_FILE"

small_size=$(wc -c < "$SMALL_FILE" | tr -d ' ')
large_size=$(wc -c < "$LARGE_FILE" | tr -d ' ')
small_lines=$(wc -l < "$SMALL_FILE" | tr -d ' ')
large_lines=$(wc -l < "$LARGE_FILE" | tr -d ' ')
echo "  small: ${small_lines} lines, $((small_size / 1024))KB"
echo "  large: ${large_lines} lines, $((large_size / 1024))KB"
echo ""

# --- Build fastjq CLI ---
echo "Building fastjq-bench..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
go build -o "$FASTJQ_BIN" "${SCRIPT_DIR}/cmd/fastjq-bench"
echo ""

# --- Timing helper ---
# Returns elapsed seconds with millisecond precision
time_cmd() {
    perl -MTime::HiRes=gettimeofday,tv_interval -e '
        my $t0 = [gettimeofday];
        system(@ARGV);
        my $elapsed = tv_interval($t0);
        printf("%.3f\n", $elapsed);
    ' -- "$@"
}

# Run N iterations, return median
median_time() {
    local -a times=()
    for ((i=0; i<ITERATIONS; i++)); do
        t=$(time_cmd "$@")
        times+=("$t")
    done
    # Sort and take median
    printf '%s\n' "${times[@]}" | sort -n | sed -n "$((( ITERATIONS + 1 ) / 2))p"
}

# --- Run benchmarks ---
declare -a BENCH_NAMES=()
declare -a BENCH_QUERIES=()
declare -a BENCH_FILES=()
declare -a BENCH_INPUTS=()

add_bench() {
    BENCH_NAMES+=("$1")
    BENCH_QUERIES+=("$2")
    BENCH_FILES+=("$3")
    BENCH_INPUTS+=("$4")
}

add_bench "identity"                    "."                                           "$SMALL_FILE" "small"
add_bench "field access"                ".field_2"                                    "$SMALL_FILE" "small"
add_bench "field access (large)"        ".field_50"                                   "$LARGE_FILE" "large"
add_bench "delete field"                "del(.field_2)"                               "$SMALL_FILE" "small"
add_bench "object construction"         "{field_0, field_2}"                          "$SMALL_FILE" "small"
add_bench "select (all match)"          'select(.field_2 == "xxxxxxxxxx")'            "$SMALL_FILE" "small"
add_bench "select (none match)"         'select(.field_2 == "nope")'                  "$SMALL_FILE" "small"
add_bench "alternative"                 '.field_2 // "default"'                       "$SMALL_FILE" "small"
add_bench "case-insensitive select"     'select(.field_2 | ascii_downcase == "xxxxxxxxxx")' "$SMALL_FILE" "small"
add_bench "prefix filter"               'select(.field_2 | startswith("xxxx"))'       "$SMALL_FILE" "small"
add_bench "field existence"             'select(has("field_2"))'                      "$SMALL_FILE" "small"
add_bench "to_entries"                  "to_entries"                                  "$SMALL_FILE" "small"
add_bench "keys"                        "keys_unsorted"                               "$SMALL_FILE" "small"

# Print header
printf "%-24s %-6s %10s %10s %8s\n" "Operation" "Input" "jq (s)" "fastjq (s)" "Speedup"
printf "%-24s %-6s %10s %10s %8s\n" "------------------------" "------" "----------" "----------" "--------"

for ((i=0; i<${#BENCH_NAMES[@]}; i++)); do
    name="${BENCH_NAMES[$i]}"
    query="${BENCH_QUERIES[$i]}"
    file="${BENCH_FILES[$i]}"
    input="${BENCH_INPUTS[$i]}"

    echo "  Running: ${name}..." >&2

    jq_time=$(median_time bash -c "jq -c '${query}' < '${file}' > /dev/null")
    fastjq_time=$(median_time bash -c "'${FASTJQ_BIN}' '${query}' < '${file}' > /dev/null")

    # Calculate speedup
    speedup=$(perl -e "printf('%.1fx', $jq_time / $fastjq_time)" 2>/dev/null || echo "N/A")

    printf "%-24s %-6s %10.3f %10.3f %8s\n" "$name" "$input" "$jq_time" "$fastjq_time" "$speedup"
done

echo ""

# --- Cleanup ---
rm -f "$SMALL_FILE" "$LARGE_FILE" "$FASTJQ_BIN" "$DATAGEN_BIN" "$DATAGEN_SRC"
echo "Done. Temp files cleaned up."
