// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Adapted from Go's math/big arithmetic assembly.

//go:build amd64 && !purego

#include "textflag.h"

// func addVV(z, x, y []Word) (c Word)
TEXT ·addVV(SB), NOSPLIT, $0-80
	MOVQ z_len+8(FP), BX
	MOVQ x_base+24(FP), SI
	MOVQ y_base+48(FP), DI
	MOVQ z_base+0(FP), R8
	MOVQ BX, R9
	ANDQ $3, R9
	SHRQ $2, BX
	MOVQ $0, R10

addVV_loop1:
	TESTQ R9, R9
	JZ addVV_loop4
	ADDQ R10, R10
	MOVQ 0(SI), R10
	ADCQ 0(DI), R10
	MOVQ R10, 0(R8)
	SBBQ R10, R10
	LEAQ 8(SI), SI
	LEAQ 8(DI), DI
	LEAQ 8(R8), R8
	SUBQ $1, R9
	JNZ addVV_loop1

addVV_loop4:
	TESTQ BX, BX
	JZ addVV_done
	ADDQ R10, R10
	MOVQ 0(SI), R9
	MOVQ 8(SI), R10
	MOVQ 16(SI), R11
	MOVQ 24(SI), R12
	ADCQ 0(DI), R9
	ADCQ 8(DI), R10
	ADCQ 16(DI), R11
	ADCQ 24(DI), R12
	MOVQ R9, 0(R8)
	MOVQ R10, 8(R8)
	MOVQ R11, 16(R8)
	MOVQ R12, 24(R8)
	SBBQ R10, R10
	LEAQ 32(SI), SI
	LEAQ 32(DI), DI
	LEAQ 32(R8), R8
	SUBQ $1, BX
	JNZ addVV_loop4

addVV_done:
	NEGQ R10
	MOVQ R10, c+72(FP)
	RET

// func subVV(z, x, y []Word) (c Word)
TEXT ·subVV(SB), NOSPLIT, $0-80
	MOVQ z_len+8(FP), BX
	MOVQ x_base+24(FP), SI
	MOVQ y_base+48(FP), DI
	MOVQ z_base+0(FP), R8
	MOVQ BX, R9
	ANDQ $3, R9
	SHRQ $2, BX
	MOVQ $0, R10

subVV_loop1:
	TESTQ R9, R9
	JZ subVV_loop4
	ADDQ R10, R10
	MOVQ 0(SI), R10
	SBBQ 0(DI), R10
	MOVQ R10, 0(R8)
	SBBQ R10, R10
	LEAQ 8(SI), SI
	LEAQ 8(DI), DI
	LEAQ 8(R8), R8
	SUBQ $1, R9
	JNZ subVV_loop1

subVV_loop4:
	TESTQ BX, BX
	JZ subVV_done
	ADDQ R10, R10
	MOVQ 0(SI), R9
	MOVQ 8(SI), R10
	MOVQ 16(SI), R11
	MOVQ 24(SI), R12
	SBBQ 0(DI), R9
	SBBQ 8(DI), R10
	SBBQ 16(DI), R11
	SBBQ 24(DI), R12
	MOVQ R9, 0(R8)
	MOVQ R10, 8(R8)
	MOVQ R11, 16(R8)
	MOVQ R12, 24(R8)
	SBBQ R10, R10
	LEAQ 32(SI), SI
	LEAQ 32(DI), DI
	LEAQ 32(R8), R8
	SUBQ $1, BX
	JNZ subVV_loop4

subVV_done:
	NEGQ R10
	MOVQ R10, c+72(FP)
	RET

// func lshVU(z, x []Word, s uint) (c Word)
TEXT ·lshVU(SB), NOSPLIT, $0-64
	MOVQ z_len+8(FP), BX
	TESTQ BX, BX
	JZ lshVU_zero
	MOVQ s+48(FP), CX
	MOVQ x_base+24(FP), SI
	MOVQ z_base+0(FP), DI
	LEAQ (SI)(BX*8), SI
	LEAQ (DI)(BX*8), DI
	MOVQ -8(SI), R8
	MOVQ $0, R9
	SHLQ CX, R8, R9
	MOVQ R9, c+56(FP)
	SUBQ $1, BX
	MOVQ BX, R9
	ANDQ $3, R9
	SHRQ $2, BX

lshVU_loop1:
	TESTQ R9, R9
	JZ lshVU_loop4
	MOVQ -16(SI), R10
	SHLQ CX, R10, R8
	MOVQ R8, -8(DI)
	MOVQ R10, R8
	LEAQ -8(SI), SI
	LEAQ -8(DI), DI
	SUBQ $1, R9
	JNZ lshVU_loop1

lshVU_loop4:
	TESTQ BX, BX
	JZ lshVU_done
	MOVQ -16(SI), R9
	MOVQ -24(SI), R10
	MOVQ -32(SI), R11
	MOVQ -40(SI), R12
	SHLQ CX, R9, R8
	SHLQ CX, R10, R9
	SHLQ CX, R11, R10
	SHLQ CX, R12, R11
	MOVQ R8, -8(DI)
	MOVQ R9, -16(DI)
	MOVQ R10, -24(DI)
	MOVQ R11, -32(DI)
	MOVQ R12, R8
	LEAQ -32(SI), SI
	LEAQ -32(DI), DI
	SUBQ $1, BX
	JNZ lshVU_loop4

lshVU_done:
	SHLQ CX, R8
	MOVQ R8, -8(DI)
	RET

lshVU_zero:
	MOVQ $0, c+56(FP)
	RET

// func addMulVVW(z, x []Word, y Word) (c Word)
TEXT ·addMulVVW(SB), NOSPLIT, $0-64
	MOVQ y+48(FP), BX
	MOVQ $0, SI
	MOVQ z_len+8(FP), DI
	MOVQ z_base+0(FP), R8
	MOVQ x_base+24(FP), R9
	MOVQ z_base+0(FP), R10
	MOVQ DI, R11
	ANDQ $3, R11
	SHRQ $2, DI

addMulVVW_loop1:
	TESTQ R11, R11
	JZ addMulVVW_loop4
	MOVQ 0(R9), AX
	MULQ BX
	ADDQ SI, AX
	MOVQ DX, SI
	ADCQ $0, SI
	ADDQ 0(R8), AX
	ADCQ $0, SI
	MOVQ AX, 0(R10)
	LEAQ 8(R8), R8
	LEAQ 8(R9), R9
	LEAQ 8(R10), R10
	SUBQ $1, R11
	JNZ addMulVVW_loop1

addMulVVW_loop4:
	TESTQ DI, DI
	JZ addMulVVW_done
	MOVQ 0(R9), AX
	MULQ BX
	ADDQ SI, AX
	MOVQ DX, SI
	ADCQ $0, SI
	ADDQ 0(R8), AX
	ADCQ $0, SI
	MOVQ AX, 0(R10)
	MOVQ 8(R9), AX
	MULQ BX
	ADDQ SI, AX
	MOVQ DX, SI
	ADCQ $0, SI
	ADDQ 8(R8), AX
	ADCQ $0, SI
	MOVQ AX, 8(R10)
	MOVQ 16(R9), AX
	MULQ BX
	ADDQ SI, AX
	MOVQ DX, SI
	ADCQ $0, SI
	ADDQ 16(R8), AX
	ADCQ $0, SI
	MOVQ AX, 16(R10)
	MOVQ 24(R9), AX
	MULQ BX
	ADDQ SI, AX
	MOVQ DX, SI
	ADCQ $0, SI
	ADDQ 24(R8), AX
	ADCQ $0, SI
	MOVQ AX, 24(R10)
	LEAQ 32(R8), R8
	LEAQ 32(R9), R9
	LEAQ 32(R10), R10
	SUBQ $1, DI
	JNZ addMulVVW_loop4

addMulVVW_done:
	MOVQ SI, c+56(FP)
	RET
