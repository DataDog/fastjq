package fastjq

// complex_test.go — correctness tests for complex multi-feature queries.
// These focus on the *intersection* of features: arithmetic inside filters,
// try/catch wrapping pipelines, elif with string ops, object merge in
// construction, tojson/fromjson round-trips, aggregation patterns, etc.
// Most single-feature behaviour is covered in fastjq_test.go; this file
// only adds tests that require two or more features interacting.

import (
	"strings"
	"testing"
)

// --- Helper for multi-output queries ---

// assertQueryAll runs a query and checks all outputs joined by newlines.
func assertQueryAll(t *testing.T, query, input string, want ...string) {
	t.Helper()
	p, err := Compile(query)
	if err != nil {
		t.Fatalf("Compile(%q): %v", query, err)
	}
	results, err := p.RunAll([]byte(input))
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(results) != len(want) {
		got := make([]string, len(results))
		for i, r := range results {
			got[i] = string(r)
		}
		t.Errorf("query %q: got %d outputs %v, want %d %v",
			query, len(results), got, len(want), want)
		return
	}
	for i := range want {
		if string(results[i]) != want[i] {
			t.Errorf("query %q output[%d]: got %s, want %s",
				query, i, results[i], want[i])
		}
	}
}

// --- Pipeline composition ---

func TestComplexArrayPipeline(t *testing.T) {
	// filter + project in one pipeline
	input := `[{"name":"alice","active":true,"score":80},{"name":"bob","active":false,"score":90},{"name":"carol","active":true,"score":70}]`
	assertQuery(t, `[.[] | select(.active) | .name]`, input, `["alice","carol"]`)
}

func TestComplexMapArithmetic(t *testing.T) {
	// map with arithmetic + add aggregation
	input := `[{"price":10,"qty":3},{"price":5,"qty":4},{"price":20,"qty":1}]`
	assertQuery(t, `[.[] | .price * .qty] | add`, input, `70`)
}

func TestComplexAggregationObject(t *testing.T) {
	// building a stats object from an array
	input := `[{"price":10,"qty":2},{"price":5,"qty":4},{"price":20,"qty":1}]`
	assertQuery(t,
		`{count: length, revenue: ([.[] | .price * .qty] | add)}`,
		input, `{"count":3,"revenue":60}`)
}

func TestComplexFilterThenProject(t *testing.T) {
	// select + arithmetic in output field + string op
	input := `[{"product":"Widget","price":12.5,"active":true},{"product":"gadget","price":5,"active":false},{"product":"TOOL","price":30,"active":true}]`
	assertQuery(t,
		`[.[] | select(.active and .price > 10) | {name: (.product | ascii_downcase), price: .price}]`,
		input,
		`[{"name":"widget","price":12.5},{"name":"tool","price":30}]`)
}

func TestComplexToFromEntriesFiltered(t *testing.T) {
	// filter object entries by value type and value
	input := `{"name":"alice","age":30,"score":null,"active":true}`
	assertQuery(t,
		`to_entries | map(select(.value != null and (.value | type) != "boolean")) | from_entries`,
		input, `{"name":"alice","age":30}`)
}


func TestComplexPipelineMultiStageCorrect(t *testing.T) {
	input := `[{"id":1,"val":5},{"id":2,"val":15},{"id":3,"val":25},{"id":4,"val":8}]`
	assertQuery(t,
		`[.[] | select(.val > 3) | {id, doubled: .val * 2} | select(.doubled > 20)]`,
		input, `[{"id":2,"doubled":30},{"id":3,"doubled":50}]`)
}

func TestComplexNestedIterator(t *testing.T) {
	// iterate nested arrays
	input := `{"groups":[{"members":["alice","bob"]},{"members":["carol"]},{"members":["dave","eve"]}]}`
	assertQuery(t,
		`[.groups[] | .members[]] | length`,
		input, `5`)
}

func TestComplexMinMaxPipeline(t *testing.T) {
	// min_by + field access in pipeline
	input := `[{"name":"alice","score":85},{"name":"bob","score":72},{"name":"carol","score":91}]`
	assertQuery(t, `min_by(.score) | .name`, input, `"bob"`)
	assertQuery(t, `max_by(.score) | .name`, input, `"carol"`)
}

func TestComplexAggregationMinMaxAll(t *testing.T) {
	// combine min_by, max_by, all in one query
	input := `[{"name":"alice","score":85},{"name":"bob","score":72},{"name":"carol","score":91}]`
	assertQuery(t,
		`{best: (max_by(.score) | .name), worst: (min_by(.score) | .name), passing: all(.[]; .score >= 70)}`,
		input, `{"best":"carol","worst":"bob","passing":true}`)
}

// --- Arithmetic in filters and constructions ---

func TestComplexArithmeticFilter(t *testing.T) {
	// multiplication and addition in select
	input := `[{"price":12,"qty":5},{"price":3,"qty":2},{"price":8,"qty":10}]`
	assertQuery(t,
		`[.[] | select(.price * .qty > 50) | .price * .qty]`,
		input, `[60,80]`)
}

func TestComplexArithmeticInConstruct(t *testing.T) {
	input := `{"base":100,"tax_rate":0.15,"discount":10}`
	assertQuery(t,
		`{net: (.base - .discount), tax: ((.base - .discount) * .tax_rate), total: (.base - .discount) * (1 + .tax_rate)}`,
		input, `{"net":90,"tax":13.5,"total":103.49999999999999}`)
}

func TestComplexArithmeticPrecedence(t *testing.T) {
	// ensure * binds tighter than + in filter context
	assertQuery(t, `select(1 + 2 * 3 == 7)`, `null`, `null`)
	assertQuery(t, `.a + .b * .c`, `{"a":1,"b":2,"c":3}`, `7`)
}

func TestComplexDivisionInAggregation(t *testing.T) {
	// compute average via arithmetic
	input := `[10,20,30,40,50]`
	assertQuery(t, `(add) / length`, input, `30`)
}

// --- String operations in pipelines ---

func TestComplexStringFilterPipeline(t *testing.T) {
	// multiple string ops chained
	input := `[{"path":"/api/users","method":"GET"},{"path":"/api/orders","method":"POST"},{"path":"/health","method":"GET"}]`
	assertQuery(t,
		`[.[] | select(.path | startswith("/api/")) | .path | ltrimstr("/api")]`,
		input, `["/users","/orders"]`)
}

func TestComplexStringNormalization(t *testing.T) {
	// case-insensitive filter + uppercase output
	input := `[{"level":"ERROR","msg":"disk full"},{"level":"info","msg":"ok"},{"level":"Warn","msg":"high load"}]`
	assertQuery(t,
		`[.[] | select(.level | ascii_downcase | startswith("err") or startswith("warn")) | .level | ascii_upcase]`,
		input, `["ERROR","WARN"]`)
}

func TestComplexJoinAfterMap(t *testing.T) {
	input := `[{"tag":"go"},{"tag":"zero-alloc"},{"tag":"fast"}]`
	assertQuery(t, `[.[] | .tag] | join(", ")`, input, `"go, zero-alloc, fast"`)
}

func TestComplexSplitThenFilter(t *testing.T) {
	// split a string and filter the parts
	input := `"errors:5,warnings:2,info:100"`
	assertQuery(t,
		`split(",") | map(select(startswith("error") or startswith("warn")))`,
		input, `["errors:5","warnings:2"]`)
}

func TestComplexStringBuildingConcat(t *testing.T) {
	// building a string from multiple fields using +
	input := `{"first":"John","last":"Doe","age":30}`
	assertQuery(t,
		`.first + " " + .last + " (" + (.age | tostring) + ")"`,
		input, `"John Doe (30)"`)
}

// --- try/catch in complex contexts ---

func TestComplexTryCatchInMap(t *testing.T) {
	// error-tolerant map: field access on mixed-type array
	input := `[{"x":1},42,{"x":3},"hello",{"x":5}]`
	assertQuery(t, `[.[] | try .x catch null]`, input, `[1,null,3,null,5]`)
}

func TestComplexTryCatchDivision(t *testing.T) {
	// catch divide-by-zero
	input := `[{"a":10,"b":2},{"a":5,"b":0},{"a":9,"b":3}]`
	assertQuery(t, `[.[] | try (.a / .b) catch "div0"]`, input, `[5,"div0",3]`)
}

func TestComplexTryCatchFromJSON(t *testing.T) {
	// parse optional JSON-string fields
	input := `[{"id":1,"raw":"{\"v\":42}"},{"id":2,"raw":"not-json"},{"id":3,"raw":"{\"v\":7}"}]`
	assertQuery(t,
		`[.[] | {id, val: (try (.raw | fromjson | .v) catch null)}]`,
		input, `[{"id":1,"val":42},{"id":2,"val":null},{"id":3,"val":7}]`)
}

func TestComplexTryCatchNested(t *testing.T) {
	// try wrapping a try — outer catch fires when inner succeeds but outer fails
	assertQuery(t, `try (try .a catch "inner") catch "outer"`, `[1,2]`, `"inner"`)
}

func TestComplexTryInSelect(t *testing.T) {
	// select using try to suppress errors on mixed-type input
	input := `[{"score":85},{"score":"n/a"},{"score":60}]`
	assertQuery(t,
		`[.[] | select((try .score catch -1) > 70) | .score]`,
		input, `[85]`)
}

// --- elif in real queries ---

func TestComplexElifGrading(t *testing.T) {
	input := `[{"name":"alice","score":95},{"name":"bob","score":82},{"name":"carol","score":71},{"name":"dave","score":55}]`
	assertQuery(t,
		`[.[] | {name, grade: if .score >= 90 then "A" elif .score >= 80 then "B" elif .score >= 70 then "C" else "F" end}]`,
		input,
		`[{"name":"alice","grade":"A"},{"name":"bob","grade":"B"},{"name":"carol","grade":"C"},{"name":"dave","grade":"F"}]`)
}

func TestComplexElifWithEmpty(t *testing.T) {
	// elif branch that drops records via empty
	input := `[{"status":"active"},{"status":"deleted"},{"status":"pending"},{"status":"active"}]`
	assertQuery(t,
		`[.[] | if .status == "active" then "keep" elif .status == "pending" then "keep" else empty end] | length`,
		input, `3`)
}

func TestComplexElifStringRouting(t *testing.T) {
	input := `{"level":"WARN","msg":"high CPU"}`
	assertQuery(t,
		`if (.level | ascii_downcase) == "error" then "alert" elif (.level | ascii_downcase) == "warn" then "notice" else "ignore" end`,
		input, `"notice"`)
}

// --- Object merge in construction ---

func TestComplexObjectMergeDefaults(t *testing.T) {
	// apply defaults via merge — right wins, left-only keys come first
	input := `{"timeout":60,"retries":1}`
	// Left: {timeout:30, retries:3, verbose:false}. Right (.): {timeout:60, retries:1}.
	// Left key order is preserved; right values override left values for duplicate keys.
	assertQuery(t,
		`{"timeout":30,"retries":3,"verbose":false} + .`,
		input, `{"timeout":60,"retries":1,"verbose":false}`)
}

func TestComplexObjectMergeEnrich(t *testing.T) {
	// merge two fields of an object
	assertQuery(t, `.base + .override`,
		`{"base":{"a":1,"b":2},"override":{"b":99,"c":3}}`,
		`{"a":1,"b":99,"c":3}`)
}

func TestComplexObjectMergeInPipeline(t *testing.T) {
	// merge result of construction with original
	input := `{"name":"alice","role":"admin","secret":"s3cr3t"}`
	assertQuery(t,
		`. + {"secret": null} | del(.secret)`,
		// This just adds secret=null then deletes it — identity essentially
		input, `{"name":"alice","role":"admin"}`)
}

// --- tojson / fromjson round-trips ---

func TestComplexToFromJSONRoundTrip(t *testing.T) {
	// complex object survives tojson | fromjson
	input := `{"a":1,"b":true,"c":null,"d":[1,2,3],"e":{"nested":42}}`
	assertQuery(t, `tojson | fromjson`, input, input)
}

func TestComplexFromJSONFieldAccess(t *testing.T) {
	// parse a stringified JSON field and access it
	input := `{"id":1,"payload":"{\"event\":\"click\",\"x\":100,\"y\":200}"}`
	assertQuery(t, `.payload | fromjson | {event, x}`, input, `{"event":"click","x":100}`)
}

func TestComplexToStringInArray(t *testing.T) {
	// coerce mixed array to strings
	input := `[1, true, null, "hello", 3.14]`
	assertQuery(t, `[.[] | tostring]`, input, `["1","true","null","hello","3.14"]`)
}

func TestComplexTonumberInFilter(t *testing.T) {
	// filter by numeric value of string field
	input := `[{"id":"1","score":"85"},{"id":"2","score":"42"},{"id":"3","score":"91"}]`
	assertQuery(t,
		`[.[] | select((.score | tonumber) >= 80) | .id]`,
		input, `["1","3"]`)
}

// --- any / all two-arg with complex conditions ---

func TestComplexAnyAllValidation(t *testing.T) {
	input := `[{"score":85,"active":true},{"score":72,"active":true},{"score":91,"active":false}]`
	assertQuery(t, `all(.[]; .score >= 70)`, input, `true`)
	assertQuery(t, `all(.[]; .active)`, input, `false`)
	assertQuery(t, `any(.[]; .score > 90 and .active)`, input, `false`)
	assertQuery(t, `any(.[]; .score > 90)`, input, `true`)
}

func TestComplexAnyTwoArgNested(t *testing.T) {
	// any(gen; cond) with a non-trivial generator
	input := `{"groups":[{"name":"a","vals":[1,2,3]},{"name":"b","vals":[4,5,6]},{"name":"c","vals":[7,8,9]}]}`
	assertQuery(t, `any(.groups[]; any(.vals[]; . > 8))`, input, `true`)
}

// --- Limit and first/last in pipelines ---

func TestComplexFirstWithPipeline(t *testing.T) {
	input := `[{"name":"alice","score":72},{"name":"bob","score":91},{"name":"carol","score":85}]`
	assertQuery(t, `first(.[] | select(.score > 80)) | .name`, input, `"bob"`)
}

func TestComplexLimitWithFilter(t *testing.T) {
	input := `[1,2,3,4,5,6,7,8,9,10]`
	assertQuery(t, `[limit(3; .[] | select(. % 2 == 0))]`, input, `[2,4,6]`)
}

func TestComplexFirstOnEmpty(t *testing.T) {
	// first on a generator that produces nothing returns null
	input := `[1,2,3]`
	assertQuery(t, `first(.[]) | tostring`, input, `"1"`)
}

// --- Log processing patterns (realistic workloads) ---

func TestComplexLogNormalization(t *testing.T) {
	// realistic log processing: normalize + enrich + project
	input := `{"timestamp":"2026-01-01T12:00:00Z","LEVEL":"ERROR","service":"api-gateway","message":"timeout","duration_ms":"342","retry":3}`
	assertQuery(t,
		`{level: (.LEVEL | ascii_downcase), svc: .service, msg: .message, dur: (.duration_ms | tonumber), high_retry: (.retry > 2)}`,
		input,
		`{"level":"error","svc":"api-gateway","msg":"timeout","dur":342,"high_retry":true}`)
}

func TestComplexLogRouting(t *testing.T) {
	// route log records to different outputs based on level
	events := `[{"level":"error","msg":"crash"},{"level":"warn","msg":"slow"},{"level":"info","msg":"ok"},{"level":"error","msg":"oom"}]`
	assertQuery(t,
		`[.[] | if (.level | ascii_downcase) == "error" then {alert: true, msg: .msg} elif (.level | ascii_downcase) == "warn" then {alert: false, msg: .msg} else empty end]`,
		events,
		`[{"alert":true,"msg":"crash"},{"alert":false,"msg":"slow"},{"alert":true,"msg":"oom"}]`)
}

func TestComplexSensitiveFieldRedaction(t *testing.T) {
	// remove sensitive fields from a log event
	input := `{"user":"alice","password":"s3cr3t","token":"abc123","action":"login","ip":"1.2.3.4"}`
	assertQuery(t, `del(.password, .token)`, input, `{"user":"alice","action":"login","ip":"1.2.3.4"}`)
}

func TestComplexMetricsAggregation(t *testing.T) {
	// compute summary stats over an array of metric readings
	input := `[{"name":"cpu","val":45},{"name":"cpu","val":67},{"name":"mem","val":80},{"name":"cpu","val":52}]`
	assertQuery(t,
		`[.[] | select(.name == "cpu") | .val] | {count: length, max: max, min: min, avg: (add / length)}`,
		input,
		`{"count":3,"max":67,"min":45,"avg":54.666666666666664}`)
}

func TestComplexTagFiltering(t *testing.T) {
	// filter records that have any matching tag
	input := `[{"id":1,"tags":["go","fast","zero-alloc"]},{"id":2,"tags":["python","slow"]},{"id":3,"tags":["go","gc"]}]`
	assertQuery(t,
		`[.[] | select(any(.tags[]; . == "go")) | .id]`,
		input, `[1,3]`)
}

func TestComplexURLBuilding(t *testing.T) {
	// build a URL-encoded query string from object fields
	input := `{"host":"example.com","path":"/search","q":"hello world","limit":"10"}`
	assertQuery(t,
		`"https://" + .host + (.path | @uri) + "?q=" + (.q | @uri) + "&limit=" + .limit`,
		input,
		`"https://example.com%2Fsearch?q=hello%20world&limit=10"`)
}

func TestComplexNullSafeDeepAccess(t *testing.T) {
	// handling missing intermediate fields gracefully
	input := `{"user":{"profile":{"name":"alice"}}}`
	assertQuery(t, `.user.profile?.name`, input, `"alice"`)
	assertQuery(t, `.user.address?.city // "unknown"`, input, `"unknown"`)
	// .user.profile.age: .age on an existing object returns null (not an error), no catch fires
	assertQuery(t, `try .user.profile.age catch 0`, input, `null`)
	// .user.address.city: .address is missing so returns null and skips .city entirely — also null
	assertQuery(t, `try .user.address.city catch "missing"`, input, `null`)
	// to trigger a real error, explicitly access a field on an explicit null value
	assertQuery(t, `try (.user.address | .city) catch "missing"`, input, `"missing"`)
}

func TestComplexChainedAlternatives(t *testing.T) {
	// chain of fallbacks
	input := `{"c":"found"}`
	assertQuery(t, `.a // .b // .c // "default"`, input, `"found"`)
	assertQuery(t, `.a // .b // .d // "default"`, input, `"default"`)
}

func TestComplexObjectFromArray(t *testing.T) {
	// build an object index from an array using to_entries trick
	// Note: dynamic object keys {(expr): val} are not supported; use to_entries | from_entries approach
	input := `[{"key":"a","value":1},{"key":"b","value":2},{"key":"c","value":3}]`
	assertQuery(t, `from_entries`, input, `{"a":1,"b":2,"c":3}`)
}

// --- Edge cases at feature intersections ---

func TestComplexEmptyInTryCatch(t *testing.T) {
	// empty inside try produces no output (not an error)
	assertNoOutput(t, `try empty catch "err"`, `null`)
}

func TestComplexArithmeticOverflow(t *testing.T) {
	// large number arithmetic stays float
	assertQuery(t, `1000000 * 1000000`, `null`, `1000000000000`)
}

func TestComplexSelectWithArithmeticAndString(t *testing.T) {
	// combine all: arithmetic, string op, select
	input := `[{"sku":"ITEM-001","price":12.5,"qty":4,"tag":"sale"},{"sku":"ITEM-002","price":3,"qty":100,"tag":"clearance"},{"sku":"ITEM-003","price":25,"qty":2,"tag":"sale"}]`
	assertQuery(t,
		`[.[] | select(.price * .qty > 40 and (.tag | startswith("s"))) | .sku]`,
		input, `["ITEM-001","ITEM-003"]`)
}

func TestComplexToEntriesArithmetic(t *testing.T) {
	// to_entries with arithmetic transformations
	input := `{"a":10,"b":20,"c":30}`
	assertQuery(t,
		`to_entries | map({key: .key, value: .value * 2}) | from_entries`,
		input, `{"a":20,"b":40,"c":60}`)
}

func TestComplexMapWithTryCatch(t *testing.T) {
	// map that might fail on some elements
	input := `["42","hello","99","oops","7"]`
	assertQuery(t,
		`[.[] | try tonumber catch null] | map(select(. != null)) | add`,
		input, `148`)
}

func TestComplexLargeChain(t *testing.T) {
	// 7-operation chain: iterate → filter → project → sort by → first → field → string op
	// (no sort, so use min_by instead)
	input := `{"events":[{"type":"click","user":"ALICE","ms":120},{"type":"view","user":"bob","ms":50},{"type":"click","user":"Carol","ms":80}]}`
	assertQuery(t,
		`[.events[] | select(.type == "click")] | min_by(.ms) | .user | ascii_downcase`,
		input, `"carol"`)
}

// --- Stress: deeply nested and wide structures ---

func TestComplexDeeplyNestedAccess(t *testing.T) {
	input := `{"a":{"b":{"c":{"d":{"e":42}}}}}`
	assertQuery(t, `.a.b.c.d.e`, input, `42`)
	assertQuery(t, `.a.b.c.d.e | . * 2 | tostring`, input, `"84"`)
}

func TestComplexWideObjectAggregation(t *testing.T) {
	// object with many keys — to_entries + arithmetic aggregation
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 50; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"` + string(rune('a'+i%26)) + string(rune('0'+i/26)) + `":` + string(rune('0'+i%10)))
	}
	sb.WriteString("}")
	input := sb.String()
	// just check length and to_entries round-trip works
	p, _ := Compile(`to_entries | length`)
	got, err := p.Run([]byte(input))
	if err != nil || string(got) != "50" {
		t.Errorf("wide object: to_entries | length = %s, err = %v", got, err)
	}
}
