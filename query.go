package fastjq

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// opType represents the type of operation in the AST.
type opType int

const (
	opIdentity         opType = iota // .
	opField                          // .foo
	opDelete                         // del(.foo)
	opPipe                           // expr | expr
	opApply                          // expr[...], expr.foo — postfix application with original-input scope
	opBind                           // expr as $x | body
	opLabel                          // label $x | body
	opBreakOp                        // break $x
	opVar                            // $x
	opReduce                         // reduce gen as $x (init; update)
	opForeach                        // foreach gen as $x (init; update; extract?)
	opWhile                          // while(cond; update)
	opRepeat                         // repeat(expr)
	opUntil                          // until(cond; next)
	opDefScope                       // def f(...): body; expr
	opCall                           // f / f(a; b)
	opAssign                         // lhs = rhs
	opUpdate                         // lhs |= rhs
	opUpdateAlt                      // lhs //= rhs
	opUpdateMath                     // lhs += rhs, -=, *=, /=, %=
	opIndex                          // .[0], .[-1]
	opIndexExpr                      // .[expr] dynamic index/key expression
	opIterator                       // .[]
	opRecursiveDescent               // ..
	opRecurse                        // recurse / recurse(f) / recurse(f; cond)
	opWalk                           // walk(f)
	opConstruct                      // {name, a: .foo}
	opArrayConstruct                 // [.foo, .bar]
	opLiteral                        // null, true, false, "string", 123
	opCompare                        // ==, !=, <, <=, >, >=
	opSelect                         // select(cond)
	opAlternative                    // expr // expr
	opTypeBuiltin                    // type builtin
	opAnd                            // expr and expr
	opOr                             // expr or expr
	opNot                            // not
	opNeg                            // -expr
	opOptional                       // expr? — suppress errors from child expression
	opEmpty                          // empty — produce zero outputs
	opHas                            // has("key")
	opIf                             // if cond then expr else expr end
	opLength                         // length
	opAbs                            // abs
	opToEntries                      // to_entries
	opFromEntries                    // from_entries
	opAdd                            // add
	opFlatten                        // flatten / flatten(n)
	opSlice                          // .[n:m], .[:m], .[n:]
	opPlus                           // expr + expr
	opIndex1                         // index(s) — first occurrence
	opRIndex1                        // rindex(s) — last occurrence
	opIndicesN                       // indices(s) — all occurrences
	opDebug                          // debug — print to stderr, pass through
	opBase64                         // @base64 — encode string to base64
	opBase64D                        // @base64d — decode base64 string
	opValues                         // values — stream non-null values of object/array
	opIn                             // in(obj) — reverse membership test
	opSplit                          // split("s")
	opJoin                           // join("s")
	opStrftime                       // strftime(fmt)
	opStrfLocaltime                  // strflocaltime(fmt)
	opStrptime                       // strptime(fmt)
	opMktime                         // mktime
	opGmtime                         // gmtime
	opFromdate                       // fromdate
	opTodate                         // todate / date (compat alias)
	opNow                            // now
	opToStream                       // tostream
	opTruncateStream                 // truncate_stream(stream)
	opFromStream                     // fromstream(stream)
	opAsciiDowncase                  // ascii_downcase
	opAsciiUpcase                    // ascii_upcase
	opStartsWith                     // startswith("s")
	opEndsWith                       // endswith("s")
	opTrim                           // trim
	opLtrim                          // ltrim
	opRtrim                          // rtrim
	opTrimStr                        // trimstr("s")
	opLtrimStr                       // ltrimstr("s")
	opRtrimStr                       // rtrimstr("s")
	opHaveDecnum                     // have_decnum
	opUTF8ByteLength                 // utf8bytelength
	opReverse                        // reverse
	opCombinations                   // combinations / combinations(n)
	opPick                           // pick(path, ...)
	opBsearch                        // bsearch(x)
	opINBuiltin                      // IN(gen) / IN(lhs; rhs)
	opINDEXBuiltin                   // INDEX(gen; key)
	opJOINBuiltin                    // JOIN(index; key)
	opPath                           // path(expr)
	opKeys                           // keys
	opKeysUnsorted                   // keys_unsorted
	opBuiltins                       // builtins
	opPaths                          // paths / paths(filter)
	opGetPath                        // getpath(path)
	opSetPath                        // setpath(path; value)
	opDelPaths                       // delpaths(paths)
	opAny                            // any / any(expr)
	opAll                            // all / all(expr)
	opFirst                          // first(expr)
	opLast                           // last(expr)
	opLimit                          // limit(n; expr)
	opSkip                           // skip(n; expr)
	opMinus                          // expr - expr
	opMul                            // expr * expr
	opDiv                            // expr / expr
	opMod                            // expr % expr
	opMin                            // min
	opMax                            // max
	opMinBy                          // min_by(f)
	opMaxBy                          // max_by(f)
	opURIEncode                      // @uri
	opTry                            // try expr / try expr catch handler
	opToJSON                         // tojson / @json
	opFromJSON                       // fromjson
	opToString                       // tostring
	opToNumber                       // tonumber
	opToBoolean                      // toboolean
	opContains                       // contains(val) — recursive containment; optional=true for inside()
	opFloor                          // floor
	opCeil                           // ceil
	opRound                          // round
	opError                          // error — throw input as error
	opGenerator                      // a, b — multi-output sequence; elems = exprs to run in order
	opHTMLEncode                     // @html
	opCSVEncode                      // @csv
	opTSVEncode                      // @tsv
	opShEncode                       // @sh
	opURIDecode                      // @urid
	// 1-arg floating-point math builtins — all zero-alloc, all take number input.
	// Remaining deferred higher-arity numeric builtins such as atan(y; x) are still
	// documented in docs/SYNTAX.md. pow/hypot/fma are now implemented.
	opMathSqrt       // sqrt
	opMathFabs       // fabs  (absolute value of number; distinct from length)
	opMathAtan       // atan  (1-arg: atan(x); 2-arg atan(y;x) not supported)
	opMathLog        // log   (natural log)
	opMathLog2       // log2
	opMathLog10      // log10
	opMathExp        // exp   (e^x)
	opMathExp2       // exp2  (2^x)
	opMathExp10      // exp10 (10^x; implemented as pow(10,x))
	opMathCbrt       // cbrt  (cube root)
	opMathLogb       // logb  (base-2 exponent)
	opMathNearbyint  // nearbyint (round; approximation: uses round-half-away-from-zero)
	opMathJ0         // j0   (Bessel function of first kind, order 0)
	opMathJ1         // j1   (Bessel function of first kind, order 1)
	opMathSin        // sin
	opMathCos        // cos
	opMathTan        // tan
	opMathAsin       // asin
	opMathAcos       // acos
	opMathTgamma     // tgamma (gamma function, Γ(x))
	opMathLgamma     // lgamma (log of absolute gamma, ln|Γ(x)|)
	opStringInterp   // "\(expr)" string interpolation; elems=expressions, segs=literal segments
	opFormatTemplate // @html "...\(...)..." — format each interpolation inside a template string
	opIsEmpty        // isempty(expr) — true if expr produces no outputs
	opNth            // nth(n; gen) — nth output of gen (0-indexed); left=n, child=gen
	// Regex operations (Go RE2 engine — linear time, pattern compiled once at Compile())
	opTest    // test(re) / test(re; flags)  — 0 allocs via re.Match
	opMatchRe // match(re) / match(re; flags) — 1 alloc on match (FindSubmatchIndex []int)
	opCapture // capture(re) / capture(re; flags) — 1 alloc on match
	opScan    // scan(re) / scan(re; flags)   — allocs per match (multi-output)
	opSub     // sub(re; "literal")           — replace first match
	opGSub    // gsub(re; "literal")          — replace all matches
	// range — Tier 2 (1 alloc per generated value, proportional to output count)
	opRange // range(n) / range(from;to) / range(from;to;step)
	// left=from, right=to, child=step (nil → step 1)
	// Sort / unique / group — Tier 2 (allocate O(n) index proportional to collection size)
	opSort      // sort
	opSortBy    // sort_by(f) — child=key function
	opUnique    // unique
	opUniqueBy  // unique_by(f) — child=key function
	opGroupBy   // group_by(f) — child=key function
	opTranspose // transpose
	opExplode   // explode — string → array of Unicode codepoints (Tier 2)
	opImplode   // implode — array of codepoints → string (Tier 2)
	// nan/infinite predicates
	opIsNaN      // isnan  — true if input is NaN
	opIsInfinite // isinfinite — true if input is ±Inf
	opIsFinite   // isfinite — true if finite and not NaN
	opIsNormal   // isnormal — true if non-zero, finite, not subnormal
	opPow        // pow(x; y) — left=x, right=y
	opHypot      // hypot(x; y) — left=x, right=y
	opFMA        // fma(x; y; z) — left=x, right=y, child=z
)

// cmpOperator is the comparison operator used in opCompare nodes.
type cmpOperator int

const (
	cmpEq  cmpOperator = iota // ==
	cmpNeq                    // !=
	cmpLt                     // <
	cmpLe                     // <=
	cmpGt                     // >
	cmpGe                     // >=
)

type updateOperator int

const (
	updatePlus updateOperator = iota
	updateMinus
	updateMul
	updateDiv
	updateMod
)

// pair represents a key-expression pair in object construction.
type pair struct {
	key     string
	keyExpr *op
	expr    *op
}

// op is a node in the query AST.
type op struct {
	typ             opType
	field           string         // for opField
	name            string         // for opVar/opBind
	fn              *funcDef       // for opDefScope
	pattern         *bindPattern   // for opBind/opReduce/opForeach destructuring bindings
	altPatterns     []*bindPattern // for opBind: fallback binding targets joined by ?//
	fields          []op           // for opDelete: list of field-access/index paths to delete
	left            *op            // for opPipe, opCompare, opAlternative, opNth
	right           *op            // for opPipe, opCompare, opAlternative
	child           *op            // for opField chaining, opSelect condition, opIsEmpty, opNth body
	extra           *op            // for opForeach: extract expression
	format          opType         // for opFormatTemplate: formatter applied to each interpolation
	index           int            // for opIndex: array index (negative = from end)
	pairs           []pair         // for opConstruct: {key: expr} pairs
	multiValuePairs bool           // for opConstruct: true if any pair expr may produce >1 output
	elems           []*op          // for opArrayConstruct, opStringInterp, opFormatTemplate: expressions
	segs            [][]byte       // for opStringInterp/opFormatTemplate: literal segments between expressions
	literal         []byte         // for opLiteral: raw JSON bytes
	re              *regexp.Regexp // for regex ops (opTest/opMatchRe/opCapture/opScan/opSub/opGSub)
	cmpOp           cmpOperator    // for opCompare: comparison operator
	updateOp        updateOperator // for opUpdateMath
	optional        bool           // for opField/opIndex/opIterator: suppress errors
}

type bindPatternKind int

const (
	bindPatternVar bindPatternKind = iota
	bindPatternArray
	bindPatternObject
)

type bindPattern struct {
	kind   bindPatternKind
	name   string
	elems  []*bindPattern
	fields []bindPatternField
}

type bindPatternField struct {
	key      string
	bindName string
	pattern  *bindPattern
}

// parse compiles a jq query string into an AST.
func parse(query string) (*op, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	result, rest, err := parseGeneratorExpr(query)
	if err != nil {
		return nil, err
	}
	rest = strings.TrimSpace(rest)

	if rest != "" {
		return nil, fmt.Errorf("unexpected trailing input: %q", rest)
	}

	// Optimization: simplify identity pipes
	result = simplify(result)

	if err := validateVars(result, nil); err != nil {
		return nil, err
	}
	if err := validateLabels(result, nil); err != nil {
		return nil, err
	}
	if err := validateFuncs(result, nil, nil); err != nil {
		return nil, err
	}

	return result, nil
}

// parsePipeExpr parses a pipe chain: expr | expr | ...
func parsePipeExpr(s string) (*op, string, error) {
	result, rest, err := parseBindExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	for strings.HasPrefix(rest, "|") {
		rest = strings.TrimSpace(rest[1:])
		right, remainder, err := parseBindExpr(rest)
		if err != nil {
			return nil, remainder, err
		}
		result = &op{typ: opPipe, left: result, right: right}
		rest = strings.TrimSpace(remainder)
	}

	return result, rest, nil
}

// parseBindExpr parses `expr as $name | body`.
// The bound value is produced by expr, but body runs against the original input.
func parseBindExpr(s string) (*op, string, error) {
	left, rest, err := parseAssignExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !(strings.HasPrefix(rest, "as") && (len(rest) == 2 || !isIdentChar(rest[2]))) {
		return left, rest, nil
	}
	asRest := rest
	rest = strings.TrimSpace(rest[2:])
	name, pattern, remaining, ok, err := parseBindingTarget(rest)
	if err != nil {
		return nil, remaining, err
	}
	if !ok {
		return left, asRest, nil
	}
	remaining = strings.TrimSpace(remaining)
	primaryPattern := bindingPatternFromTarget(name, pattern)
	var altPatterns []*bindPattern
	for strings.HasPrefix(remaining, "?//") {
		nextName, nextPattern, nextRemaining, nextOK, err := parseBindingTarget(strings.TrimSpace(remaining[3:]))
		if err != nil {
			return nil, nextRemaining, err
		}
		if !nextOK {
			return nil, remaining, fmt.Errorf("expected binding target after ?//")
		}
		altPatterns = append(altPatterns, bindingPatternFromTarget(nextName, nextPattern))
		remaining = strings.TrimSpace(nextRemaining)
	}
	if len(remaining) == 0 || remaining[0] != '|' {
		return left, asRest, nil
	}
	body, rest, err := parseGeneratorExpr(remaining[1:])
	if err != nil {
		return nil, rest, err
	}
	return &op{typ: opBind, name: name, pattern: primaryPattern, altPatterns: altPatterns, left: left, right: body}, rest, nil
}

func bindingPatternFromTarget(name string, pattern *bindPattern) *bindPattern {
	if pattern != nil {
		return pattern
	}
	if name == "" {
		return nil
	}
	return &bindPattern{kind: bindPatternVar, name: name}
}

func parseAssignExpr(s string) (*op, string, error) {
	left, rest, err := parseExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !canStartAssignment(left) {
		return left, rest, nil
	}

	switch {
	case strings.HasPrefix(rest, "//="):
		right, remaining, err := parseExpr(strings.TrimSpace(rest[3:]))
		if err != nil {
			return nil, remaining, err
		}
		return &op{typ: opUpdateAlt, left: left, right: right}, remaining, nil
	case strings.HasPrefix(rest, "|="):
		right, remaining, err := parseExpr(strings.TrimSpace(rest[2:]))
		if err != nil {
			return nil, remaining, err
		}
		return &op{typ: opUpdate, left: left, right: right}, remaining, nil
	case strings.HasPrefix(rest, "+="):
		return parseUpdateMathExpr(left, rest[2:], updatePlus)
	case strings.HasPrefix(rest, "-="):
		return parseUpdateMathExpr(left, rest[2:], updateMinus)
	case strings.HasPrefix(rest, "*="):
		return parseUpdateMathExpr(left, rest[2:], updateMul)
	case strings.HasPrefix(rest, "/="):
		return parseUpdateMathExpr(left, rest[2:], updateDiv)
	case strings.HasPrefix(rest, "%="):
		return parseUpdateMathExpr(left, rest[2:], updateMod)
	case len(rest) > 0 && rest[0] == '=' && (len(rest) == 1 || rest[1] != '='):
		right, remaining, err := parseExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, remaining, err
		}
		return &op{typ: opAssign, left: left, right: right}, remaining, nil
	default:
		return left, rest, nil
	}
}

func parseUpdateMathExpr(left *op, s string, update updateOperator) (*op, string, error) {
	right, remaining, err := parseExpr(strings.TrimSpace(s))
	if err != nil {
		return nil, remaining, err
	}
	return &op{typ: opUpdateMath, left: left, right: right, updateOp: update}, remaining, nil
}

func canStartAssignment(node *op) bool {
	if node == nil {
		return false
	}
	if containsUnsupportedAssignNode(node) {
		return false
	}
	return true
}

func containsUnsupportedAssignNode(node *op) bool {
	if node == nil {
		return false
	}
	switch node.typ {
	case opReduce, opForeach, opDefScope:
		return true
	case opIndexExpr:
		return containsUnsupportedAssignNode(node.left) || containsUnsupportedAssignNode(node.child)
	case opConstruct:
		for _, p := range node.pairs {
			if containsUnsupportedAssignNode(p.expr) {
				return true
			}
		}
		return false
	case opArrayConstruct, opGenerator, opStringInterp, opFormatTemplate:
		for _, elem := range node.elems {
			if containsUnsupportedAssignNode(elem) {
				return true
			}
		}
		return false
	default:
		return containsUnsupportedAssignNode(node.left) || containsUnsupportedAssignNode(node.right) || containsUnsupportedAssignNode(node.child)
	}
}

func parseLabelExpr(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '$' {
		return nil, s, fmt.Errorf("expected label name after label")
	}
	name, rest := readIdentifier(s[1:])
	if name == "" {
		return nil, rest, fmt.Errorf("expected label name after label")
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '|' {
		return nil, rest, fmt.Errorf("expected '|' after label $%s", name)
	}
	body, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	return &op{typ: opLabel, name: name, child: body}, rest, nil
}

func parseBreakExpr(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '$' {
		return nil, s, fmt.Errorf("expected label name after break")
	}
	name, rest := readIdentifier(s[1:])
	if name == "" {
		return nil, rest, fmt.Errorf("expected label name after break")
	}
	return &op{typ: opBreakOp, name: name}, rest, nil
}

// parseGeneratorExpr parses generator syntax in contexts where commas produce
// multiple outputs and bind tighter than pipes. This matches jq's parsing for
// forms like `[a, b | f]`, which is `[(a, b) | f]`, not `[a, (b | f)]`.
func parseGeneratorExpr(s string) (*op, string, error) {
	first, rest, err := parseGeneratorTerm(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	for strings.HasPrefix(rest, "|") {
		rest = strings.TrimSpace(rest[1:])
		right, remainder, err := parseGeneratorTerm(rest)
		if err != nil {
			return nil, remainder, err
		}
		first = &op{typ: opPipe, left: first, right: right}
		rest = strings.TrimSpace(remainder)
	}

	return first, rest, nil
}

// parseGeneratorTerm parses a comma-separated generator term where each element
// is a regular expression operand. Returns a single op if there is only one
// element, or an opGenerator if there are multiple.
func parseGeneratorTerm(s string) (*op, string, error) {
	first, rest, err := parseBindExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ',' {
		return first, rest, nil
	}
	elems := []*op{first}
	for len(rest) > 0 && rest[0] == ',' {
		rest = strings.TrimSpace(rest[1:])
		next, rest2, err := parseBindExpr(rest)
		if err != nil {
			return nil, rest2, err
		}
		elems = append(elems, next)
		rest = strings.TrimSpace(rest2)
	}
	return &op{typ: opGenerator, elems: elems}, rest, nil
}

// parseExpr parses a single expression at the lowest precedence level.
// Precedence chain: parseExpr → parseAlt → parseCmp → parseAtom
func parseExpr(s string) (*op, string, error) {
	return parseAlt(s)
}

// parseAlt parses alternative expressions: expr // expr // ...
// Left-associative. Delegates down to parseOr.
func parseAlt(s string) (*op, string, error) {
	left, rest, err := parseOr(s)
	if err != nil {
		return nil, rest, err
	}
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) >= 2 && rest[0] == '/' && rest[1] == '/' {
			if len(rest) >= 3 && rest[2] == '=' {
				break
			}
			rest = strings.TrimSpace(rest[2:])
			right, remainder, err := parseCmp(rest)
			if err != nil {
				return nil, remainder, err
			}
			left = &op{typ: opAlternative, left: left, right: right}
			rest = remainder
			continue
		}
		break
	}
	return left, rest, nil
}

// parseOr parses: expr or expr or ...
// Left-associative. Delegates down to parseAnd.
func parseOr(s string) (*op, string, error) {
	left, rest, err := parseAnd(s)
	if err != nil {
		return nil, rest, err
	}
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) >= 2 && rest[0] == 'o' && rest[1] == 'r' && (len(rest) == 2 || !isIdentChar(rest[2])) {
			rest = strings.TrimSpace(rest[2:])
			right, remainder, err := parseAnd(rest)
			if err != nil {
				return nil, remainder, err
			}
			left = &op{typ: opOr, left: left, right: right}
			rest = remainder
			continue
		}
		break
	}
	return left, rest, nil
}

// parseAnd parses: expr and expr and ...
// Left-associative. Delegates down to parseCmp.
func parseAnd(s string) (*op, string, error) {
	left, rest, err := parseCmp(s)
	if err != nil {
		return nil, rest, err
	}
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) >= 3 && rest[0] == 'a' && rest[1] == 'n' && rest[2] == 'd' && (len(rest) == 3 || !isIdentChar(rest[3])) {
			rest = strings.TrimSpace(rest[3:])
			right, remainder, err := parseCmp(rest)
			if err != nil {
				return nil, remainder, err
			}
			left = &op{typ: opAnd, left: left, right: right}
			rest = remainder
			continue
		}
		break
	}
	return left, rest, nil
}

// parseAddExpr parses additive expressions: expr + expr, expr - expr (left-associative).
// Delegates down to parseMulExpr.
func parseAddExpr(s string) (*op, string, error) {
	left, rest, err := parseMulExpr(s)
	if err != nil {
		return nil, rest, err
	}
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && rest[0] == '+' {
			if len(rest) > 1 && rest[1] == '=' {
				break
			}
			rest = strings.TrimSpace(rest[1:])
			right, remainder, err := parseMulExpr(rest)
			if err != nil {
				return nil, remainder, err
			}
			left = &op{typ: opPlus, left: left, right: right}
			rest = remainder
			continue
		}
		if len(rest) > 0 && rest[0] == '-' {
			if len(rest) > 1 && rest[1] == '=' {
				break
			}
			rest = strings.TrimSpace(rest[1:])
			right, remainder, err := parseMulExpr(rest)
			if err != nil {
				return nil, remainder, err
			}
			left = &op{typ: opMinus, left: left, right: right}
			rest = remainder
			continue
		}
		break
	}
	return left, rest, nil
}

// parseUnaryExpr parses prefix unary operators such as -expr.
// Delegates down to parseAtom.
func parseUnaryExpr(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] == '-' {
		if strings.HasPrefix(s, "-nan") && (len(s) == 4 || !isIdentChar(s[4])) {
			return parseAtom(s)
		}
		if strings.HasPrefix(s, "-infinite") && (len(s) == 9 || !isIdentChar(s[9])) {
			return parseAtom(s)
		}
		if len(s) == 1 || !isDigit(s[1]) {
			right, rest, err := parseUnaryExpr(s[1:])
			if err != nil {
				return nil, rest, err
			}
			return &op{typ: opNeg, child: right}, rest, nil
		}
	}
	return parseAtom(s)
}

// parseMulExpr parses multiplicative expressions: expr * expr, expr / expr, expr % expr (left-associative).
// Delegates down to parseUnaryExpr.
func parseMulExpr(s string) (*op, string, error) {
	left, rest, err := parseUnaryExpr(s)
	if err != nil {
		return nil, rest, err
	}
	for {
		rest = strings.TrimSpace(rest)
		var typ opType
		if len(rest) > 0 && rest[0] == '*' {
			if len(rest) > 1 && rest[1] == '=' {
				break
			}
			typ = opMul
		} else if len(rest) > 0 && rest[0] == '/' && !(len(rest) >= 2 && (rest[1] == '/' || rest[1] == '=')) {
			typ = opDiv
		} else if len(rest) > 0 && rest[0] == '%' {
			if len(rest) > 1 && rest[1] == '=' {
				break
			}
			typ = opMod
		} else {
			break
		}
		rest = strings.TrimSpace(rest[1:])
		right, remainder, err := parseUnaryExpr(rest)
		if err != nil {
			return nil, remainder, err
		}
		left = &op{typ: typ, left: left, right: right}
		rest = remainder
	}
	return left, rest, nil
}

// parseCmp parses comparison expressions: ==, !=, <, <=, >, >=
// Non-associative (no chaining). Delegates down to parseAddExpr.
func parseCmp(s string) (*op, string, error) {
	left, rest, err := parseAddExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	var operator cmpOperator
	var advance int
	switch {
	case len(rest) >= 2 && rest[0] == '=' && rest[1] == '=':
		operator, advance = cmpEq, 2
	case len(rest) >= 2 && rest[0] == '!' && rest[1] == '=':
		operator, advance = cmpNeq, 2
	case len(rest) >= 2 && rest[0] == '<' && rest[1] == '=':
		operator, advance = cmpLe, 2
	case len(rest) >= 2 && rest[0] == '>' && rest[1] == '=':
		operator, advance = cmpGe, 2
	case len(rest) >= 1 && rest[0] == '<':
		operator, advance = cmpLt, 1
	case len(rest) >= 1 && rest[0] == '>':
		operator, advance = cmpGt, 1
	}
	if advance == 0 {
		return left, rest, nil
	}
	rest = strings.TrimSpace(rest[advance:])
	right, remainder, err := parseAtom(rest)
	if err != nil {
		return nil, remainder, err
	}
	return &op{typ: opCompare, left: left, right: right, cmpOp: operator}, remainder, nil
}

// parseAtom parses a single atomic expression (not including pipe, alternative, or comparison).
func parseAtom(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", fmt.Errorf("unexpected end of expression")
	}

	// String literal
	if s[0] == '"' {
		return parseStringLiteral(s)
	}

	// Variable reference: $name
	if s[0] == '$' {
		return parseVarRef(s)
	}
	if strings.HasPrefix(s, "break") && (len(s) == 5 || !isIdentChar(s[5])) {
		return parseBreakExpr(s[5:])
	}
	if strings.HasPrefix(s, "def") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseDefExpr(s[3:])
	}

	// del()
	if strings.HasPrefix(s, "del(") {
		return parseDel(s)
	}

	// select()
	if strings.HasPrefix(s, "select(") {
		return parseSelect(s)
	}
	if strings.HasPrefix(s, "walk(") {
		node, rest, err := parseUnaryGenBuiltin(s[5:], opWalk)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}
	if strings.HasPrefix(s, "recurse") && (len(s) == 7 || !isIdentChar(s[7])) {
		rest := strings.TrimSpace(s[7:])
		if len(rest) > 0 && rest[0] == '(' {
			node, tail, err := parseRecurse(rest[1:])
			if err != nil {
				return nil, tail, err
			}
			return applyPostfixPipe(node, tail)
		}
		return applyPostfixPipe(&op{typ: opRecurse}, rest)
	}

	// null literal (with boundary check)
	if strings.HasPrefix(s, "null") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opLiteral, literal: []byte("null")}, s[4:], nil
	}

	// true literal (with boundary check)
	if strings.HasPrefix(s, "true") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opLiteral, literal: []byte("true")}, s[4:], nil
	}

	// false literal (with boundary check)
	if strings.HasPrefix(s, "false") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opLiteral, literal: []byte("false")}, s[5:], nil
	}

	// try / try-catch
	if strings.HasPrefix(s, "try") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseTry(s[3:])
	}
	if strings.HasPrefix(s, "label") && (len(s) == 5 || !isIdentChar(s[5])) {
		return parseLabelExpr(s[5:])
	}

	// if-then-else
	if strings.HasPrefix(s, "if") && (len(s) == 2 || !isIdentChar(s[2])) {
		node, rest, err := parseIf(s)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}

	// empty — produce zero outputs
	if strings.HasPrefix(s, "empty") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opEmpty}, s[5:], nil
	}
	// has("key")
	if strings.HasPrefix(s, "has(") {
		return parseHas(s)
	}

	// length builtin
	if strings.HasPrefix(s, "length") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opLength}, s[6:], nil
	}
	if strings.HasPrefix(s, "abs") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opAbs}, s[3:], nil
	}

	// first / last — no-arg desugar to .[0] / .[-1]; with arg use dedicated op
	if strings.HasPrefix(s, "first") && (len(s) == 5 || !isIdentChar(s[5])) {
		rest := strings.TrimSpace(s[5:])
		if len(rest) > 0 && rest[0] == '(' {
			inner, rest2, err := parseGeneratorExpr(rest[1:])
			if err != nil {
				return nil, rest2, err
			}
			rest2 = strings.TrimSpace(rest2)
			if len(rest2) == 0 || rest2[0] != ')' {
				return nil, rest2, fmt.Errorf("expected ')' after first() argument")
			}
			return &op{typ: opFirst, child: inner}, rest2[1:], nil
		}
		return &op{typ: opIndex, index: 0}, rest, nil // first → .[0]
	}
	if strings.HasPrefix(s, "last") && (len(s) == 4 || !isIdentChar(s[4])) {
		rest := strings.TrimSpace(s[4:])
		if len(rest) > 0 && rest[0] == '(' {
			inner, rest2, err := parseGeneratorExpr(rest[1:])
			if err != nil {
				return nil, rest2, err
			}
			rest2 = strings.TrimSpace(rest2)
			if len(rest2) == 0 || rest2[0] != ')' {
				return nil, rest2, fmt.Errorf("expected ')' after last() argument")
			}
			return &op{typ: opLast, child: inner}, rest2[1:], nil
		}
		return &op{typ: opIndex, index: -1}, rest, nil // last → .[-1]
	}

	// limit(n; expr) — body can be a comma-separated generator: limit(1; a, b)
	if strings.HasPrefix(s, "limit(") {
		nExpr, rest, err := parseGeneratorExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in limit(n; expr)")
		}
		rest = strings.TrimSpace(rest[1:])
		genExpr, rest, err := parseGeneratorExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after limit() arguments")
		}
		limits := make([]*op, 0, len(generatorElems(nExpr)))
		for _, countExpr := range generatorElems(nExpr) {
			limits = append(limits, &op{typ: opLimit, left: countExpr, child: genExpr})
		}
		return collapseGeneratorNodes(limits), rest[1:], nil
	}
	if strings.HasPrefix(s, "skip(") {
		nExpr, rest, err := parseGeneratorExpr(s[5:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in skip(n; expr)")
		}
		rest = strings.TrimSpace(rest[1:])
		genExpr, rest, err := parseGeneratorExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after skip() arguments")
		}
		return &op{typ: opSkip, left: nExpr, child: genExpr}, rest[1:], nil
	}

	// keys / keys_unsorted
	if strings.HasPrefix(s, "keys") && (len(s) == 4 || !isIdentChar(s[4])) {
		return applyPostfixPipe(&op{typ: opKeys}, s[4:])
	}
	if strings.HasPrefix(s, "keys_unsorted") && (len(s) == 13 || !isIdentChar(s[13])) {
		return applyPostfixPipe(&op{typ: opKeysUnsorted}, s[13:])
	}
	if strings.HasPrefix(s, "builtins") && (len(s) == 8 || !isIdentChar(s[8])) {
		return applyPostfixPipe(&op{typ: opBuiltins}, s[8:])
	}
	if strings.HasPrefix(s, "have_decnum") && (len(s) == 11 || !isIdentChar(s[11])) {
		return &op{typ: opHaveDecnum}, s[11:], nil
	}
	if strings.HasPrefix(s, "path(") {
		node, rest, err := parseUnaryExprBuiltin(s[5:], opPath)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}
	if strings.HasPrefix(s, "leaf_paths") && (len(s) == 10 || !isIdentChar(s[10])) {
		mkTypeNeq := func(t string) *op {
			return &op{
				typ:  opCompare,
				left: &op{typ: opTypeBuiltin},
				right: &op{
					typ:     opLiteral,
					literal: []byte(`"` + t + `"`),
				},
				cmpOp: cmpNeq,
			}
		}
		cond := &op{typ: opAnd, left: mkTypeNeq("array"), right: mkTypeNeq("object")}
		return &op{typ: opPaths, child: &op{typ: opSelect, child: cond}}, s[10:], nil
	}
	if strings.HasPrefix(s, "paths") && (len(s) == 5 || !isIdentChar(s[5])) {
		rest := strings.TrimSpace(s[5:])
		if len(rest) == 0 || rest[0] != '(' {
			return &op{typ: opPaths}, rest, nil
		}
		return parseUnaryExprBuiltin(rest[1:], opPaths)
	}
	if strings.HasPrefix(s, "getpath(") {
		return parseUnaryGenBuiltin(s[8:], opGetPath)
	}
	if strings.HasPrefix(s, "setpath(") {
		pathExpr, rest, err := parsePipeExpr(s[8:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in setpath(path; value)")
		}
		valueExpr, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after setpath() arguments")
		}
		return &op{typ: opSetPath, left: pathExpr, child: valueExpr}, rest[1:], nil
	}
	if strings.HasPrefix(s, "delpaths(") {
		return parseUnaryExprBuiltin(s[9:], opDelPaths)
	}
	if strings.HasPrefix(s, "reduce") && (len(s) == 6 || !isIdentChar(s[6])) {
		return parseReduce(s[6:])
	}
	if strings.HasPrefix(s, "foreach") && (len(s) == 7 || !isIdentChar(s[7])) {
		return parseForeach(s[7:])
	}

	// any / all — with optional (expr) argument
	if strings.HasPrefix(s, "any") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseAnyAll(s[3:], opAny)
	}
	if strings.HasPrefix(s, "all") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseAnyAll(s[3:], opAll)
	}
	if strings.HasPrefix(s, "while(") {
		cond, rest, err := parsePipeExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in while(cond; update)")
		}
		update, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after while() arguments")
		}
		return &op{typ: opWhile, left: cond, child: update}, rest[1:], nil
	}
	if strings.HasPrefix(s, "repeat(") {
		node, rest, err := parseGeneratorArgBuiltin(s[7:], opRepeat)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}
	if strings.HasPrefix(s, "until(") {
		cond, rest, err := parsePipeExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in until(cond; next)")
		}
		next, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after until() arguments")
		}
		return &op{typ: opUntil, left: cond, child: next}, rest[1:], nil
	}
	if strings.HasPrefix(s, "pick(") {
		args, rest, err := parseCommaArgList(s[5:])
		if err != nil {
			return nil, rest, err
		}
		return &op{typ: opPick, elems: args}, rest, nil
	}
	if strings.HasPrefix(s, "bsearch(") {
		return parseUnaryGenBuiltin(s[8:], opBsearch)
	}
	if strings.HasPrefix(s, "IN(") {
		left, rest, err := parseGeneratorExpr(s[3:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 {
			return nil, rest, fmt.Errorf("unclosed IN()")
		}
		if rest[0] == ')' {
			return &op{typ: opINBuiltin, child: left}, rest[1:], nil
		}
		if rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' or ')' in IN()")
		}
		right, rest, err := parseGeneratorExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after IN() arguments")
		}
		return &op{typ: opINBuiltin, left: left, child: right}, rest[1:], nil
	}
	if strings.HasPrefix(s, "INDEX(") {
		stream, rest, err := parseGeneratorExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in INDEX(gen; key)")
		}
		key, rest, err := parseGeneratorExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after INDEX() arguments")
		}
		return &op{typ: opINDEXBuiltin, left: stream, child: key}, rest[1:], nil
	}
	if strings.HasPrefix(s, "JOIN(") {
		indexExpr, rest, err := parsePipeExpr(s[5:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in JOIN(index; key)")
		}
		key, rest, err := parseGeneratorExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after JOIN() arguments")
		}
		return &op{typ: opJOINBuiltin, left: indexExpr, child: key}, rest[1:], nil
	}

	// add
	if strings.HasPrefix(s, "add") && (len(s) == 3 || !isIdentChar(s[3])) {
		rest := strings.TrimSpace(s[3:])
		if len(rest) > 0 && rest[0] == '(' {
			inner, rest2, err := parseGeneratorExpr(rest[1:])
			if err != nil {
				return nil, rest2, err
			}
			rest2 = strings.TrimSpace(rest2)
			if len(rest2) == 0 || rest2[0] != ')' {
				return nil, rest2, fmt.Errorf("expected ')' after add() expression")
			}
			return applyPostfixPipe(addBuiltinNode(inner), rest2[1:])
		}
		return &op{typ: opAdd}, s[3:], nil
	}

	// sort / sort_by — check _by first
	if strings.HasPrefix(s, "sort_by(") {
		return parseUnaryGenBuiltin(s[8:], opSortBy)
	}
	if strings.HasPrefix(s, "sort") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opSort}, s[4:], nil
	}
	// unique / unique_by
	if strings.HasPrefix(s, "unique_by(") {
		return parseUnaryGenBuiltin(s[10:], opUniqueBy)
	}
	if strings.HasPrefix(s, "unique") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opUnique}, s[6:], nil
	}
	// group_by
	if strings.HasPrefix(s, "group_by(") {
		return parseUnaryGenBuiltin(s[9:], opGroupBy)
	}
	// transpose
	if strings.HasPrefix(s, "transpose") && (len(s) == 9 || !isIdentChar(s[9])) {
		return &op{typ: opTranspose}, s[9:], nil
	}

	// min_by / min / max_by / max — check _by variants first
	if strings.HasPrefix(s, "min_by(") {
		return parseUnaryExprBuiltin(s[7:], opMinBy)
	}
	if strings.HasPrefix(s, "min") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMin}, s[3:], nil
	}
	if strings.HasPrefix(s, "max_by(") {
		return parseUnaryExprBuiltin(s[7:], opMaxBy)
	}
	if strings.HasPrefix(s, "max") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMax}, s[3:], nil
	}

	// flatten / flatten(n)
	if strings.HasPrefix(s, "flatten") && (len(s) == 7 || !isIdentChar(s[7])) {
		rest := strings.TrimSpace(s[7:])
		if len(rest) > 0 && rest[0] == '(' {
			depthExpr, rest2, err := parseGeneratorExpr(rest[1:])
			if err != nil {
				return nil, rest2, err
			}
			rest2 = strings.TrimSpace(rest2)
			if len(rest2) == 0 || rest2[0] != ')' {
				return nil, rest2, fmt.Errorf("expected ')' after flatten() argument")
			}
			flatteners := make([]*op, 0, len(generatorElems(depthExpr)))
			for _, depth := range generatorElems(depthExpr) {
				flatteners = append(flatteners, &op{typ: opFlatten, child: depth})
			}
			return collapseGeneratorNodes(flatteners), rest2[1:], nil
		}
		return &op{typ: opFlatten, index: -1}, rest, nil // -1 = unlimited depth
	}

	// index(s) / rindex(s) / indices(s)
	if strings.HasPrefix(s, "indices(") {
		return parseGeneratorArgBuiltin(s[8:], opIndicesN)
	}
	if strings.HasPrefix(s, "index(") {
		return parseGeneratorArgBuiltin(s[6:], opIndex1)
	}
	if strings.HasPrefix(s, "rindex(") {
		return parseGeneratorArgBuiltin(s[7:], opRIndex1)
	}

	// debug — print to stderr, pass through
	if strings.HasPrefix(s, "debug") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opDebug}, s[5:], nil
	}

	// values — non-null filter (desugars to select(. != null))
	if strings.HasPrefix(s, "values") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opValues}, s[6:], nil
	}

	// Type-filter builtins: select(type == "X") aliases
	for _, tf := range []struct {
		name, typeName string
	}{
		{"numbers", "number"}, {"strings", "string"}, {"arrays", "array"},
		{"objects", "object"}, {"booleans", "boolean"}, {"nulls", "null"},
	} {
		n := len(tf.name)
		if strings.HasPrefix(s, tf.name) && (len(s) == n || !isIdentChar(s[n])) {
			lit := &op{typ: opLiteral, literal: []byte(`"` + tf.typeName + `"`)}
			cmp := &op{typ: opCompare, left: &op{typ: opTypeBuiltin}, right: lit, cmpOp: cmpEq}
			return &op{typ: opSelect, child: cmp}, s[n:], nil
		}
	}
	// iterables = select(type == "array" or type == "object")
	if strings.HasPrefix(s, "iterables") && (len(s) == 9 || !isIdentChar(s[9])) {
		mkTypeCmp := func(t string) *op {
			return &op{typ: opCompare, left: &op{typ: opTypeBuiltin},
				right: &op{typ: opLiteral, literal: []byte(`"` + t + `"`)}, cmpOp: cmpEq}
		}
		cond := &op{typ: opOr, left: mkTypeCmp("array"), right: mkTypeCmp("object")}
		return &op{typ: opSelect, child: cond}, s[9:], nil
	}
	// scalars = select(type != "array" and type != "object")
	if strings.HasPrefix(s, "scalars") && (len(s) == 7 || !isIdentChar(s[7])) {
		mkTypeNeq := func(t string) *op {
			return &op{typ: opCompare, left: &op{typ: opTypeBuiltin},
				right: &op{typ: opLiteral, literal: []byte(`"` + t + `"`)}, cmpOp: cmpNeq}
		}
		cond := &op{typ: opAnd, left: mkTypeNeq("array"), right: mkTypeNeq("object")}
		return &op{typ: opSelect, child: cond}, s[7:], nil
	}

	// in(expr) — reverse membership test
	if strings.HasPrefix(s, "in(") {
		return parseUnaryExprBuiltin(s[3:], opIn)
	}

	// Format strings: @base64d, @base64, @uri, @urid, @json, @html, @csv, @tsv, @sh, @text
	if s[0] == '@' {
		if strings.HasPrefix(s, "@base64d") && (len(s) == 8 || !isIdentChar(s[8])) {
			return parseFormatBuiltin(s[8:], opBase64D)
		}
		if strings.HasPrefix(s, "@base64") && (len(s) == 7 || !isIdentChar(s[7])) {
			return parseFormatBuiltin(s[7:], opBase64)
		}
		if strings.HasPrefix(s, "@urid") && (len(s) == 5 || !isIdentChar(s[5])) {
			return parseFormatBuiltin(s[5:], opURIDecode)
		}
		if strings.HasPrefix(s, "@uri") && (len(s) == 4 || !isIdentChar(s[4])) {
			return parseFormatBuiltin(s[4:], opURIEncode)
		}
		if strings.HasPrefix(s, "@json") && (len(s) == 5 || !isIdentChar(s[5])) {
			return parseFormatBuiltin(s[5:], opToJSON)
		}
		if strings.HasPrefix(s, "@html") && (len(s) == 5 || !isIdentChar(s[5])) {
			return parseFormatBuiltin(s[5:], opHTMLEncode)
		}
		if strings.HasPrefix(s, "@csv") && (len(s) == 4 || !isIdentChar(s[4])) {
			return parseFormatBuiltin(s[4:], opCSVEncode)
		}
		if strings.HasPrefix(s, "@tsv") && (len(s) == 4 || !isIdentChar(s[4])) {
			return parseFormatBuiltin(s[4:], opTSVEncode)
		}
		if strings.HasPrefix(s, "@sh") && (len(s) == 3 || !isIdentChar(s[3])) {
			return parseFormatBuiltin(s[3:], opShEncode)
		}
		if strings.HasPrefix(s, "@text") && (len(s) == 5 || !isIdentChar(s[5])) {
			return parseFormatBuiltin(s[5:], opToString) // @text == tostring
		}
		return nil, s, fmt.Errorf("unsupported format string %q", s[:min(len(s), 16)])
	}

	// split(s) / join(s)
	if strings.HasPrefix(s, "split(") {
		return parseStringArgBuiltin(s[6:], opSplit)
	}
	if strings.HasPrefix(s, "join(") {
		return parseStringArgBuiltin(s[5:], opJoin)
	}
	if strings.HasPrefix(s, "tostream") && (len(s) == 8 || !isIdentChar(s[8])) {
		return &op{typ: opToStream}, s[8:], nil
	}
	if strings.HasPrefix(s, "truncate_stream(") {
		return parseGeneratorArgBuiltin(s[16:], opTruncateStream)
	}
	if strings.HasPrefix(s, "fromstream(") {
		return parseGeneratorArgBuiltin(s[11:], opFromStream)
	}
	if strings.HasPrefix(s, "strftime(") {
		return parseGeneratorArgBuiltin(s[9:], opStrftime)
	}
	if strings.HasPrefix(s, "strflocaltime(") {
		return parseGeneratorArgBuiltin(s[14:], opStrfLocaltime)
	}
	if strings.HasPrefix(s, "strptime(") {
		return parseGeneratorArgBuiltin(s[9:], opStrptime)
	}
	if strings.HasPrefix(s, "mktime") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opMktime}, s[6:], nil
	}
	if strings.HasPrefix(s, "gmtime") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opGmtime}, s[6:], nil
	}
	if strings.HasPrefix(s, "fromdate") && (len(s) == 8 || !isIdentChar(s[8])) {
		return &op{typ: opFromdate}, s[8:], nil
	}
	if strings.HasPrefix(s, "todate") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opTodate}, s[6:], nil
	}
	if strings.HasPrefix(s, "date") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opTodate}, s[4:], nil
	}
	if strings.HasPrefix(s, "now") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opNow}, s[3:], nil
	}

	// ascii_downcase / ascii_upcase
	if strings.HasPrefix(s, "ascii_downcase") && (len(s) == 14 || !isIdentChar(s[14])) {
		return &op{typ: opAsciiDowncase}, s[14:], nil
	}
	if strings.HasPrefix(s, "ascii_upcase") && (len(s) == 12 || !isIdentChar(s[12])) {
		return &op{typ: opAsciiUpcase}, s[12:], nil
	}

	// explode / implode
	if strings.HasPrefix(s, "explode") && (len(s) == 7 || !isIdentChar(s[7])) {
		return &op{typ: opExplode}, s[7:], nil
	}
	if strings.HasPrefix(s, "implode") && (len(s) == 7 || !isIdentChar(s[7])) {
		return &op{typ: opImplode}, s[7:], nil
	}
	if strings.HasPrefix(s, "utf8bytelength") && (len(s) == 14 || !isIdentChar(s[14])) {
		return &op{typ: opUTF8ByteLength}, s[14:], nil
	}
	if strings.HasPrefix(s, "reverse") && (len(s) == 7 || !isIdentChar(s[7])) {
		return &op{typ: opReverse}, s[7:], nil
	}
	if strings.HasPrefix(s, "combinations(") {
		return parseUnaryExprBuiltin(s[13:], opCombinations)
	}
	if strings.HasPrefix(s, "combinations") && (len(s) == 12 || !isIdentChar(s[12])) {
		return &op{typ: opCombinations}, s[12:], nil
	}

	// startswith(s) / endswith(s) / trim() / ltrim() / rtrim() / trimstr(s) / ltrimstr(s) / rtrimstr(s)
	if strings.HasPrefix(s, "startswith(") {
		return parseStringArgBuiltin(s[11:], opStartsWith)
	}
	if strings.HasPrefix(s, "endswith(") {
		return parseStringArgBuiltin(s[9:], opEndsWith)
	}
	if strings.HasPrefix(s, "trim") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opTrim}, s[4:], nil
	}
	if strings.HasPrefix(s, "ltrim") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opLtrim}, s[5:], nil
	}
	if strings.HasPrefix(s, "rtrim") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opRtrim}, s[5:], nil
	}
	if strings.HasPrefix(s, "trimstr(") {
		return parseStringExprArgBuiltin(s[8:], opTrimStr)
	}
	if strings.HasPrefix(s, "ltrimstr(") {
		return parseStringExprArgBuiltin(s[9:], opLtrimStr)
	}
	if strings.HasPrefix(s, "rtrimstr(") {
		return parseStringExprArgBuiltin(s[9:], opRtrimStr)
	}

	// to_entries / from_entries
	if strings.HasPrefix(s, "to_entries") && (len(s) == 10 || !isIdentChar(s[10])) {
		return &op{typ: opToEntries}, s[10:], nil
	}
	if strings.HasPrefix(s, "from_entries") && (len(s) == 12 || !isIdentChar(s[12])) {
		return &op{typ: opFromEntries}, s[12:], nil
	}
	if strings.HasPrefix(s, "with_entries(") {
		inner, rest, err := parseWithEntriesBody(s[13:])
		if err != nil {
			return nil, rest, fmt.Errorf("in with_entries(): %w", err)
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after with_entries() expression")
		}
		return applyPostfixPipe(withEntriesBuiltinNode(inner), rest[1:])
	}

	// map(expr) — desugars to [.[] | expr] at parse time
	if strings.HasPrefix(s, "map(") {
		inner, rest, err := parseGeneratorExpr(s[4:])
		if err != nil {
			return nil, rest, fmt.Errorf("in map(): %w", err)
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after map() expression")
		}
		return applyPostfixPipe(mapBuiltinNode(inner), rest[1:])
	}
	if strings.HasPrefix(s, "map_values(") {
		inner, rest, err := parseGeneratorExpr(s[11:])
		if err != nil {
			return nil, rest, fmt.Errorf("in map_values(): %w", err)
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after map_values() expression")
		}
		return applyPostfixPipe(mapValuesBuiltinNode(inner), rest[1:])
	}

	// tojson / fromjson / tostring / tonumber / toboolean
	if strings.HasPrefix(s, "tojson") && (len(s) == 6 || !isIdentChar(s[6])) {
		return applyPostfixPipe(&op{typ: opToJSON}, s[6:])
	}
	if strings.HasPrefix(s, "fromjson") && (len(s) == 8 || !isIdentChar(s[8])) {
		return applyPostfixPipe(&op{typ: opFromJSON}, s[8:])
	}
	if strings.HasPrefix(s, "tostring") && (len(s) == 8 || !isIdentChar(s[8])) {
		return applyPostfixPipe(&op{typ: opToString}, s[8:])
	}
	if strings.HasPrefix(s, "tonumber") && (len(s) == 8 || !isIdentChar(s[8])) {
		return applyPostfixPipe(&op{typ: opToNumber}, s[8:])
	}
	if strings.HasPrefix(s, "toboolean") && (len(s) == 9 || !isIdentChar(s[9])) {
		return applyPostfixPipe(&op{typ: opToBoolean}, s[9:])
	}

	// not builtin (with boundary check)
	if strings.HasPrefix(s, "not") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opNot}, s[3:], nil
	}

	// type builtin (with boundary check)
	if strings.HasPrefix(s, "type") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opTypeBuiltin}, s[4:], nil
	}

	// contains(val) — recursive containment check
	if strings.HasPrefix(s, "contains(") {
		inner, rest, err := parsePipeExpr(s[9:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after contains() argument")
		}
		return &op{typ: opContains, child: inner}, rest[1:], nil
	}

	// inside(val) — reverse of contains: a | inside(b) == b | contains(a)
	if strings.HasPrefix(s, "inside(") {
		inner, rest, err := parsePipeExpr(s[7:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after inside() argument")
		}
		return &op{typ: opContains, child: inner, optional: true}, rest[1:], nil
	}

	// floor / ceil / round — numeric rounding (no argument)
	if strings.HasPrefix(s, "floor") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opFloor}, s[5:], nil
	}
	if strings.HasPrefix(s, "ceil") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opCeil}, s[4:], nil
	}
	if strings.HasPrefix(s, "round") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opRound}, s[5:], nil
	}

	// error — throw input as an error (0-arg) or error(expr) throw expr (1-arg)
	if strings.HasPrefix(s, "error") && (len(s) == 5 || !isIdentChar(s[5])) {
		if len(s) > 5 && s[5] == '(' {
			inner, rest, err := parsePipeExpr(s[6:])
			if err != nil {
				return nil, rest, err
			}
			rest = strings.TrimSpace(rest)
			if len(rest) == 0 || rest[0] != ')' {
				return nil, rest, fmt.Errorf("expected ')' after error()")
			}
			return &op{typ: opError, child: inner}, rest[1:], nil
		}
		return &op{typ: opError}, s[5:], nil
	}

	// nan/infinite constants and predicates
	if strings.HasPrefix(s, "nan") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opLiteral, literal: []byte("NaN")}, s[3:], nil
	}
	if strings.HasPrefix(s, "infinite") && (len(s) == 8 || !isIdentChar(s[8])) {
		return &op{typ: opLiteral, literal: []byte("infinite")}, s[8:], nil
	}
	// isinfinite before isnan/isfinite/isnormal (longer prefix first)
	if strings.HasPrefix(s, "isinfinite") && (len(s) == 10 || !isIdentChar(s[10])) {
		return &op{typ: opIsInfinite}, s[10:], nil
	}
	if strings.HasPrefix(s, "isnan") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opIsNaN}, s[5:], nil
	}
	if strings.HasPrefix(s, "isfinite") && (len(s) == 8 || !isIdentChar(s[8])) {
		return &op{typ: opIsFinite}, s[8:], nil
	}
	if strings.HasPrefix(s, "isnormal") && (len(s) == 8 || !isIdentChar(s[8])) {
		return &op{typ: opIsNormal}, s[8:], nil
	}
	// pow(x; y) — 2-arg power function
	if strings.HasPrefix(s, "pow(") {
		xExpr, rest, err := parsePipeExpr(s[4:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in pow(x; y)")
		}
		yExpr, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after pow(x; y)")
		}
		return &op{typ: opPow, left: xExpr, right: yExpr}, rest[1:], nil
	}

	// isempty(expr) — true if expr produces no outputs
	if strings.HasPrefix(s, "isempty(") {
		inner, rest, err := parseGeneratorExpr(s[8:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after isempty()")
		}
		return &op{typ: opIsEmpty, child: inner}, rest[1:], nil
	}

	// nth(n; gen) — nth output of a generator (0-indexed)
	if strings.HasPrefix(s, "nth(") {
		nExpr, rest, err := parseGeneratorExpr(s[4:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && rest[0] == ')' {
			nths := make([]*op, 0, len(generatorElems(nExpr)))
			for _, countExpr := range generatorElems(nExpr) {
				idx, ok := literalIntValue(countExpr)
				if !ok {
					return nil, rest, fmt.Errorf("nth(n) requires integer arguments")
				}
				nths = append(nths, &op{typ: opIndex, index: idx})
			}
			return collapseGeneratorNodes(nths), rest[1:], nil
		}
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in nth(n; gen)")
		}
		rest = strings.TrimSpace(rest[1:])
		genExpr, rest, err := parseGeneratorExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after nth() arguments")
		}
		nths := make([]*op, 0, len(generatorElems(nExpr)))
		for _, countExpr := range generatorElems(nExpr) {
			nths = append(nths, &op{typ: opNth, left: countExpr, child: genExpr})
		}
		return collapseGeneratorNodes(nths), rest[1:], nil
	}

	if strings.HasPrefix(s, "hypot(") {
		xExpr, rest, err := parsePipeExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in hypot(x; y)")
		}
		yExpr, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after hypot(x; y)")
		}
		return &op{typ: opHypot, left: xExpr, right: yExpr}, rest[1:], nil
	}
	if strings.HasPrefix(s, "fma(") {
		xExpr, rest, err := parsePipeExpr(s[4:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in fma(x; y; z)")
		}
		yExpr, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected second ';' in fma(x; y; z)")
		}
		zExpr, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after fma(x; y; z)")
		}
		return &op{typ: opFMA, left: xExpr, right: yExpr, child: zExpr}, rest[1:], nil
	}

	// 1-arg floating-point math builtins (all take the input number, return a number).
	// Unsupported higher-arity forms remain limited to the still-deferred tail such as atan(y; x).
	if strings.HasPrefix(s, "sqrt") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathSqrt}, s[4:], nil
	}
	if strings.HasPrefix(s, "fabs") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathFabs}, s[4:], nil
	}
	if strings.HasPrefix(s, "atan") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathAtan}, s[4:], nil
	}
	if strings.HasPrefix(s, "lgamma") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opMathLgamma}, s[6:], nil
	}
	if strings.HasPrefix(s, "log2") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathLog2}, s[4:], nil
	}
	if strings.HasPrefix(s, "log10") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opMathLog10}, s[5:], nil
	}
	if strings.HasPrefix(s, "log") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMathLog}, s[3:], nil
	}
	if strings.HasPrefix(s, "exp10") && (len(s) == 5 || !isIdentChar(s[5])) {
		return &op{typ: opMathExp10}, s[5:], nil
	}
	if strings.HasPrefix(s, "exp2") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathExp2}, s[4:], nil
	}
	if strings.HasPrefix(s, "exp") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMathExp}, s[3:], nil
	}
	if strings.HasPrefix(s, "cbrt") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathCbrt}, s[4:], nil
	}
	if strings.HasPrefix(s, "logb") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathLogb}, s[4:], nil
	}
	if strings.HasPrefix(s, "nearbyint") && (len(s) == 9 || !isIdentChar(s[9])) {
		return &op{typ: opMathNearbyint}, s[9:], nil
	}
	if strings.HasPrefix(s, "j0") && (len(s) == 2 || !isIdentChar(s[2])) {
		return &op{typ: opMathJ0}, s[2:], nil
	}
	if strings.HasPrefix(s, "j1") && (len(s) == 2 || !isIdentChar(s[2])) {
		return &op{typ: opMathJ1}, s[2:], nil
	}
	if strings.HasPrefix(s, "asin") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathAsin}, s[4:], nil
	}
	if strings.HasPrefix(s, "acos") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opMathAcos}, s[4:], nil
	}
	if strings.HasPrefix(s, "sin") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMathSin}, s[3:], nil
	}
	if strings.HasPrefix(s, "cos") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMathCos}, s[3:], nil
	}
	if strings.HasPrefix(s, "tan") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opMathTan}, s[3:], nil
	}
	if strings.HasPrefix(s, "tgamma") && (len(s) == 6 || !isIdentChar(s[6])) {
		return &op{typ: opMathTgamma}, s[6:], nil
	}

	// range(n) / range(from;to) / range(from;to;step) — Tier 2: 1 alloc per value
	if strings.HasPrefix(s, "range(") {
		return parseRange(s[6:])
	}

	// Regex builtins — pattern compiled at parse time (Go RE2, linear-time matching).
	// test(re) is 0-alloc; match/capture alloc one []int on a hit; scan/sub/gsub alloc per match.
	if strings.HasPrefix(s, "test(") {
		return parseRegexBuiltin(s[5:], opTest)
	}
	if strings.HasPrefix(s, "match(") {
		return parseRegexBuiltin(s[6:], opMatchRe)
	}
	if strings.HasPrefix(s, "capture(") {
		return parseRegexBuiltin(s[8:], opCapture)
	}
	if strings.HasPrefix(s, "scan(") {
		return parseRegexBuiltin(s[5:], opScan)
	}
	if strings.HasPrefix(s, "sub(") {
		return parseRegexWithReplacement(s[4:], opSub)
	}
	if strings.HasPrefix(s, "gsub(") {
		return parseRegexWithReplacement(s[5:], opGSub)
	}

	// Dot expressions
	if s[0] == '.' {
		return parseDotExpr(s)
	}

	// Parenthesized expression: (expr)
	if s[0] == '(' {
		inner, rest, err := parseGeneratorExpr(s[1:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' to close grouped expression")
		}
		return applyPostfixPipe(inner, rest[1:])
	}

	// Object construction
	if s[0] == '{' {
		node, rest, err := parseConstruct(s)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}

	// Array construction
	if s[0] == '[' {
		node, rest, err := parseArrayConstruct(s)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}

	// Number literal: digit or '-' followed by digit.
	// Check nan/infinite FIRST because -nan and -infinite start with '-'.
	if s[0] == '-' && len(s) > 1 && !isDigit(s[1]) {
		if strings.HasPrefix(s, "-nan") && (len(s) == 4 || !isIdentChar(s[4])) {
			return &op{typ: opLiteral, literal: []byte("NaN")}, s[4:], nil
		}
		if strings.HasPrefix(s, "-infinite") && (len(s) == 9 || !isIdentChar(s[9])) {
			return &op{typ: opLiteral, literal: []byte("-infinite")}, s[9:], nil
		}
	}
	if isDigit(s[0]) || (s[0] == '-' && len(s) > 1 && isDigit(s[1])) {
		return parseNumberLiteral(s)
	}
	if isIdentStart(s[0]) {
		node, rest, err := parseUserCall(s)
		if err != nil {
			return nil, rest, err
		}
		return applyPostfixPipe(node, rest)
	}

	return nil, s, fmt.Errorf("unexpected character %q", s[0:1])
}

func parseDefExpr(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	name, rest := readIdentifier(s)
	if name == "" {
		return nil, rest, fmt.Errorf("expected function name after def")
	}

	var params []string
	var valueParams []bool
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && rest[0] == '(' {
		var err error
		params, valueParams, rest, err = parseDefParams(rest[1:])
		if err != nil {
			return nil, rest, err
		}
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ':' {
		return nil, rest, fmt.Errorf("expected ':' after def %s", name)
	}
	body, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' after def %s body", name)
	}
	child, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	return &op{
		typ: opDefScope,
		fn: &funcDef{
			name:        name,
			params:      params,
			valueParams: valueParams,
			body:        body,
		},
		child: child,
	}, rest, nil
}

func parseRecurse(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] == ')' {
		return &op{typ: opRecurse}, s[1:], nil
	}
	update, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 {
		return nil, rest, fmt.Errorf("expected ')' after recurse() arguments")
	}
	if rest[0] == ')' {
		return &op{typ: opRecurse, left: update}, rest[1:], nil
	}
	if rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' or ')' after recurse() update expression")
	}
	cond, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after recurse() condition")
	}
	return &op{typ: opRecurse, left: update, child: cond}, rest[1:], nil
}

func parseDefParams(s string) ([]string, []bool, string, error) {
	var params []string
	var valueParams []bool
	for {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			return nil, nil, s, fmt.Errorf("unclosed function parameter list")
		}
		if s[0] == ')' {
			return params, valueParams, s[1:], nil
		}
		isValue := false
		if s[0] == '$' {
			isValue = true
			s = s[1:]
		}
		name, rest := readIdentifier(s)
		if name == "" {
			return nil, nil, rest, fmt.Errorf("expected parameter name in function definition")
		}
		params = append(params, name)
		valueParams = append(valueParams, isValue)
		s = strings.TrimSpace(rest)
		if len(s) == 0 {
			return nil, nil, s, fmt.Errorf("unclosed function parameter list")
		}
		if s[0] == ';' {
			s = s[1:]
			continue
		}
		if s[0] == ')' {
			return params, valueParams, s[1:], nil
		}
		return nil, nil, s, fmt.Errorf("expected ';' or ')' in function parameter list")
	}
}

func parseUserCall(s string) (*op, string, error) {
	name, rest := readIdentifier(s)
	if name == "" {
		return nil, s, fmt.Errorf("expected identifier")
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '(' {
		return &op{typ: opCall, name: name}, rest, nil
	}
	rest = rest[1:]
	var args []*op
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 {
			return nil, rest, fmt.Errorf("unclosed call to %s()", name)
		}
		if rest[0] == ')' {
			return &op{typ: opCall, name: name, elems: args}, rest[1:], nil
		}
		arg, next, err := parseGeneratorExpr(rest)
		if err != nil {
			return nil, next, err
		}
		args = append(args, arg)
		rest = strings.TrimSpace(next)
		if len(rest) == 0 {
			return nil, rest, fmt.Errorf("unclosed call to %s()", name)
		}
		if rest[0] == ';' {
			rest = rest[1:]
			continue
		}
		if rest[0] == ')' {
			return &op{typ: opCall, name: name, elems: args}, rest[1:], nil
		}
		return nil, rest, fmt.Errorf("expected ';' or ')' after %s() argument", name)
	}
}

func parseVarRef(s string) (*op, string, error) {
	name, rest := readIdentifier(s[1:])
	if name == "" {
		return nil, s, fmt.Errorf("expected variable name after '$'")
	}
	node := &op{typ: opVar, name: name}
	if len(rest) > 0 && rest[0] == '[' {
		child, remaining, err := parseBracketExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		node.child = child
		rest = remaining
	} else if len(rest) > 0 && rest[0] == '.' {
		if len(rest) > 1 && (isIdentStart(rest[1]) || rest[1] == '"') {
			child, remaining, err := parseFieldChain(rest[1:])
			if err != nil {
				return nil, rest, err
			}
			node.child = child
			rest = remaining
		} else if len(rest) > 1 && rest[1] == '[' {
			child, remaining, err := parseBracketExpr(rest[1:])
			if err != nil {
				return nil, rest, err
			}
			node.child = child
			rest = remaining
		} else if len(rest) > 1 && rest[1] == '?' {
			node.child = &op{typ: opIdentity}
			rest = rest[2:]
		}
	} else if len(rest) > 0 && rest[0] == '?' {
		node.optional = true
		rest = rest[1:]
	}
	return node, rest, nil
}

func validateVars(node *op, scope map[string]bool) error {
	if node == nil {
		return nil
	}
	switch node.typ {
	case opVar:
		if node.name != "__loc__" && (scope == nil || !scope[node.name]) {
			return fmt.Errorf("$%s is not defined", node.name)
		}
		return validateVars(node.child, scope)
	case opBind:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		next := cloneVarScope(scope)
		addBindingPatternVars(next, node.name, node.pattern)
		for _, alt := range node.altPatterns {
			addBindingPatternVars(next, "", alt)
		}
		return validateVars(node.right, next)
	case opReduce:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		if err := validateVars(node.right, scope); err != nil {
			return err
		}
		next := cloneVarScope(scope)
		addBindingPatternVars(next, node.name, node.pattern)
		return validateVars(node.child, next)
	case opForeach:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		if err := validateVars(node.right, scope); err != nil {
			return err
		}
		next := cloneVarScope(scope)
		addBindingPatternVars(next, node.name, node.pattern)
		if err := validateVars(node.child, next); err != nil {
			return err
		}
		return validateVars(node.extra, next)
	case opDefScope:
		next := cloneVarScope(scope)
		for i, name := range node.fn.params {
			if node.fn.valueParams[i] {
				next[name] = true
			}
		}
		if err := validateVars(node.fn.body, next); err != nil {
			return err
		}
		return validateVars(node.child, scope)
	case opPipe, opCompare, opAlternative, opAnd, opOr, opPlus, opMinus, opMul, opDiv, opMod, opPow:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		return validateVars(node.right, scope)
	case opField, opIndex:
		return validateVars(node.child, scope)
	case opConstruct:
		for _, p := range node.pairs {
			if err := validateVars(p.keyExpr, scope); err != nil {
				return err
			}
			if err := validateVars(p.expr, scope); err != nil {
				return err
			}
		}
		return nil
	case opArrayConstruct, opGenerator, opStringInterp, opFormatTemplate, opCall, opPick:
		for _, elem := range node.elems {
			if err := validateVars(elem, scope); err != nil {
				return err
			}
		}
		return nil
	case opSelect, opFlatten, opContains, opIsEmpty, opNth, opSortBy, opUniqueBy, opGroupBy, opTest, opMatchRe, opCapture, opScan, opSub, opGSub, opPaths, opWalk:
		if err := validateVars(node.child, scope); err != nil {
			return err
		}
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		return validateVars(node.right, scope)
	case opIf:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		if err := validateVars(node.right, scope); err != nil {
			return err
		}
		return validateVars(node.child, scope)
	case opTry:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		return validateVars(node.right, scope)
	default:
		if err := validateVars(node.left, scope); err != nil {
			return err
		}
		if err := validateVars(node.right, scope); err != nil {
			return err
		}
		return validateVars(node.child, scope)
	}
}

func validateLabels(node *op, scope map[string]bool) error {
	if node == nil {
		return nil
	}
	switch node.typ {
	case opLabel:
		next := cloneVarScope(scope)
		if next == nil {
			next = make(map[string]bool)
		}
		next[node.name] = true
		return validateLabels(node.child, next)
	case opBreakOp:
		if scope == nil || !scope[node.name] {
			return fmt.Errorf("$*label-%s is not defined", node.name)
		}
		return nil
	case opBind, opReduce:
		if err := validateLabels(node.left, scope); err != nil {
			return err
		}
		if err := validateLabels(node.right, scope); err != nil {
			return err
		}
		return validateLabels(node.child, scope)
	case opForeach:
		if err := validateLabels(node.left, scope); err != nil {
			return err
		}
		if err := validateLabels(node.right, scope); err != nil {
			return err
		}
		if err := validateLabels(node.child, scope); err != nil {
			return err
		}
		return validateLabels(node.extra, scope)
	case opDefScope:
		if err := validateLabels(node.fn.body, scope); err != nil {
			return err
		}
		return validateLabels(node.child, scope)
	case opConstruct:
		for _, p := range node.pairs {
			if err := validateLabels(p.expr, scope); err != nil {
				return err
			}
		}
		return nil
	case opArrayConstruct, opGenerator, opStringInterp, opFormatTemplate, opCall, opPick:
		for _, elem := range node.elems {
			if err := validateLabels(elem, scope); err != nil {
				return err
			}
		}
		return nil
	default:
		if err := validateLabels(node.left, scope); err != nil {
			return err
		}
		if err := validateLabels(node.right, scope); err != nil {
			return err
		}
		return validateLabels(node.child, scope)
	}
}

func validateFuncs(node *op, scope map[string]bool, vars map[string]bool) error {
	if node == nil {
		return nil
	}
	switch node.typ {
	case opCall:
		key := funcKey(node.name, len(node.elems))
		if scope == nil || !scope[key] {
			return fmt.Errorf("%s is not defined", key)
		}
		for _, arg := range node.elems {
			if err := validateFuncs(arg, scope, vars); err != nil {
				return err
			}
		}
		return nil
	case opDefScope:
		nextScope := cloneVarScope(scope)
		key := funcKey(node.fn.name, len(node.fn.params))
		nextScope[key] = true
		nextVars := cloneVarScope(vars)
		for i, name := range node.fn.params {
			if !node.fn.valueParams[i] {
				nextScope[funcKey(name, 0)] = true
				continue
			}
			nextVars[name] = true
		}
		if err := validateFuncs(node.fn.body, nextScope, nextVars); err != nil {
			return err
		}
		return validateFuncs(node.child, nextScope, vars)
	case opBind:
		if err := validateFuncs(node.left, scope, vars); err != nil {
			return err
		}
		return validateFuncs(node.right, scope, vars)
	case opReduce:
		if err := validateFuncs(node.left, scope, vars); err != nil {
			return err
		}
		if err := validateFuncs(node.right, scope, vars); err != nil {
			return err
		}
		return validateFuncs(node.child, scope, vars)
	case opForeach:
		if err := validateFuncs(node.left, scope, vars); err != nil {
			return err
		}
		if err := validateFuncs(node.right, scope, vars); err != nil {
			return err
		}
		if err := validateFuncs(node.child, scope, vars); err != nil {
			return err
		}
		return validateFuncs(node.extra, scope, vars)
	case opConstruct:
		for _, p := range node.pairs {
			if err := validateFuncs(p.expr, scope, vars); err != nil {
				return err
			}
		}
		return nil
	case opArrayConstruct, opGenerator, opStringInterp, opFormatTemplate, opPick:
		for _, elem := range node.elems {
			if err := validateFuncs(elem, scope, vars); err != nil {
				return err
			}
		}
		return nil
	default:
		if err := validateFuncs(node.left, scope, vars); err != nil {
			return err
		}
		if err := validateFuncs(node.right, scope, vars); err != nil {
			return err
		}
		return validateFuncs(node.child, scope, vars)
	}
}

func cloneVarScope(scope map[string]bool) map[string]bool {
	if scope == nil {
		return make(map[string]bool)
	}
	out := make(map[string]bool, len(scope)+1)
	for k, v := range scope {
		out[k] = v
	}
	return out
}

func addBindingPatternVars(scope map[string]bool, name string, pattern *bindPattern) {
	if pattern != nil {
		addPatternVars(scope, pattern)
		return
	}
	if name != "" {
		scope[name] = true
	}
}

func addPatternVars(scope map[string]bool, pattern *bindPattern) {
	if pattern == nil {
		return
	}
	switch pattern.kind {
	case bindPatternVar:
		scope[pattern.name] = true
	case bindPatternArray:
		for _, elem := range pattern.elems {
			addPatternVars(scope, elem)
		}
	case bindPatternObject:
		for _, field := range pattern.fields {
			if field.bindName != "" {
				scope[field.bindName] = true
			}
			addPatternVars(scope, field.pattern)
		}
	}
}

func parseBindingTarget(s string) (string, *bindPattern, string, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil, s, false, nil
	}
	switch s[0] {
	case '$':
		name, rest := readIdentifier(s[1:])
		if name == "" {
			return "", nil, rest, false, nil
		}
		return name, nil, rest, true, nil
	case '[':
		pat, rest, err := parseArrayBindingPattern(s)
		return "", pat, rest, true, err
	case '{':
		pat, rest, err := parseObjectBindingPattern(s)
		return "", pat, rest, true, err
	default:
		return "", nil, s, false, nil
	}
}

func parseArrayBindingPattern(s string) (*bindPattern, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '[' {
		return nil, s, fmt.Errorf("expected '['")
	}
	s = s[1:]
	var elems []*bindPattern
	for {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed array binding pattern")
		}
		if s[0] == ']' {
			if len(elems) == 0 {
				return nil, s, fmt.Errorf("expected binding pattern inside []")
			}
			return &bindPattern{kind: bindPatternArray, elems: elems}, s[1:], nil
		}
		elem, rest, err := parseNestedBindingPattern(s)
		if err != nil {
			return nil, rest, err
		}
		elems = append(elems, elem)
		s = strings.TrimSpace(rest)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed array binding pattern")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == ']' {
			continue
		}
		return nil, s, fmt.Errorf("expected ',' in array binding pattern")
	}
}

func parseObjectBindingPattern(s string) (*bindPattern, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '{' {
		return nil, s, fmt.Errorf("expected '{'")
	}
	s = s[1:]
	var fields []bindPatternField
	for {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed object binding pattern")
		}
		if s[0] == '}' {
			if len(fields) == 0 {
				return nil, s, fmt.Errorf("expected binding pattern inside {}")
			}
			return &bindPattern{kind: bindPatternObject, fields: fields}, s[1:], nil
		}
		field, rest, err := parseObjectBindingField(s)
		if err != nil {
			return nil, rest, err
		}
		fields = append(fields, field)
		s = strings.TrimSpace(rest)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed object binding pattern")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == '}' {
			continue
		}
		return nil, s, fmt.Errorf("expected ',' in object binding pattern")
	}
}

func parseNestedBindingPattern(s string) (*bindPattern, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return nil, s, fmt.Errorf("expected binding pattern")
	}
	switch s[0] {
	case '$':
		name, rest := readIdentifier(s[1:])
		if name == "" {
			return nil, rest, fmt.Errorf("expected variable name after $")
		}
		return &bindPattern{kind: bindPatternVar, name: name}, rest, nil
	case '[':
		return parseArrayBindingPattern(s)
	case '{':
		return parseObjectBindingPattern(s)
	default:
		return nil, s, fmt.Errorf("expected binding pattern")
	}
}

func parseObjectBindingField(s string) (bindPatternField, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return bindPatternField{}, s, fmt.Errorf("expected object binding field")
	}
	if s[0] == '$' {
		name, rest := readIdentifier(s[1:])
		if name == "" {
			return bindPatternField{}, rest, fmt.Errorf("expected variable name after $")
		}
		field := bindPatternField{key: name, bindName: name}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ':' {
			return field, rest, nil
		}
		pat, rest, err := parseNestedBindingPattern(rest[1:])
		if err != nil {
			return bindPatternField{}, rest, err
		}
		field.pattern = pat
		return field, rest, nil
	}
	if s[0] == '"' {
		key, rest, err := parseStringKey(s)
		if err != nil {
			return bindPatternField{}, rest, err
		}
		return parseExplicitObjectBindingField(key, rest)
	}
	if s[0] == '(' {
		expr, rest, err := parseGeneratorExpr(s[1:])
		if err != nil {
			return bindPatternField{}, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return bindPatternField{}, rest, fmt.Errorf("expected ')' in object binding field")
		}
		key, err := staticObjectBindingKey(expr)
		if err != nil {
			return bindPatternField{}, rest, err
		}
		return parseExplicitObjectBindingField(key, rest[1:])
	}
	key, rest := readIdentifier(s)
	if key == "" {
		return bindPatternField{}, rest, fmt.Errorf("expected field name in object binding pattern")
	}
	return parseExplicitObjectBindingField(key, rest)
}

func parseExplicitObjectBindingField(key, rest string) (bindPatternField, string, error) {
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ':' {
		return bindPatternField{}, rest, fmt.Errorf("expected ':' in object binding field")
	}
	pat, next, err := parseNestedBindingPattern(rest[1:])
	if err != nil {
		return bindPatternField{}, next, err
	}
	return bindPatternField{key: key, pattern: pat}, next, nil
}

func parseStringKey(s string) (string, string, error) {
	lit, rest, err := parseStringLiteral(s)
	if err != nil {
		return "", rest, err
	}
	key, ok, err := decodedStringLiteralKey(lit)
	if err != nil {
		return "", rest, err
	}
	if !ok {
		return "", rest, fmt.Errorf("string interpolation in object key not supported")
	}
	return key, rest, nil
}

func decodedStringLiteralKey(node *op) (string, bool, error) {
	if node == nil {
		return "", false, fmt.Errorf("expected string literal")
	}
	if node.typ != opLiteral {
		return "", false, nil
	}
	if len(node.literal) == 0 || node.literal[0] != '"' {
		return "", false, fmt.Errorf("expected string literal")
	}
	var out []byte
	ss := scanner{data: node.literal}
	raw := ss.readString()
	out = appendCanonicalRawJSONStringContent(out[:0], raw)
	return string(out), true, nil
}

func shorthandConstructPairForStringKey(node *op) (*pair, error) {
	key, ok, err := decodedStringLiteralKey(node)
	if err != nil {
		return nil, err
	}
	if ok {
		return &pair{key: key, expr: &op{typ: opField, field: key}}, nil
	}
	return &pair{
		keyExpr: node,
		expr:    &op{typ: opIndexExpr, left: node},
	}, nil
}

func staticObjectBindingKey(node *op) (string, error) {
	if node == nil {
		return "", fmt.Errorf("expected object key expression")
	}
	switch node.typ {
	case opLiteral:
		if len(node.literal) == 0 {
			return "", fmt.Errorf("expected object key expression")
		}
		switch node.literal[0] {
		case '"':
			var out []byte
			ss := scanner{data: node.literal}
			raw := ss.readString()
			out = appendCanonicalRawJSONStringContent(out[:0], raw)
			return string(out), nil
		case 't', 'f', 'n':
			return "", fmt.Errorf("Cannot use %s (%s) as object key", jsonTypeName(node.literal), string(node.literal))
		default:
			return "", fmt.Errorf("Cannot use %s (%s) as object key", jsonTypeName(node.literal), string(node.literal))
		}
	case opPlus:
		left, err := staticObjectBindingKey(node.left)
		if err != nil {
			return "", err
		}
		right, err := staticObjectBindingKey(node.right)
		if err != nil {
			return "", err
		}
		return left + right, nil
	default:
		return "", fmt.Errorf("Cannot use non-constant expression as object key")
	}
}

func parseReduce(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	gen, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !(strings.HasPrefix(rest, "as") && (len(rest) == 2 || !isIdentChar(rest[2]))) {
		return nil, rest, fmt.Errorf("expected 'as $name' in reduce")
	}
	rest = strings.TrimSpace(rest[2:])
	name, pattern, rest, ok, err := parseBindingTarget(rest)
	if err != nil {
		return nil, rest, err
	}
	if !ok {
		return nil, rest, fmt.Errorf("expected binding pattern after as in reduce")
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '(' {
		return nil, rest, fmt.Errorf("expected '(' after reduce as $%s", name)
	}
	initExpr, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' in reduce")
	}
	updateExpr, rest, err := parseGeneratorExpr(strings.TrimSpace(rest[1:]))
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after reduce")
	}
	return &op{typ: opReduce, name: name, pattern: pattern, left: gen, right: initExpr, child: updateExpr}, rest[1:], nil
}

func parseForeach(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	gen, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !(strings.HasPrefix(rest, "as") && (len(rest) == 2 || !isIdentChar(rest[2]))) {
		return nil, rest, fmt.Errorf("expected 'as $name' in foreach")
	}
	rest = strings.TrimSpace(rest[2:])
	name, pattern, rest, ok, err := parseBindingTarget(rest)
	if err != nil {
		return nil, rest, err
	}
	if !ok {
		return nil, rest, fmt.Errorf("expected binding pattern after as in foreach")
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '(' {
		return nil, rest, fmt.Errorf("expected '(' after foreach as $%s", name)
	}
	initExpr, rest, err := parseGeneratorExpr(rest[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' in foreach")
	}
	updateExpr, rest, err := parseGeneratorExpr(strings.TrimSpace(rest[1:]))
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	extractExpr := &op{typ: opIdentity}
	if len(rest) > 0 && rest[0] == ';' {
		extractExpr, rest, err = parseGeneratorExpr(strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after foreach")
	}
	return &op{typ: opForeach, name: name, pattern: pattern, left: gen, right: initExpr, child: updateExpr, extra: extractExpr}, rest[1:], nil
}

// parseDotExpr parses identity (.) or field access (.foo, .foo.bar),
// array index (.[0], .[-1]), or iterator (.[]).
func parseDotExpr(s string) (*op, string, error) {
	s = s[1:] // skip '.'

	// Recursive descent: ..
	if len(s) > 0 && s[0] == '.' {
		return applyPostfixPipe(&op{typ: opRecursiveDescent}, s[1:])
	}

	// Check for .[...] — array index or iterator
	if len(s) > 0 && s[0] == '[' {
		return parseBracketExpr(s)
	}
	if len(s) > 0 && s[0] == '?' {
		return &op{typ: opOptional, child: &op{typ: opIdentity}}, s[1:], nil
	}

	// Identity: just "." followed by end, whitespace, pipe, comma, paren, or @format
	if s == "" || s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r' || s[0] == '|' || s[0] == ',' || s[0] == ';' || s[0] == ')' || s[0] == '}' || s[0] == ']' || s[0] == '=' || s[0] == '!' || s[0] == '<' || s[0] == '>' || s[0] == '/' || s[0] == '+' || s[0] == '-' || s[0] == '*' || s[0] == '%' {
		return &op{typ: opIdentity}, s, nil
	}

	return parseFieldChain(s)
}

// parseFieldChain parses "foo.bar.baz" or "foo[0]" into a chained opField.
func parseFieldChain(s string) (*op, string, error) {
	var (
		name string
		rest string
	)
	if len(s) > 0 && s[0] == '"' {
		raw, remaining, err := parseJSONStringLiteral(s)
		if err != nil {
			return nil, remaining, err
		}
		name = raw
		rest = remaining
	} else {
		name, rest = readIdentifier(s)
		if name == "" {
			return nil, s, fmt.Errorf("expected field name after '.'")
		}
	}

	node := &op{typ: opField, field: name}

	// Check for optional marker
	if len(rest) > 0 && rest[0] == '?' {
		node.optional = true
		rest = rest[1:]
	}

	// Check for chained bracket: .foo[0] or .foo[]
	if len(rest) > 0 && rest[0] == '[' {
		child, remaining, err := parseBracketExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		node.child = child
		rest = remaining
		return node, rest, nil
	}

	// Check for chained field: .foo.bar
	if len(rest) > 0 && rest[0] == '.' {
		// Peek ahead — .foo.bar and .foo.[] are both valid chains.
		if len(rest) > 1 && (isIdentStart(rest[1]) || rest[1] == '"' || rest[1] == '[') {
			if rest[1] == '[' {
				child, remaining, err := parseBracketExpr(rest[1:])
				if err != nil {
					return nil, rest, err
				}
				node.child = child
				rest = remaining
				return node, rest, nil
			}
			child, remaining, err := parseFieldChain(rest[1:])
			if err != nil {
				return nil, rest, err
			}
			node.child = child
			rest = remaining
		}
	}

	return node, rest, nil
}

// parseAnyAll parses any/all with an optional (expr) argument.
// s is the text after "any"/"all" has been consumed.
// Supports both one-arg any(expr) and two-arg any(gen; cond) forms.
func parseAnyAll(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '(' {
		return &op{typ: typ}, s, nil // no-arg form
	}
	inner, rest, err := parseGeneratorExpr(s[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && rest[0] == ';' {
		// Two-arg form: any(gen; cond)
		rest = strings.TrimSpace(rest[1:])
		cond, rest2, err := parsePipeExpr(rest)
		if err != nil {
			return nil, rest2, err
		}
		rest2 = strings.TrimSpace(rest2)
		if len(rest2) == 0 || rest2[0] != ')' {
			return nil, rest2, fmt.Errorf("expected ')' after any/all arguments")
		}
		// Use left=generator, child=condition; right=nil distinguishes from one-arg
		return &op{typ: typ, left: inner, child: cond}, rest2[1:], nil
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after any/all argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

func parseJSONStringLiteral(s string) (string, string, error) {
	if len(s) == 0 || s[0] != '"' {
		return "", s, fmt.Errorf("expected string literal")
	}
	i := 1
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == '"' {
			return s[1:i], s[i+1:], nil
		}
		i++
	}
	return "", s, fmt.Errorf("unterminated string literal")
}

func generatorElems(node *op) []*op {
	if node == nil {
		return nil
	}
	if node.typ == opGenerator {
		return node.elems
	}
	return []*op{node}
}

func collapseGeneratorNodes(nodes []*op) *op {
	switch len(nodes) {
	case 0:
		return &op{typ: opEmpty}
	case 1:
		return nodes[0]
	default:
		return &op{typ: opGenerator, elems: nodes}
	}
}

// parseUnaryGenBuiltin parses a builtin of the form name(gen_expr).
// Unlike parseUnaryExprBuiltin, the argument is parsed as a generator
// expression (may contain commas producing multiple outputs), e.g. sort_by(.a, .b).
// s starts after the opening '(' has been consumed.
func parseUnaryGenBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	inner, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

// parseUnaryExprBuiltin parses a builtin of the form name(expr).
// s starts after the opening '(' has been consumed.
func parseUnaryExprBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	inner, rest, err := parsePipeExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

func parseGeneratorArgBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	inner, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

func parseStringExprArgBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] == '"' {
		return parseStringArgBuiltin(s, typ)
	}
	inner, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

// parseStringArgBuiltin parses builtins of the form name("literal_string").
// s should start just after the opening '(' has been consumed.
// The unquoted string content is stored in op.field.
func parseStringArgBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	nodes := make([]*op, 0, 1)
	for {
		if len(s) == 0 || s[0] != '"' {
			return nil, s, fmt.Errorf("expected string argument")
		}
		i := 1
		for i < len(s) {
			if s[i] == '\\' {
				i += 2
				continue
			}
			if s[i] == '"' {
				break
			}
			i++
		}
		if i >= len(s) {
			return nil, s, fmt.Errorf("unterminated string argument")
		}
		nodes = append(nodes, &op{typ: typ, field: s[1:i]})
		s = strings.TrimSpace(s[i+1:])
		if len(s) == 0 {
			return nil, s, fmt.Errorf("expected ')' after string argument")
		}
		if s[0] == ')' {
			return collapseGeneratorNodes(nodes), s[1:], nil
		}
		if s[0] != ',' {
			return nil, s, fmt.Errorf("expected ')' after string argument")
		}
		s = strings.TrimSpace(s[1:])
	}
}

func parseCommaArgList(s string) ([]*op, string, error) {
	var args []*op
	for {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed argument list")
		}
		if s[0] == ')' {
			return args, s[1:], nil
		}
		arg, rest, err := parsePipeExpr(s)
		if err != nil {
			return nil, rest, err
		}
		args = append(args, arg)
		s = strings.TrimSpace(rest)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("unclosed argument list")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == ')' {
			return args, s[1:], nil
		}
		return nil, s, fmt.Errorf("expected ',' or ')' in argument list")
	}
}

// parseHas parses has("key") or has(n) — object key / array index membership.
func parseHas(s string) (*op, string, error) {
	s = strings.TrimSpace(s[4:]) // skip "has("

	// has(nan) — NaN is never a valid index/key; always returns false
	if strings.HasPrefix(s, "nan") && (len(s) == 3 || !isIdentChar(s[3])) {
		rest := strings.TrimSpace(s[3:])
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after has(nan)")
		}
		return &op{typ: opLiteral, literal: []byte("false")}, rest[1:], nil
	}

	// has(n) — integer argument for array index check
	if len(s) > 0 && (isDigit(s[0]) || (s[0] == '-' && len(s) > 1 && isDigit(s[1]))) {
		neg := s[0] == '-'
		if neg {
			s = s[1:]
		}
		idx, rest, err := parseInt(s)
		if err != nil {
			return nil, s, fmt.Errorf("has() integer argument: %w", err)
		}
		if neg {
			idx = -idx
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after has() argument")
		}
		return &op{typ: opHas, index: idx, literal: []byte("array")}, rest[1:], nil
	}

	if len(s) == 0 || s[0] != '"' {
		return nil, s, fmt.Errorf("has() requires a string field name or integer index")
	}
	// Scan to closing quote, handling escapes
	i := 1
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == '"' {
			break
		}
		i++
	}
	if i >= len(s) {
		return nil, s, fmt.Errorf("unterminated string in has()")
	}
	key := s[1:i]
	rest := strings.TrimSpace(s[i+1:])
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after has() argument")
	}
	return &op{typ: opHas, field: key}, rest[1:], nil
}

// parseIf parses if COND then EXPR else EXPR end.
// The else branch is optional; if omitted it defaults to identity (.).
func parseIf(s string) (*op, string, error) {
	s = strings.TrimSpace(s[2:]) // skip "if"

	cond, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "then") || (len(rest) > 4 && isIdentChar(rest[4])) {
		return nil, rest, fmt.Errorf("expected 'then' in if expression")
	}
	rest = strings.TrimSpace(rest[4:])

	thenBranch, rest, err := parseGeneratorExpr(rest)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	var elseBranch *op
	elifConsumedEnd := false
	if strings.HasPrefix(rest, "elif") && (len(rest) == 4 || !isIdentChar(rest[4])) {
		// Desugar: elif C then ... end  →  else (if C then ... end)
		elifNode, remaining, elifErr := parseIf("if" + rest[4:])
		if elifErr != nil {
			return nil, remaining, elifErr
		}
		elseBranch = elifNode
		rest = remaining
		elifConsumedEnd = true
	} else if strings.HasPrefix(rest, "else") && (len(rest) == 4 || !isIdentChar(rest[4])) {
		rest = strings.TrimSpace(rest[4:])
		elseBranch, rest, err = parseGeneratorExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
	}

	if !elifConsumedEnd {
		if !strings.HasPrefix(rest, "end") || (len(rest) > 3 && isIdentChar(rest[3])) {
			return nil, rest, fmt.Errorf("expected 'end' to close if expression")
		}
		rest = rest[3:]
	}

	// left=cond, right=thenBranch, child=elseBranch (nil → identity)
	return &op{typ: opIf, left: cond, right: thenBranch, child: elseBranch}, rest, nil
}

// parseDel parses del(.foo), del(.foo, .bar), del(.foo.bar), del(.[0]).
func parseDel(s string) (*op, string, error) {
	s = s[4:] // skip "del("

	var fields []op
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, "", fmt.Errorf("unclosed del()")
		}
		if s[0] == ')' {
			break
		}
		if len(fields) > 0 {
			if s[0] != ',' {
				return nil, s, fmt.Errorf("expected ',' in del() argument list")
			}
			s = strings.TrimSpace(s[1:])
		}
		expr, rest, err := parsePipeExpr(s)
		if err != nil {
			return nil, rest, fmt.Errorf("in del(): %w", err)
		}
		if expr.typ == opGenerator {
			fields = append(fields, flattenDeleteFields(expr.elems)...)
		} else {
			fields = append(fields, *expr)
		}
		s = rest
	}

	s = s[1:] // skip ')'

	if len(fields) == 0 {
		return nil, s, fmt.Errorf("del() requires at least one argument")
	}

	return &op{typ: opDelete, fields: fields}, s, nil
}

func flattenDeleteFields(nodes []*op) []op {
	fields := make([]op, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.typ == opGenerator {
			fields = append(fields, flattenDeleteFields(node.elems)...)
			continue
		}
		fields = append(fields, *node)
	}
	return fields
}

// parseSelect parses select(cond).
func parseSelect(s string) (*op, string, error) {
	s = s[7:] // skip "select("

	cond, rest, err := parsePipeExpr(s)
	if err != nil {
		return nil, rest, fmt.Errorf("in select(): %w", err)
	}

	rest = strings.TrimSpace(rest)
	if rest == "" || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after select condition")
	}
	rest = rest[1:] // skip ')'

	return &op{typ: opSelect, child: cond}, rest, nil
}

// parseOptionalSliceBound parses an optional slice bound expression from s,
// returning (expr, rest) where expr is nil if the bound is omitted.
func parseOptionalSliceBound(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] == ']' {
		return nil, s, nil
	}
	return parseExpr(s)
}

func parseBracketSuffix(rest string) (optional bool, child *op, remaining string, err error) {
	if len(rest) > 0 && rest[0] == '?' {
		optional = true
		rest = rest[1:]
	}
	if len(rest) > 0 && rest[0] == '.' {
		if len(rest) > 1 && isIdentStart(rest[1]) {
			child, remaining, err = parseFieldChain(rest[1:])
			if err != nil {
				return false, nil, rest, err
			}
			rest = remaining
		} else if len(rest) > 1 && rest[1] == '[' {
			child, remaining, err = parseBracketExpr(rest[1:])
			if err != nil {
				return false, nil, rest, err
			}
			rest = remaining
		}
	} else if len(rest) > 0 && rest[0] == '[' {
		child, remaining, err = parseBracketExpr(rest)
		if err != nil {
			return false, nil, rest, err
		}
		rest = remaining
	}
	return optional, child, rest, nil
}

func finalizeBracketNode(node *op, rest string) (*op, string, error) {
	optional, child, remaining, err := parseBracketSuffix(rest)
	if err != nil {
		return nil, remaining, err
	}
	if optional {
		node.optional = true
	}
	node.child = child
	return node, remaining, nil
}

func applyPostfixPipe(node *op, rest string) (*op, string, error) {
	for {
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && rest[0] == '?' {
			node = &op{typ: opOptional, child: node}
			rest = rest[1:]
			continue
		}
		if len(rest) > 0 && rest[0] == '[' {
			suffix, remaining, err := parseBracketExpr(rest)
			if err != nil {
				return nil, remaining, err
			}
			node = &op{typ: opApply, left: node, right: suffix}
			rest = remaining
			continue
		}
		if len(rest) > 1 && rest[0] == '.' && isIdentStart(rest[1]) {
			suffix, remaining, err := parseFieldChain(rest[1:])
			if err != nil {
				return nil, remaining, err
			}
			node = &op{typ: opApply, left: node, right: suffix}
			rest = remaining
			continue
		}
		return node, rest, nil
	}
}

func finalizeBracketMulti(nodes []*op, rest string) (*op, string, error) {
	optional, child, remaining, err := parseBracketSuffix(rest)
	if err != nil {
		return nil, remaining, err
	}
	for _, node := range nodes {
		if optional {
			node.optional = true
		}
	}
	gen := &op{typ: opGenerator, elems: nodes}
	if child == nil {
		return gen, remaining, nil
	}
	return &op{typ: opPipe, left: gen, right: child}, remaining, nil
}

func literalIntValue(node *op) (int, bool) {
	if node == nil || node.typ != opLiteral {
		return 0, false
	}
	s := string(node.literal)
	if s == "" {
		return 0, false
	}
	if strings.ContainsAny(s, ".eE") {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func literalIndexValue(node *op) (int, bool) {
	if idx, ok := literalIntValue(node); ok {
		return idx, true
	}
	if node == nil || node.typ != opLiteral {
		return 0, false
	}
	f, ok := parseJSONFloat(node.literal)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// parseSliceEnd parses the optional end bound and closing ']' of a slice.
// s starts after the ':'. Returns the opSlice node.
func parseSliceEnd(s string, startExpr *op) (*op, string, error) {
	s = strings.TrimSpace(s)
	endExpr, rest, err := parseOptionalSliceBound(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ']' {
		return nil, rest, fmt.Errorf("expected ']' after slice")
	}
	return finalizeBracketNode(&op{typ: opSlice, left: startExpr, right: endExpr}, rest[1:])
}

// parseBracketExpr parses [N] (index), [] (iterator), or [N:M] (slice).
// Assumes s starts with '['.
func parseBracketExpr(s string) (*op, string, error) {
	s = s[1:] // skip '['
	s = strings.TrimSpace(s)

	// .[] — iterator
	if len(s) > 0 && s[0] == ']' {
		return finalizeBracketNode(&op{typ: opIterator}, s[1:])
	}

	// .[:M] — slice with no start
	if len(s) > 0 && s[0] == ':' {
		return parseSliceEnd(s[1:], nil)
	}

	// .[expr] or .[expr:M] or .[expr:] — index or slice with start
	startExpr, rest, err := parseExpr(s)
	if err != nil {
		return nil, s, err
	}
	rest = strings.TrimSpace(rest)

	// .[expr:M] or .[expr:] — slice
	if len(rest) > 0 && rest[0] == ':' {
		return parseSliceEnd(rest[1:], startExpr)
	}

	if len(rest) > 0 && rest[0] == ',' {
		nodes := make([]*op, 0, 2)
		idx, ok := literalIndexValue(startExpr)
		if !ok {
			return nil, rest, fmt.Errorf("expected integer array index")
		}
		nodes = append(nodes, &op{typ: opIndex, index: idx})
		for len(rest) > 0 && rest[0] == ',' {
			nextExpr, nextRest, err := parseExpr(strings.TrimSpace(rest[1:]))
			if err != nil {
				return nil, nextRest, err
			}
			nextIdx, ok := literalIndexValue(nextExpr)
			if !ok {
				return nil, nextRest, fmt.Errorf("expected integer array index")
			}
			nodes = append(nodes, &op{typ: opIndex, index: nextIdx})
			rest = strings.TrimSpace(nextRest)
		}
		if len(rest) == 0 || rest[0] != ']' {
			return nil, rest, fmt.Errorf("expected ']' after array index")
		}
		return finalizeBracketMulti(nodes, rest[1:])
	}

	if len(rest) == 0 || rest[0] != ']' {
		return nil, rest, fmt.Errorf("expected ']' after array index")
	}
	rest = rest[1:] // skip ']'

	idx, ok := literalIntValue(startExpr)
	if ok {
		return finalizeBracketNode(&op{typ: opIndex, index: idx}, rest)
	}
	return finalizeBracketNode(&op{typ: opIndexExpr, left: startExpr}, rest)
}

// parseInt parses a non-negative integer from the start of s.
func parseInt(s string) (int, string, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, s, fmt.Errorf("expected digit")
	}
	n := 0
	for _, ch := range s[:i] {
		n = n*10 + int(ch-'0')
	}
	return n, s[i:], nil
}

// hasMultiOutput returns true if the op may produce more than one output value.
// Used at compile time to choose the right execution path for object construction.
// Conservative: returns true when uncertain.
func hasMultiOutput(n *op) bool {
	if n == nil {
		return false
	}
	switch n.typ {
	case opIterator, opRecursiveDescent, opRecurse, opWalk, opRange, opScan, opGenerator, opPaths, opReduce, opForeach, opWhile, opRepeat, opPath, opCall, opAssign, opUpdate, opUpdateAlt, opUpdateMath, opBsearch, opToStream, opTruncateStream:
		return true

	// Ops that cap or reduce to at most one output regardless of their children:
	case opFirst, opLast, opLimit, opNth, opIsEmpty, opAny, opAll, opSelect,
		opArrayConstruct: // collects all elements into a single array
		return false

	case opConstruct:
		// Multi-output if any pair value is multi-output (Cartesian product).
		for _, p := range n.pairs {
			if hasMultiOutput(p.keyExpr) || hasMultiOutput(p.expr) {
				return true
			}
		}
		return false

	case opIf:
		// Condition (left) drives which branch runs, not the output count.
		// Then (right) or else (child) produce the actual outputs.
		return hasMultiOutput(n.right) || hasMultiOutput(n.child)
	case opLabel:
		return hasMultiOutput(n.child)
	case opDefScope:
		return hasMultiOutput(n.child)
	}

	// For all remaining ops: propagate through all children.
	// This handles:
	//   opField with child iterator (.field[])  → child = opIterator → true
	//   opPipe, opAlternative, opTry            → left/right/child propagation
	//   literals, arithmetic, comparison        → no children → false
	return hasMultiOutput(n.left) || hasMultiOutput(n.right) || hasMultiOutput(n.child)
}

// parseConstruct parses object construction: {name}, {name, age}, {a: .foo, b: .bar}.
// Assumes s starts with '{'.
func parseConstruct(s string) (*op, string, error) {
	s = s[1:] // skip '{'
	var pairs []pair

	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, "", fmt.Errorf("unclosed object construction")
		}
		if s[0] == '}' {
			s = s[1:]
			break
		}
		if len(pairs) > 0 {
			if s[0] != ',' {
				return nil, s, fmt.Errorf("expected ',' in object construction")
			}
			s = strings.TrimSpace(s[1:])
		}

		var (
			current pair
			err     error
		)
		switch s[0] {
		case '"':
			var keyNode *op
			keyNode, s, err = parseStringLiteral(s)
			if err != nil {
				return nil, s, err
			}
			s = strings.TrimSpace(s)
			if len(s) > 0 && s[0] == ':' {
				var ok bool
				if current.key, ok, err = decodedStringLiteralKey(keyNode); err != nil {
					return nil, s, err
				}
				if !ok {
					current.keyExpr = keyNode
				}
				s = strings.TrimSpace(s[1:])
				current.expr, s, err = parsePipeExpr(s)
				if err != nil {
					return nil, s, fmt.Errorf("in object construction: %w", err)
				}
			} else {
				p, err := shorthandConstructPairForStringKey(keyNode)
				if err != nil {
					return nil, s, err
				}
				current = *p
			}
		case '$':
			name, rest := readIdentifier(s[1:])
			if name == "" {
				return nil, s, fmt.Errorf("expected variable name after $")
			}
			s = strings.TrimSpace(rest)
			if len(s) > 0 && s[0] == ':' {
				current.keyExpr = &op{typ: opVar, name: name}
				s = strings.TrimSpace(s[1:])
				current.expr, s, err = parsePipeExpr(s)
				if err != nil {
					return nil, s, fmt.Errorf("in object construction: %w", err)
				}
			} else {
				current.key = name
				current.expr = &op{typ: opVar, name: name}
			}
		case '(':
			current.keyExpr, s, err = parseGeneratorExpr(s[1:])
			if err != nil {
				return nil, s, err
			}
			s = strings.TrimSpace(s)
			if len(s) == 0 || s[0] != ')' {
				return nil, s, fmt.Errorf("expected ')' to close grouped expression")
			}
			s = strings.TrimSpace(s[1:])
			if len(s) == 0 || s[0] != ':' {
				return nil, s, fmt.Errorf("expected ':' in object construction")
			}
			s = strings.TrimSpace(s[1:])
			current.expr, s, err = parsePipeExpr(s)
			if err != nil {
				return nil, s, fmt.Errorf("in object construction: %w", err)
			}
		default:
			key, rest := readIdentifier(s)
			if key == "" {
				return nil, s, fmt.Errorf("expected field name in object construction")
			}
			s = strings.TrimSpace(rest)
			if len(s) > 0 && s[0] == ':' {
				current.key = key
				s = strings.TrimSpace(s[1:])
				current.expr, s, err = parsePipeExpr(s)
				if err != nil {
					return nil, s, fmt.Errorf("in object construction: %w", err)
				}
			} else {
				current.key = key
				current.expr = &op{typ: opField, field: key}
			}
		}
		pairs = append(pairs, current)
	}

	// Detect at compile time whether any pair value may produce multiple outputs.
	// This drives the execMulti dispatch: single-output pairs use execConstruct (fast path),
	// multi-output pairs use execConstructMulti (Cartesian product path).
	multiVal := false
	for _, p := range pairs {
		if hasMultiOutput(p.keyExpr) || hasMultiOutput(p.expr) {
			multiVal = true
			break
		}
	}
	return &op{typ: opConstruct, pairs: pairs, multiValuePairs: multiVal}, s, nil
}

// parseArrayConstruct parses array construction: [.foo, .bar].
// Array bodies are generator contexts, so commas bind tighter than pipes.
// Assumes s starts with '['.
func parseArrayConstruct(s string) (*op, string, error) {
	s = s[1:] // skip '['
	var elems []*op

	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, "", fmt.Errorf("unclosed array construction")
		}
		if s[0] == ']' {
			s = s[1:]
			break
		}
		if len(elems) > 0 {
			if s[0] != ',' {
				return nil, s, fmt.Errorf("expected ',' in array construction")
			}
			s = strings.TrimSpace(s[1:])
		}

		expr, remaining, err := parseGeneratorExpr(s)
		if err != nil {
			return nil, remaining, fmt.Errorf("in array construction: %w", err)
		}
		elems = append(elems, expr)
		s = remaining
	}

	return &op{typ: opArrayConstruct, elems: elems}, s, nil
}

// parseStringLiteral parses a JSON string literal including quotes.
// Assumes s starts with '"'.
// If the string contains \(expr) interpolation sequences, it returns
// opStringInterp; otherwise opLiteral.
func parseStringLiteral(s string) (*op, string, error) {
	var segs [][]byte // literal segments
	var exprs []*op   // interpolated expressions

	i := 1 // skip opening '"'
	segStart := i

	for i < len(s) {
		ch := s[i]
		if ch == '\\' {
			if i+1 < len(s) && s[i+1] == '(' {
				// String interpolation \(expr) found.
				// Save literal segment up to here (not including \().
				segs = append(segs, []byte(s[segStart:i]))

				// Parse the inner expression — scan for balanced ')'.
				j := i + 2 // skip \(
				depth := 1
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '(':
						depth++
					case ')':
						depth--
					case '"':
						// Skip inner string to avoid false paren matching.
						j++
						for j < len(s) && s[j] != '"' {
							if s[j] == '\\' {
								j++
							}
							j++
						}
					}
					if depth > 0 {
						j++
					}
				}
				if depth != 0 {
					return nil, s, fmt.Errorf("unterminated \\( in string interpolation")
				}

				exprStr := s[i+2 : j]
				expr, _, err := parsePipeExpr(exprStr)
				if err != nil {
					return nil, s, fmt.Errorf("in string interpolation \\(...): %w", err)
				}
				exprs = append(exprs, expr)
				i = j + 1 // skip past closing )
				segStart = i
				continue
			}
			i += 2 // regular escape: \n, \t, \uXXXX etc.
			continue
		}
		if ch == '"' {
			// End of string.
			if len(exprs) == 0 {
				// No interpolation: return plain literal.
				i++ // include closing '"'
				return &op{typ: opLiteral, literal: []byte(s[:i])}, s[i:], nil
			}
			// Has interpolation: save final segment and return opStringInterp.
			segs = append(segs, []byte(s[segStart:i]))
			return &op{typ: opStringInterp, elems: exprs, segs: segs}, s[i+1:], nil
		}
		i++
	}
	return nil, s, fmt.Errorf("unterminated string literal")
}

func parseFormatBuiltin(rest string, format opType) (*op, string, error) {
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '"' {
		return &op{typ: format}, rest, nil
	}
	node, tail, err := parseStringLiteral(rest)
	if err != nil {
		return nil, tail, err
	}
	if node.typ != opStringInterp {
		return node, tail, nil
	}
	node.typ = opFormatTemplate
	node.format = format
	return node, tail, nil
}

func mapBuiltinNode(inner *op) *op {
	iter := &op{typ: opIterator}
	pipe := &op{typ: opPipe, left: iter, right: inner}
	return &op{typ: opArrayConstruct, elems: []*op{pipe}}
}

func addBuiltinNode(inner *op) *op {
	return &op{
		typ:   opPipe,
		left:  &op{typ: opArrayConstruct, elems: []*op{inner}},
		right: &op{typ: opAdd},
	}
}

func withEntriesBuiltinNode(inner *op) *op {
	return &op{
		typ:  opPipe,
		left: &op{typ: opToEntries},
		right: &op{
			typ:   opPipe,
			left:  mapBuiltinNode(inner),
			right: &op{typ: opFromEntries},
		},
	}
}

func parseWithEntriesBody(s string) (*op, string, error) {
	if field, rhs, rest, ok, err := parseSimpleFieldUpdate(s); ok || err != nil {
		if err != nil {
			return nil, rest, err
		}
		if field != "key" && field != "value" {
			return nil, rest, fmt.Errorf("with_entries() only supports .key or .value updates")
		}
		return withEntriesFieldUpdateInnerNode(field, rhs), rest, nil
	}
	return parseGeneratorExpr(s)
}

func parseSimpleFieldUpdate(s string) (field string, rhs *op, rest string, ok bool, err error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '.' {
		return "", nil, s, false, nil
	}
	field, rest = readIdentifier(s[1:])
	if field == "" {
		return "", nil, rest, false, nil
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "|=") {
		return "", nil, rest, false, nil
	}
	rhs, rest, err = parseGeneratorExpr(strings.TrimSpace(rest[2:]))
	if err != nil {
		return "", nil, rest, false, err
	}
	return field, rhs, rest, true, nil
}

func withEntriesFieldUpdateInnerNode(field string, rhs *op) *op {
	entryKey := &op{typ: opField, field: "key"}
	entryValue := &op{typ: opField, field: "value"}
	keyExpr := entryKey
	valueExpr := entryValue
	switch field {
	case "key":
		keyExpr = &op{typ: opPipe, left: &op{typ: opField, field: "key"}, right: rhs}
	case "value":
		valueExpr = &op{typ: opPipe, left: &op{typ: opField, field: "value"}, right: rhs}
	}
	return &op{
		typ: opConstruct,
		pairs: []pair{
			{key: "key", expr: keyExpr},
			{key: "value", expr: valueExpr},
		},
	}
}

func mapValuesBuiltinNode(inner *op) *op {
	const entryVar = "_fastjq_map_values_entry"

	entryKey := &op{typ: opVar, name: entryVar, child: &op{typ: opField, field: "key"}}
	entryValue := &op{typ: opVar, name: entryVar, child: &op{typ: opField, field: "value"}}
	valueResult := &op{typ: opPipe, left: entryValue, right: inner}
	entryObject := &op{
		typ: opConstruct,
		pairs: []pair{
			{key: "key", expr: entryKey},
			{key: "value", expr: &op{typ: opIdentity}},
		},
	}
	objectMapInner := &op{
		typ:   opBind,
		name:  entryVar,
		left:  &op{typ: opIdentity},
		right: &op{typ: opPipe, left: valueResult, right: entryObject},
	}
	objectBranch := withEntriesBuiltinNode(objectMapInner)
	cond := &op{
		typ:  opCompare,
		left: &op{typ: opTypeBuiltin},
		right: &op{
			typ:     opLiteral,
			literal: []byte(`"object"`),
		},
		cmpOp: cmpEq,
	}
	return &op{typ: opIf, left: cond, right: objectBranch, child: mapBuiltinNode(inner)}
}

// parseNumberLiteral parses a JSON number literal.
// Handles integers, decimals, and scientific notation.
func parseNumberLiteral(s string) (*op, string, error) {
	i := 0
	if s[i] == '-' {
		i++
	}
	// digits
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	// optional decimal
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	// optional exponent
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	return applyPostfixPipe(&op{typ: opLiteral, literal: []byte(s[:i])}, s[i:])
}

// readIdentifier reads a field name (letters, digits, underscore, hyphen).
func readIdentifier(s string) (string, string) {
	i := 0
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// parseTry parses: try expr [catch handler]
// s is the text after "try" has been consumed.
// The body is parsed with parseOr (NOT parseAlt/parseExpr), so:
//
//	try .a | .b         →  (try .a) | .b   (pipe handled above)
//	try .a // "default" →  (try .a) // "default"   (// handled above)
func parseTry(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	body, rest, err := parseOr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	var handler *op
	if strings.HasPrefix(rest, "catch") && (len(rest) == 5 || !isIdentChar(rest[5])) {
		rest = strings.TrimSpace(rest[5:])
		handler, rest, err = parseMulExpr(rest)
		if err != nil {
			return nil, rest, err
		}
	}
	return &op{typ: opTry, left: body, right: handler}, rest, nil
}

// parseRegexBuiltin parses test(re), match(re), capture(re), scan(re) —
// all optionally accept a flags string as a second ';'-separated argument.
// Supported flags: i (case-insensitive), m (multiline ^/$), s (dot-matches-newline).
// The regexp is compiled once at Compile() time; Run() never allocates for the engine.
// s starts after the opening '(' has been consumed.
func parseRegexBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '"' {
		return nil, s, fmt.Errorf("expected string pattern in regex builtin")
	}
	i := 1
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == '"' {
			break
		}
		i++
	}
	if i >= len(s) {
		return nil, s, fmt.Errorf("unterminated pattern string in regex builtin")
	}
	pattern := s[1:i]
	rest := strings.TrimSpace(s[i+1:])

	// Optional '; "flags"' argument
	if len(rest) > 0 && rest[0] == ';' {
		rest = strings.TrimSpace(rest[1:])
		if len(rest) == 0 || rest[0] != '"' {
			return nil, rest, fmt.Errorf("regex flags must be a string")
		}
		j := 1
		for j < len(rest) && rest[j] != '"' {
			if rest[j] == '\\' {
				j++
			}
			j++
		}
		if j >= len(rest) {
			return nil, rest, fmt.Errorf("unterminated flags string in regex builtin")
		}
		flags := rest[1:j]
		rest = strings.TrimSpace(rest[j+1:])
		// Map jq flags to RE2 inline flags: i, m, s supported; g and x ignored.
		var goFlags strings.Builder
		for _, f := range flags {
			switch f {
			case 'i', 'm', 's':
				goFlags.WriteRune(f)
			}
		}
		if goFlags.Len() > 0 {
			pattern = "(?" + goFlags.String() + ")" + pattern
		}
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after regex builtin arguments")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, rest[1:], fmt.Errorf("invalid regexp %q: %w", pattern, err)
	}
	return &op{typ: typ, re: re}, rest[1:], nil
}

// parseRegexWithReplacement parses sub(re; "literal") and gsub(re; "literal").
// The pattern supports the same optional flags as parseRegexBuiltin.
// The replacement is a literal string stored in node.field.
// s starts after the opening '(' has been consumed.
func parseRegexWithReplacement(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '"' {
		return nil, s, fmt.Errorf("expected pattern string in %v", typ)
	}
	i := 1
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == '"' {
			break
		}
		i++
	}
	if i >= len(s) {
		return nil, s, fmt.Errorf("unterminated pattern string in %v", typ)
	}
	pattern := s[1:i]
	rest := strings.TrimSpace(s[i+1:])

	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' between pattern and replacement in %v", typ)
	}
	rest = strings.TrimSpace(rest[1:])
	if len(rest) == 0 || rest[0] != '"' {
		return nil, rest, fmt.Errorf("expected replacement string in %v", typ)
	}
	j := 1
	for j < len(rest) {
		if rest[j] == '\\' {
			j += 2
			continue
		}
		if rest[j] == '"' {
			break
		}
		j++
	}
	if j >= len(rest) {
		return nil, rest, fmt.Errorf("unterminated replacement string in %v", typ)
	}
	replacement := rest[1:j]
	rest = strings.TrimSpace(rest[j+1:])

	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after %v arguments", typ)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, rest[1:], fmt.Errorf("invalid regexp %q: %w", pattern, err)
	}
	return &op{typ: typ, re: re, field: replacement}, rest[1:], nil
}

// parseRange parses range(n), range(from;to), range(from;to;step).
// s starts after the opening '(' has been consumed.
// Desugaring: range(n) → range(0; n; 1), range(from;to) → range(from; to; 1).
// Stored as opRange{left:from, right:to, child:step-or-nil}.
func parseRange(s string) (*op, string, error) {
	s = strings.TrimSpace(s)
	arg1, rest, err := parseGeneratorExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	if len(rest) > 0 && rest[0] == ')' {
		// range(n): from=0, to=n, step=1
		zero := &op{typ: opLiteral, literal: []byte("0")}
		nodes := make([]*op, 0, len(generatorElems(arg1)))
		for _, toExpr := range generatorElems(arg1) {
			nodes = append(nodes, &op{typ: opRange, left: zero, right: toExpr})
		}
		return collapseGeneratorNodes(nodes), rest[1:], nil
	}
	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' or ')' in range()")
	}
	rest = strings.TrimSpace(rest[1:])
	arg2, rest, err := parseGeneratorExpr(rest)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	if len(rest) > 0 && rest[0] == ')' {
		// range(from; to): step=1
		nodes := make([]*op, 0, len(generatorElems(arg1))*len(generatorElems(arg2)))
		for _, fromExpr := range generatorElems(arg1) {
			for _, toExpr := range generatorElems(arg2) {
				nodes = append(nodes, &op{typ: opRange, left: fromExpr, right: toExpr})
			}
		}
		return collapseGeneratorNodes(nodes), rest[1:], nil
	}
	if len(rest) == 0 || rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' or ')' in range()")
	}
	rest = strings.TrimSpace(rest[1:])
	arg3, rest, err := parseGeneratorExpr(rest)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after range() arguments")
	}
	// range(from; to; step)
	nodes := make([]*op, 0, len(generatorElems(arg1))*len(generatorElems(arg2))*len(generatorElems(arg3)))
	for _, fromExpr := range generatorElems(arg1) {
		for _, toExpr := range generatorElems(arg2) {
			for _, stepExpr := range generatorElems(arg3) {
				nodes = append(nodes, &op{typ: opRange, left: fromExpr, right: toExpr, child: stepExpr})
			}
		}
	}
	return collapseGeneratorNodes(nodes), rest[1:], nil
}

// simplify optimizes the AST. Currently: removes identity from pipes.
// `. | expr` → expr, `expr | .` → expr.
func simplify(node *op) *op {
	if node == nil {
		return nil
	}
	if node.typ == opPipe {
		node.left = simplify(node.left)
		node.right = simplify(node.right)
		if node.left.typ == opIdentity {
			return node.right
		}
		if node.right.typ == opIdentity {
			return node.left
		}
	}
	return node
}
