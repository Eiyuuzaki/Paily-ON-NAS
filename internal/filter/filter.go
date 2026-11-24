// Package filter provides the expr-lang expression evaluation engine used by
// the filtering and naming subsystems.
package filter

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// caches holds compiled expr programs keyed by
// "<expr>\x00<envType>\x00<resultKind>".
var caches sync.Map // map[string]*vm.Program

// envKey names the environment type a program was compiled against.
//
// The compiled program binds fields by INDEX, not by name: NodeFilterEnv.Server
// is field 0, while SourceFilterEnv field 0 is ID. Reusing a program across
// targets therefore evaluates a correct-looking expression against the wrong
// field and returns a plausible but wrong answer, with no error anywhere.
func envKey(env any) string {
	if env == nil {
		return "nil"
	}
	return reflect.TypeOf(env).String()
}

// compiled returns a cached *vm.Program for the given key, compiling it on first
// access.
func compiled(expression string, cacheKey string, opts ...expr.Option) (*vm.Program, error) {
	if v, ok := caches.Load(cacheKey); ok {
		return v.(*vm.Program), nil
	}
	prog, err := expr.Compile(expression, opts...)
	if err != nil {
		return nil, fmt.Errorf("filter: compile %q: %w", expression, err)
	}
	caches.Store(cacheKey, prog)
	return prog, nil
}

// EvalBool evaluates expression against env, expecting a bool result.
// env must be a struct compatible with the expression.
func EvalBool(expression string, env any) (bool, error) {
	cacheKey := expression + "\x00" + envKey(env) + "\x00bool"
	prog, err := compiled(expression, cacheKey, expr.Env(env), expr.AsBool())
	if err != nil {
		return false, err
	}
	result, err := vm.Run(prog, env)
	if err != nil {
		return false, fmt.Errorf("filter: eval %q: %w", expression, err)
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("filter: expression %q returned %T, expected bool", expression, result)
	}
	return b, nil
}

// EvalString evaluates expression against env, expecting a string result.
func EvalString(expression string, env any) (string, error) {
	// Try compiling with AsKind(reflect.String) first; fall back to AsAny for
	// complex expressions (e.g. ternary chains) the type-checker can't infer.
	cacheKey := expression + "\x00" + envKey(env) + "\x00string"
	prog, err := compiled(expression, cacheKey, expr.Env(env), expr.AsKind(reflect.String))
	if err != nil {
		cacheKey2 := expression + "\x00" + envKey(env) + "\x00any"
		prog2, err2 := compiled(expression, cacheKey2, expr.Env(env), expr.AsAny())
		if err2 != nil {
			return "", fmt.Errorf("filter: compile %q: %w", expression, err)
		}
		result, err2 := vm.Run(prog2, env)
		if err2 != nil {
			return "", fmt.Errorf("filter: eval %q: %w", expression, err2)
		}
		if s, ok := result.(string); ok {
			return s, nil
		}
		return fmt.Sprintf("%v", result), nil
	}
	result, err := vm.Run(prog, env)
	if err != nil {
		return "", fmt.Errorf("filter: eval %q: %w", expression, err)
	}
	if s, ok := result.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", result), nil
}
