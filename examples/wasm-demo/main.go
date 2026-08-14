//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
)

var live []js.Func

func main() {
	namespace := js.Global().Get("Object").New()
	exports := map[string]func(js.Value) any{
		"multiply":  jsMultiply,
		"benchmark": jsBenchmark,
	}
	for name, export := range exports {
		wrapped := guard(name, export)
		live = append(live, wrapped)
		namespace.Set(name, wrapped)
	}
	namespace.Set("maxInputDigits", maxInputDigits)
	js.Global().Set("bigfft", namespace)

	select {}
}

func guard(name string, export func(js.Value) any) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = errorResult(fmt.Sprintf("%s failed: %v", name, recovered), true)
			}
		}()

		opts := js.Undefined()
		if len(args) > 0 {
			opts = args[0]
		}
		return export(opts)
	})
}

func jsMultiply(opts js.Value) any {
	if !isObject(opts) {
		return errorResult("multiply expects an options object", false)
	}
	left := opts.Get("left")
	right := opts.Get("right")
	if left.Type() != js.TypeString || right.Type() != js.TypeString {
		return errorResult("left and right must be decimal strings", false)
	}

	calculation, err := calculate(left.String(), right.String())
	if err != nil {
		return errorResult(err.Error(), false)
	}

	return js.ValueOf(map[string]any{
		"product":        calculation.product,
		"leftDigits":     calculation.leftDigits,
		"rightDigits":    calculation.rightDigits,
		"productDigits":  calculation.productDigits,
		"leftBits":       calculation.leftBits,
		"rightBits":      calculation.rightBits,
		"productBits":    calculation.productBits,
		"parseMillis":    calculation.parseMillis,
		"multiplyMillis": calculation.multiplyMillis,
		"formatMillis":   calculation.formatMillis,
	})
}

func jsBenchmark(opts js.Value) any {
	if !isObject(opts) {
		return errorResult("benchmark expects an options object", false)
	}
	digits := opts.Get("digits")
	iterations := opts.Get("iterations")
	if digits.Type() != js.TypeNumber || iterations.Type() != js.TypeNumber {
		return errorResult("digits and iterations must be numbers", false)
	}

	benchmark, err := benchmarkMultiplication(digits.Int(), iterations.Int())
	if err != nil {
		return errorResult(err.Error(), false)
	}

	return js.ValueOf(map[string]any{
		"digits":          benchmark.digits,
		"iterations":      benchmark.iterations,
		"inputBits":       benchmark.inputBits,
		"bigfftMillis":    benchmark.bigfftMillis,
		"standardMillis":  benchmark.standardMillis,
		"speedup":         benchmark.speedup,
		"resultsVerified": benchmark.resultsVerified,
	})
}

func isObject(value js.Value) bool {
	return value.Type() == js.TypeObject && !value.IsNull()
}

func errorResult(message string, panicked bool) js.Value {
	return js.ValueOf(map[string]any{
		"error": message,
		"panic": panicked,
	})
}
