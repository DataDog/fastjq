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
