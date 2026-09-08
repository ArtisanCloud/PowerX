// Package evidence 定义平台通用计算和可追踪结果；不包含领域指标公式。
package evidence

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"regexp"
	"strings"
)

const CalculatorKey = "powerx.calculation.evaluate"
const CalculatorVersion = "1.0.0"

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)$`)

func decimal(value string) (*big.Rat, error) {
	if len(value) > 80 || !decimalPattern.MatchString(value) {
		return nil, fmt.Errorf("calculation.number_invalid")
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("calculation.number_invalid")
	}
	return r, nil
}

func validateBoundExpression(expression string, bindings map[string]string) error {
	if len(expression) > 512 {
		return fmt.Errorf("calculation.limits_exceeded")
	}
	node, err := parser.ParseExpr(expression)
	if err != nil {
		return fmt.Errorf("calculation.expression_invalid")
	}
	used := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.BasicLit:
			err = fmt.Errorf("evidence.literal_operand_forbidden")
			return false
		case *ast.Ident:
			if _, ok := bindings[n.Name]; !ok {
				err = fmt.Errorf("calculation.binding_missing: %s", n.Name)
			}
			used[n.Name] = true
		}
		return true
	})
	if err != nil {
		return err
	}
	if len(used) != len(bindings) {
		return fmt.Errorf("evidence.unused_binding")
	}
	return nil
}

// Calculate 只解释算术 AST，绝不执行 Go 代码；使用有理数避免浮点误差。
// 允许的字面常量仅为显式表达式中的比例因子；业务操作数由调用方绑定。
func Calculate(ctx context.Context, expression string, bindings map[string]string, precision int, percent bool) (string, error) {
	if len(expression) == 0 || len(expression) > 512 || len(bindings) > 32 || precision < 0 || precision > 12 {
		return "", fmt.Errorf("calculation.limits_exceeded")
	}
	node, err := parser.ParseExpr(expression)
	if err != nil {
		return "", fmt.Errorf("calculation.expression_invalid")
	}
	steps := 0
	var evaluate func(ast.Expr) (*big.Rat, error)
	evaluate = func(n ast.Expr) (*big.Rat, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		steps++
		if steps > 128 {
			return nil, fmt.Errorf("calculation.limits_exceeded")
		}
		switch n := n.(type) {
		case *ast.Ident:
			value, ok := bindings[n.Name]
			if !ok {
				return nil, fmt.Errorf("calculation.binding_missing: %s", n.Name)
			}
			return decimal(value)
		case *ast.BasicLit:
			if n.Kind != token.INT && n.Kind != token.FLOAT {
				return nil, fmt.Errorf("calculation.expression_invalid")
			}
			return decimal(n.Value)
		case *ast.ParenExpr:
			return evaluate(n.X)
		case *ast.UnaryExpr:
			x, err := evaluate(n.X)
			if err != nil {
				return nil, err
			}
			if n.Op == token.SUB {
				return x.Neg(x), nil
			}
			if n.Op == token.ADD {
				return x, nil
			}
		case *ast.BinaryExpr:
			x, err := evaluate(n.X)
			if err != nil {
				return nil, err
			}
			y, err := evaluate(n.Y)
			if err != nil {
				return nil, err
			}
			r := new(big.Rat)
			switch n.Op {
			case token.ADD:
				r.Add(x, y)
			case token.SUB:
				r.Sub(x, y)
			case token.MUL:
				r.Mul(x, y)
			case token.QUO:
				if y.Sign() == 0 {
					return nil, fmt.Errorf("calculation.division_by_zero")
				}
				r.Quo(x, y)
			default:
				return nil, fmt.Errorf("calculation.expression_invalid")
			}
			if r.Num().BitLen() > 4096 || r.Denom().BitLen() > 4096 {
				return nil, fmt.Errorf("calculation.limits_exceeded")
			}
			return r, nil
		}
		return nil, fmt.Errorf("calculation.expression_invalid")
	}
	value, err := evaluate(node)
	if err != nil {
		return "", err
	}
	if percent {
		value.Mul(value, big.NewRat(100, 1))
	}
	result := value.FloatString(precision)
	if strings.Contains(result, ".") {
		result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	}
	if result == "-0" {
		result = "0"
	}
	if percent {
		result += "%"
	}
	return result, nil
}
