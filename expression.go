package jsonpath

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	exprErrorMismatchedParens  = "Mismatched parentheses"
	exprErrorBadExpression     = "Bad Expression"
	exprErrorFinalValueNotBool = "Expression evaluated to a non-bool"
	exprErrorNotEnoughOperands = "Not enough operands for operation %q"
	exprErrorBadValueForType   = "Bad value %q for type %q"
	exprErrorBadOperandType    = "Operand type expected to be %q for operation %q"
)

// Lowest priority = lowest #
var opa = map[int]struct {
	prec   int
	rAssoc bool
}{
	exprOpAnd:     {1, false},
	exprOpOr:      {1, false},
	exprOpEq:      {2, false},
	exprOpNeq:     {2, false},
	exprOpLt:      {3, false},
	exprOpLe:      {3, false},
	exprOpGt:      {3, false},
	exprOpGe:      {3, false},
	exprOpPlus:    {4, false},
	exprOpMinus:   {4, false},
	exprOpSlash:   {5, false},
	exprOpStar:    {5, false},
	exprOpPercent: {5, false}, // Disabled, no modulo for float
	exprOpHat:     {6, false},
}

// Shunting-yard Algorithm (infix -> postfix)
// http://rosettacode.org/wiki/Parsing/Shunting-yard_algorithm#Go
func infixToPostFix(items []Item) (out []Item, err error) {
	stack := newStack()

	for _, i := range items {
		switch i.typ {
		case exprParenLeft:
			stack.push(i) // push "(" to stack
		case exprParenRight:
			found := false
			for {
				// pop item ("(" or operator) from stack
				op_interface, ok := stack.pop()
				if !ok {
					return nil, errors.New(exprErrorMismatchedParens)
				}
				op := op_interface.(Item)
				if op.typ == exprParenLeft {
					found = true
					break // discard "("
				}
				out = append(out, op) // add operator to result
			}
			if !found {
				return nil, errors.New(exprErrorMismatchedParens)
			}
		default:
			if o1, isOp := opa[i.typ]; isOp {
				// token is an operator
				for stack.len() > 0 {
					// consider top item on stack
					op_int, _ := stack.peek()
					op := op_int.(Item)
					if o2, isOp := opa[op.typ]; !isOp || o1.prec > o2.prec ||
						o1.prec == o2.prec && o1.rAssoc {
						break
					}
					// top item is an operator that needs to come off
					stack.pop()           // pop it
					out = append(out, op) // add it to result
				}
				// push operator (the new one) to stack
				stack.push(i)
			} else { // token is an operand
				out = append(out, i) // add operand to result
			}
		}
	}
	// drain stack to result
	for stack.len() > 0 {
		op_int, _ := stack.pop()
		op := op_int.(Item)
		if op.typ == exprParenLeft {
			return nil, errors.New(exprErrorMismatchedParens)
		}
		out = append(out, op)
	}
	return
}

func evaluatePostFix(items []Item, values map[string]interface{}) (bool, error) {
	s := newStack()

	if len(items) == 0 {
		return false, errors.New(exprErrorBadExpression)
	}

	for _, item := range items {
		switch item.typ {

		// VALUES
		case exprBool:
			val, err := strconv.ParseBool(string(item.val))
			if err != nil {
				return false, fmt.Errorf(exprErrorBadValueForType, string(item.val), exprTokenNames[exprBool])
			}
			s.push(val)
		case exprNumber:
			val, err := strconv.ParseFloat(string(item.val), 64)
			if err != nil {
				return false, fmt.Errorf(exprErrorBadValueForType, string(item.val), exprTokenNames[exprNumber])
			}
			s.push(val)
		case exprPath:
			return false, fmt.Errorf("VALUES NOT SUPPORTED YET")

		// OPERATORS
		case exprOpAnd:
			a, b, err := take2Bool(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(a && b)
		case exprOpOr:
			a, b, err := take2Bool(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(a || b)
		case exprOpGt:
			a, b, err := take2Float(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(b > a)
		case exprOpGe:
			a, b, err := take2Float(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(b >= a)
		case exprOpLt:
			a, b, err := take2Float(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(b < a)
		case exprOpLe:
			a, b, err := take2Float(s, item.typ)
			if err != nil {
				return false, err
			}

			s.push(b <= a)
		// Other
		default:
			return false, fmt.Errorf("Token not supported in evaluator: %v", exprTokenNames[item.typ])
		}
	}

	if s.len() != 1 {
		fmt.Println(s.len())
		return false, fmt.Errorf(exprErrorBadExpression)
	}
	end_int, _ := s.pop()
	end_bool, ok := end_int.(bool)
	if !ok {
		return false, fmt.Errorf(exprErrorFinalValueNotBool)
	}
	return end_bool, nil
}

func firstError(errors ...error) error {
	for _, e := range errors {
		if e != nil {
			return e
		}
	}
	return nil
}

func take1Bool(s *stack, op int) (bool, error) {
	t := exprBool
	val, ok := s.pop()
	if !ok {
		return false, fmt.Errorf(exprErrorNotEnoughOperands, exprTokenNames[t], exprTokenNames[op])
	}

	b, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf(exprErrorBadOperandType, exprTokenNames[t], exprTokenNames[op])
	}
	return b, nil
}

func take2Bool(s *stack, op int) (bool, bool, error) {
	a, a_err := take1Bool(s, op)
	b, b_err := take1Bool(s, op)
	return a, b, firstError(a_err, b_err)
}

func take1Float(s *stack, op int) (float64, error) {
	t := exprNumber
	val, ok := s.pop()
	if !ok {
		return 0.0, fmt.Errorf(exprErrorNotEnoughOperands, exprTokenNames[t], exprTokenNames[op])
	}

	b, ok := val.(float64)
	if !ok {
		return 0.0, fmt.Errorf(exprErrorBadOperandType, exprTokenNames[t], exprTokenNames[op])
	}
	return b, nil
}

func take2Float(s *stack, op int) (float64, float64, error) {
	a, a_err := take1Float(s, op)
	b, b_err := take1Float(s, op)
	return a, b, firstError(a_err, b_err)
}
