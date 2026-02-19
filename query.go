package fastjq

import (
	"fmt"
	"strings"
)

// opType represents the type of operation in the AST.
type opType int

const (
	opIdentity       opType = iota // .
	opField                        // .foo
	opDelete                       // del(.foo)
	opPipe                         // expr | expr
	opIndex                        // .[0], .[-1]
	opIterator                     // .[]
	opConstruct                    // {name, a: .foo}
	opArrayConstruct               // [.foo, .bar]
	opLiteral                      // null, true, false, "string", 123
	opCompare                      // ==, !=, <, <=, >, >=
	opSelect                       // select(cond)
	opAlternative                  // expr // expr
	opTypeBuiltin                  // type builtin
	opAnd                          // expr and expr
	opOr                           // expr or expr
	opNot                          // not
	opEmpty                        // empty — produce zero outputs
	opHas                          // has("key")
	opIf                           // if cond then expr else expr end
	opLength                       // length
	opToEntries                    // to_entries
	opFromEntries                  // from_entries
	opWithEntries                  // with_entries(f) — dedicated single-pass executor
	opAdd                          // add
	opRange                        // range(n) / range(from;to) / range(from;to;step)
	opFlatten                      // flatten / flatten(n)
	opSplit                        // split("s")
	opJoin                         // join("s")
	opAsciiDowncase                // ascii_downcase
	opAsciiUpcase                  // ascii_upcase
	opStartsWith                   // startswith("s")
	opEndsWith                     // endswith("s")
	opLtrimStr                     // ltrimstr("s")
	opRtrimStr                     // rtrimstr("s")
	opKeysUnsorted                 // keys_unsorted
	opAny                          // any / any(expr)
	opAll                          // all / all(expr)
	opFirst                        // first(expr)
	opLast                         // last(expr)
	opLimit                        // limit(n; expr)
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

// pair represents a key-expression pair in object construction.
type pair struct {
	key  string
	expr *op
}

// op is a node in the query AST.
type op struct {
	typ      opType
	field    string  // for opField
	fields   []op    // for opDelete: list of field-access/index paths to delete
	left     *op     // for opPipe, opCompare, opAlternative
	right    *op     // for opPipe, opCompare, opAlternative
	child    *op     // for opField chaining, opSelect condition
	index    int     // for opIndex: array index (negative = from end)
	pairs    []pair  // for opConstruct: {key: expr} pairs
	elems    []*op   // for opArrayConstruct: expressions
	literal  []byte       // for opLiteral: raw JSON bytes
	cmpOp    cmpOperator  // for opCompare: comparison operator
	optional bool         // for opField/opIndex/opIterator: suppress errors
}

// parse compiles a jq query string into an AST.
func parse(query string) (*op, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	result, rest, err := parsePipeExpr(query)
	if err != nil {
		return nil, err
	}
	rest = strings.TrimSpace(rest)

	if rest != "" {
		return nil, fmt.Errorf("unexpected trailing input: %q", rest)
	}

	// Optimization: simplify identity pipes
	result = simplify(result)

	return result, nil
}

// parsePipeExpr parses a pipe chain: expr | expr | ...
func parsePipeExpr(s string) (*op, string, error) {
	result, rest, err := parseExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	for strings.HasPrefix(rest, "|") {
		rest = strings.TrimSpace(rest[1:])
		right, remainder, err := parseExpr(rest)
		if err != nil {
			return nil, remainder, err
		}
		result = &op{typ: opPipe, left: result, right: right}
		rest = strings.TrimSpace(remainder)
	}

	return result, rest, nil
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

// parseCmp parses comparison expressions: ==, !=, <, <=, >, >=
// Non-associative (no chaining). Delegates down to parseAtom.
func parseCmp(s string) (*op, string, error) {
	left, rest, err := parseAtom(s)
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

	// del()
	if strings.HasPrefix(s, "del(") {
		return parseDel(s)
	}

	// select()
	if strings.HasPrefix(s, "select(") {
		return parseSelect(s)
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

	// if-then-else
	if strings.HasPrefix(s, "if") && (len(s) == 2 || !isIdentChar(s[2])) {
		return parseIf(s)
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

	// first / last — no-arg desugar to .[0] / .[-1]; with arg use dedicated op
	if strings.HasPrefix(s, "first") && (len(s) == 5 || !isIdentChar(s[5])) {
		rest := strings.TrimSpace(s[5:])
		if len(rest) > 0 && rest[0] == '(' {
			inner, rest2, err := parsePipeExpr(rest[1:])
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
			inner, rest2, err := parsePipeExpr(rest[1:])
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

	// limit(n; expr)
	if strings.HasPrefix(s, "limit(") {
		nExpr, rest, err := parsePipeExpr(s[6:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ';' {
			return nil, rest, fmt.Errorf("expected ';' in limit(n; expr)")
		}
		rest = strings.TrimSpace(rest[1:])
		genExpr, rest, err := parsePipeExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after limit() arguments")
		}
		return &op{typ: opLimit, left: nExpr, child: genExpr}, rest[1:], nil
	}

	// keys_unsorted
	if strings.HasPrefix(s, "keys_unsorted") && (len(s) == 13 || !isIdentChar(s[13])) {
		return &op{typ: opKeysUnsorted}, s[13:], nil
	}

	// any / all — with optional (expr) argument
	if strings.HasPrefix(s, "any") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseAnyAll(s[3:], opAny)
	}
	if strings.HasPrefix(s, "all") && (len(s) == 3 || !isIdentChar(s[3])) {
		return parseAnyAll(s[3:], opAll)
	}

	// add
	if strings.HasPrefix(s, "add") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opAdd}, s[3:], nil
	}

	// range(n) / range(from; to) / range(from; to; step)
	if strings.HasPrefix(s, "range(") {
		return parseRange(s[6:])
	}

	// flatten / flatten(n)
	if strings.HasPrefix(s, "flatten") && (len(s) == 7 || !isIdentChar(s[7])) {
		rest := strings.TrimSpace(s[7:])
		if len(rest) > 0 && rest[0] == '(' {
			depthExpr, rest2, err := parsePipeExpr(rest[1:])
			if err != nil {
				return nil, rest2, err
			}
			rest2 = strings.TrimSpace(rest2)
			if len(rest2) == 0 || rest2[0] != ')' {
				return nil, rest2, fmt.Errorf("expected ')' after flatten() argument")
			}
			return &op{typ: opFlatten, child: depthExpr}, rest2[1:], nil
		}
		return &op{typ: opFlatten, index: -1}, rest, nil // -1 = unlimited depth
	}

	// split(s) / join(s)
	if strings.HasPrefix(s, "split(") {
		return parseStringArgBuiltin(s[6:], opSplit)
	}
	if strings.HasPrefix(s, "join(") {
		return parseStringArgBuiltin(s[5:], opJoin)
	}

	// ascii_downcase / ascii_upcase
	if strings.HasPrefix(s, "ascii_downcase") && (len(s) == 14 || !isIdentChar(s[14])) {
		return &op{typ: opAsciiDowncase}, s[14:], nil
	}
	if strings.HasPrefix(s, "ascii_upcase") && (len(s) == 12 || !isIdentChar(s[12])) {
		return &op{typ: opAsciiUpcase}, s[12:], nil
	}

	// startswith(s) / endswith(s) / ltrimstr(s) / rtrimstr(s)
	if strings.HasPrefix(s, "startswith(") {
		return parseStringArgBuiltin(s[11:], opStartsWith)
	}
	if strings.HasPrefix(s, "endswith(") {
		return parseStringArgBuiltin(s[9:], opEndsWith)
	}
	if strings.HasPrefix(s, "ltrimstr(") {
		return parseStringArgBuiltin(s[9:], opLtrimStr)
	}
	if strings.HasPrefix(s, "rtrimstr(") {
		return parseStringArgBuiltin(s[9:], opRtrimStr)
	}

	// to_entries / from_entries / with_entries
	if strings.HasPrefix(s, "to_entries") && (len(s) == 10 || !isIdentChar(s[10])) {
		return &op{typ: opToEntries}, s[10:], nil
	}
	if strings.HasPrefix(s, "from_entries") && (len(s) == 12 || !isIdentChar(s[12])) {
		return &op{typ: opFromEntries}, s[12:], nil
	}
	if strings.HasPrefix(s, "with_entries(") {
		inner, rest, err := parsePipeExpr(s[13:])
		if err != nil {
			return nil, rest, fmt.Errorf("in with_entries(): %w", err)
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after with_entries() expression")
		}
		rest = rest[1:]
		return &op{typ: opWithEntries, child: inner}, rest, nil
	}

	// map(expr) — desugars to [.[] | expr] at parse time
	if strings.HasPrefix(s, "map(") {
		inner, rest, err := parsePipeExpr(s[4:])
		if err != nil {
			return nil, rest, fmt.Errorf("in map(): %w", err)
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' after map() expression")
		}
		rest = rest[1:]
		// Desugar: [.[] | inner]
		iter := &op{typ: opIterator}
		pipe := &op{typ: opPipe, left: iter, right: inner}
		return &op{typ: opArrayConstruct, elems: []*op{pipe}}, rest, nil
	}

	// not builtin (with boundary check)
	if strings.HasPrefix(s, "not") && (len(s) == 3 || !isIdentChar(s[3])) {
		return &op{typ: opNot}, s[3:], nil
	}

	// type builtin (with boundary check)
	if strings.HasPrefix(s, "type") && (len(s) == 4 || !isIdentChar(s[4])) {
		return &op{typ: opTypeBuiltin}, s[4:], nil
	}

	// Dot expressions
	if s[0] == '.' {
		return parseDotExpr(s)
	}

	// Parenthesized expression: (expr)
	if s[0] == '(' {
		inner, rest, err := parsePipeExpr(s[1:])
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
		if len(rest) == 0 || rest[0] != ')' {
			return nil, rest, fmt.Errorf("expected ')' to close grouped expression")
		}
		return inner, rest[1:], nil
	}

	// Object construction
	if s[0] == '{' {
		return parseConstruct(s)
	}

	// Array construction
	if s[0] == '[' {
		return parseArrayConstruct(s)
	}

	// Number literal: digit or '-' followed by digit
	if isDigit(s[0]) || (s[0] == '-' && len(s) > 1 && isDigit(s[1])) {
		return parseNumberLiteral(s)
	}

	return nil, s, fmt.Errorf("unexpected character %q", s[0:1])
}

// parseDotExpr parses identity (.) or field access (.foo, .foo.bar),
// array index (.[0], .[-1]), or iterator (.[]).
func parseDotExpr(s string) (*op, string, error) {
	s = s[1:] // skip '.'

	// Check for .[...] — array index or iterator
	if len(s) > 0 && s[0] == '[' {
		return parseBracketExpr(s)
	}

	// Identity: just "." followed by end, whitespace, pipe, comma, or paren
	if s == "" || s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r' || s[0] == '|' || s[0] == ',' || s[0] == ')' || s[0] == '}' || s[0] == ']' || s[0] == '=' || s[0] == '!' || s[0] == '/' {
		return &op{typ: opIdentity}, s, nil
	}

	return parseFieldChain(s)
}

// parseFieldChain parses "foo.bar.baz" or "foo[0]" into a chained opField.
func parseFieldChain(s string) (*op, string, error) {
	name, rest := readIdentifier(s)
	if name == "" {
		return nil, s, fmt.Errorf("expected field name after '.'")
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
		// Peek ahead — if next char after '.' is a letter, it's a chain
		if len(rest) > 1 && isIdentStart(rest[1]) {
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

// parseRange parses range(n), range(from; to), or range(from; to; step).
// s starts after the opening '(' has been consumed.
func parseRange(s string) (*op, string, error) {
	arg1, rest, err := parsePipeExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	if len(rest) == 0 || rest[0] == ')' {
		// range(n) — from=0, to=n, step=1 (stored as left=to only; executor special-cases)
		if len(rest) > 0 {
			rest = rest[1:]
		}
		return &op{typ: opRange, left: arg1}, rest, nil
	}
	if rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' or ')' in range()")
	}
	arg2, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	if len(rest) == 0 || rest[0] == ')' {
		// range(from; to)
		if len(rest) > 0 {
			rest = rest[1:]
		}
		return &op{typ: opRange, left: arg1, right: arg2}, rest, nil
	}
	if rest[0] != ';' {
		return nil, rest, fmt.Errorf("expected ';' or ')' in range()")
	}
	arg3, rest, err := parsePipeExpr(strings.TrimSpace(rest[1:]))
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after range() arguments")
	}
	// range(from; to; step)
	return &op{typ: opRange, left: arg1, right: arg2, child: arg3}, rest[1:], nil
}

// parseAnyAll parses any/all with an optional (expr) argument.
// s is the text after "any"/"all" has been consumed.
// any(g; cond) two-arg form is not supported and returns an error.
func parseAnyAll(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '(' {
		return &op{typ: typ}, s, nil // no-arg form
	}
	inner, rest, err := parsePipeExpr(s[1:])
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && rest[0] == ';' {
		return nil, rest, fmt.Errorf("any/all(generator; cond) two-arg form not yet supported; use any(expr) or any")
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after any/all argument")
	}
	return &op{typ: typ, child: inner}, rest[1:], nil
}

// parseStringArgBuiltin parses builtins of the form name("literal_string").
// s should start just after the opening '(' has been consumed.
// The unquoted string content is stored in op.field.
func parseStringArgBuiltin(s string, typ opType) (*op, string, error) {
	s = strings.TrimSpace(s)
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
	key := s[1:i]
	rest := strings.TrimSpace(s[i+1:])
	if len(rest) == 0 || rest[0] != ')' {
		return nil, rest, fmt.Errorf("expected ')' after string argument")
	}
	return &op{typ: typ, field: key}, rest[1:], nil
}

// parseHas parses has("key") — checks whether an object contains a field.
func parseHas(s string) (*op, string, error) {
	s = strings.TrimSpace(s[4:]) // skip "has("
	if len(s) == 0 || s[0] != '"' {
		return nil, s, fmt.Errorf("has() requires a string field name")
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

	cond, rest, err := parsePipeExpr(s)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "then") || (len(rest) > 4 && isIdentChar(rest[4])) {
		return nil, rest, fmt.Errorf("expected 'then' in if expression")
	}
	rest = strings.TrimSpace(rest[4:])

	thenBranch, rest, err := parsePipeExpr(rest)
	if err != nil {
		return nil, rest, err
	}
	rest = strings.TrimSpace(rest)

	var elseBranch *op
	if strings.HasPrefix(rest, "else") && (len(rest) == 4 || !isIdentChar(rest[4])) {
		rest = strings.TrimSpace(rest[4:])
		elseBranch, rest, err = parsePipeExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		rest = strings.TrimSpace(rest)
	}

	if !strings.HasPrefix(rest, "end") || (len(rest) > 3 && isIdentChar(rest[3])) {
		return nil, rest, fmt.Errorf("expected 'end' to close if expression")
	}
	rest = rest[3:]

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
		fields = append(fields, *expr)
		s = rest
	}

	s = s[1:] // skip ')'

	if len(fields) == 0 {
		return nil, s, fmt.Errorf("del() requires at least one argument")
	}

	return &op{typ: opDelete, fields: fields}, s, nil
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

// parseBracketExpr parses [N] (index) or [] (iterator) after a dot or field.
// Assumes s starts with '['.
func parseBracketExpr(s string) (*op, string, error) {
	s = s[1:] // skip '['
	s = strings.TrimSpace(s)

	// .[] — iterator
	if len(s) > 0 && s[0] == ']' {
		s = s[1:] // skip ']'
		node := &op{typ: opIterator}
		// Check for optional marker
		if len(s) > 0 && s[0] == '?' {
			node.optional = true
			s = s[1:]
		}
		return node, s, nil
	}

	// .[N] or .[-N] — array index
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	idx, rest, err := parseInt(s)
	if err != nil {
		return nil, s, fmt.Errorf("expected number in array index: %w", err)
	}
	if neg {
		idx = -idx
	}
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ']' {
		return nil, rest, fmt.Errorf("expected ']' after array index")
	}
	rest = rest[1:] // skip ']'

	node := &op{typ: opIndex, index: idx}

	// Check for optional marker
	if len(rest) > 0 && rest[0] == '?' {
		node.optional = true
		rest = rest[1:]
	}

	// Check for further chaining: .[0].name or .[0][1]
	if len(rest) > 0 && rest[0] == '.' {
		if len(rest) > 1 && isIdentStart(rest[1]) {
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
		}
	} else if len(rest) > 0 && rest[0] == '[' {
		child, remaining, err := parseBracketExpr(rest)
		if err != nil {
			return nil, rest, err
		}
		node.child = child
		rest = remaining
	}

	return node, rest, nil
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

		// Read key name
		key, rest := readIdentifier(s)
		if key == "" {
			return nil, s, fmt.Errorf("expected field name in object construction")
		}
		s = strings.TrimSpace(rest)

		// Check for `: expr` (rename) or shorthand
		if len(s) > 0 && s[0] == ':' {
			s = strings.TrimSpace(s[1:])
			expr, remaining, err := parsePipeExpr(s)
			if err != nil {
				return nil, remaining, fmt.Errorf("in object construction: %w", err)
			}
			pairs = append(pairs, pair{key: key, expr: expr})
			s = strings.TrimSpace(remaining)
		} else {
			// Shorthand: {name} is equivalent to {name: .name}
			pairs = append(pairs, pair{key: key, expr: &op{typ: opField, field: key}})
		}
	}

	return &op{typ: opConstruct, pairs: pairs}, s, nil
}

// parseArrayConstruct parses array construction: [.foo, .bar].
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

		expr, remaining, err := parsePipeExpr(s)
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
func parseStringLiteral(s string) (*op, string, error) {
	i := 1 // skip opening '"'
	for i < len(s) {
		ch := s[i]
		if ch == '\\' {
			i += 2
			continue
		}
		if ch == '"' {
			i++ // include closing '"'
			return &op{typ: opLiteral, literal: []byte(s[:i])}, s[i:], nil
		}
		i++
	}
	return nil, s, fmt.Errorf("unterminated string literal")
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
	return &op{typ: opLiteral, literal: []byte(s[:i])}, s[i:], nil
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
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
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
