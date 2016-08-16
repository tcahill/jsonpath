package jsonpath

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	opTypeIndex = iota
	opTypeIndexRange
	opTypeIndexWild
	opTypeIndexExpr
	opTypeName
	opTypeNameList
	opTypeNameWild
)

type Path struct {
	stringValue     string
	operators       []*operator
	captureEndValue bool
}

type operator struct {
	typ         int
	indexStart  int
	indexEnd    int
	hasIndexEnd bool
	keyStrings  map[string]struct{}

	dependentPaths []*Path
	whereClause    []Item
}

func genIndexKey(tr tokenReader) (*operator, error) {
	k := &operator{}
	var t *Item
	var ok bool
	if t, ok = tr.next(); !ok {
		return nil, errors.New("Expected number, key, expression or *, but got none")
	}

	switch t.typ {
	case pathWildcard:
		k.typ = opTypeIndexWild
		k.indexStart = 0
		if t, ok = tr.next(); !ok {
			return nil, errors.New("Expected ] after *, but got none")
		}
		if t.typ != pathBracketRight {
			return nil, fmt.Errorf("Expected ] after * instead of %q", t.val)
		}
	case pathIndex:
		v, err := strconv.Atoi(string(t.val))
		if err != nil {
			return nil, fmt.Errorf("Could not parse %q into int64", t.val)
		}
		k.indexStart = v
		k.indexEnd = v
		k.hasIndexEnd = true

		if t, ok = tr.next(); !ok {
			return nil, errors.New("Expected number or *, but got none")
		}
		switch t.typ {
		case pathIndexRange:
			if t, ok = tr.next(); !ok {
				return nil, errors.New("Expected number or *, but got none")
			}
			switch t.typ {
			case pathIndex:
				v, err := strconv.Atoi(string(t.val))
				if err != nil {
					return nil, fmt.Errorf("Could not parse %q into int64", t.val)
				}
				k.indexEnd = v - 1
				k.hasIndexEnd = true

				if t, ok = tr.next(); !ok || t.typ != pathBracketRight {
					return nil, errors.New("Expected ], but got none")
				}
			case pathBracketRight:
				k.hasIndexEnd = false
			default:
				return nil, fmt.Errorf("Unexpected value within brackets after index: %q", t.val)
			}

			k.typ = opTypeIndexRange
		case pathBracketRight:
			k.typ = opTypeIndex

		default:
			return nil, fmt.Errorf("Unexpected value within brackets after index: %q", t.val)
		}
	case pathKey:
		k.keyStrings = map[string]struct{}{string(t.val[1 : len(t.val)-1]): struct{}{}}
		k.typ = opTypeName

		if t, ok = tr.next(); !ok || t.typ != pathBracketRight {
			return nil, errors.New("Expected ], but got none")
		}
	case pathWhere:
		k.typ = opTypeIndexExpr
		var err error
		if t, ok := tr.next(); !ok || t.typ != pathExpression {
			fmt.Printf("Value: %s\n", t.val)
			return nil, fmt.Errorf("Expected expression, but got %s", pathTokenNames[t.typ])
		}
		trimmed := t.val[1 : len(t.val)-1]
		whereLexer := NewSliceLexer(trimmed, EXPRESSION)
		items := readerToArray(whereLexer)
		if errItem, found := findErrors(items); found {
			return nil, errors.New(string(errItem.val))
		}

		// transform expression into postfix form
		k.whereClause, err = infixToPostFix(items[:len(items)-1]) // trim EOF
		if err != nil {
			return nil, err
		}
		k.dependentPaths = make([]*Path, 0)
		// parse all paths in expression
		for _, item := range k.whereClause {
			if item.typ == exprPath {
				p, err := parsePath(string(item.val))
				if err != nil {
					return nil, err
				}
				k.dependentPaths = append(k.dependentPaths, p)
			}
		}
	default:
		return nil, fmt.Errorf("Unexpected value within brackets: %q", t.val)
	}

	return k, nil
}

func parsePath(pathString string) (*Path, error) {
	lexer := NewSliceLexer([]byte(pathString), PATH)
	p, err := tokensToOperators(lexer)
	if err != nil {
		return nil, err
	}

	p.stringValue = pathString
	return p, nil
}

func tokensToOperators(tr tokenReader) (*Path, error) {
	q := &Path{
		stringValue:     "",
		captureEndValue: false,
		operators:       make([]*operator, 0),
	}
	for {
		p, ok := tr.next()
		if !ok {
			break
		}
		switch p.typ {
		case pathRoot:
			if len(q.operators) != 0 {
				return nil, errors.New("Unexpected root node after start")
			}
			continue
		case pathCurrent:
			if len(q.operators) != 0 {
				return nil, errors.New("Unexpected current node after start")
			}
			continue
		case pathPeriod:
			continue
		case pathBracketLeft:
			k, err := genIndexKey(tr)
			if err != nil {
				return nil, err
			}
			q.operators = append(q.operators, k)
		case pathKey:
			keyName := p.val
			if len(p.val) == 0 {
				return nil, fmt.Errorf("Key length is zero at %d", p.pos)
			}
			if p.val[0] == '"' && p.val[len(p.val)-1] == '"' {
				keyName = p.val[1 : len(p.val)-1]
			}
			q.operators = append(q.operators, &operator{typ: opTypeName, keyStrings: map[string]struct{}{string(keyName): struct{}{}}})
		case pathWildcard:
			q.operators = append(q.operators, &operator{typ: opTypeNameWild})
		case pathValue:
			q.captureEndValue = true
		case pathError:
			return q, errors.New(string(p.val))
		}
	}
	return q, nil
}
