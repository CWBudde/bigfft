//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
)

var live []js.Func

func main() {
	namespace := js.Global().Get("Object").New()
	multiply := guardedMultiply()
	live = append(live, multiply)
	namespace.Set("multiply", multiply)
	namespace.Set("maxInputDigits", maxInputDigits)
	js.Global().Set("bigfft", namespace)

	select {}
}

func guardedMultiply() js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = errorResult(fmt.Sprintf("multiplication failed: %v", recovered), true)
			}
		}()

		if len(args) == 0 || args[0].Type() != js.TypeObject || args[0].IsNull() {
			return errorResult("multiply expects an options object", false)
		}
		opts := args[0]
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
	})
}

func errorResult(message string, panicked bool) js.Value {
	return js.ValueOf(map[string]any{
		"error": message,
		"panic": panicked,
	})
}
