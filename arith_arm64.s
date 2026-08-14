// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Adapted from Go's math/big arithmetic assembly.

//go:build arm64 && !purego

#include "textflag.h"

// func addVV(z, x, y []Word) (c Word)
TEXT ·addVV(SB), NOSPLIT, $0-80
	MOVD z_len+8(FP), R0
	MOVD x_base+24(FP), R1
	MOVD y_base+48(FP), R2
	MOVD z_base+0(FP), R3
	AND $3, R0, R4
	LSR $2, R0
	ADDS ZR, R0

addVV_loop1:
	CBZ R4, addVV_loop4
	MOVD.P 8(R1), R5
	MOVD.P 8(R2), R6
	ADCS R6, R5
	MOVD.P R5, 8(R3)
	SUB $1, R4
	CBNZ R4, addVV_loop1

addVV_loop4:
	CBZ R0, addVV_done
	LDP.P 32(R1), (R4, R5)
	LDP -16(R1), (R6, R7)
	LDP.P 32(R2), (R8, R9)
	LDP -16(R2), (R10, R11)
	ADCS R8, R4
	ADCS R9, R5
	ADCS R10, R6
	ADCS R11, R7
	STP.P (R4, R5), 32(R3)
	STP (R6, R7), -16(R3)
	SUB $1, R0
	CBNZ R0, addVV_loop4

addVV_done:
	ADC ZR, ZR, R1
	MOVD R1, c+72(FP)
	RET

// func subVV(z, x, y []Word) (c Word)
TEXT ·subVV(SB), NOSPLIT, $0-80
	MOVD z_len+8(FP), R0
	MOVD x_base+24(FP), R1
	MOVD y_base+48(FP), R2
	MOVD z_base+0(FP), R3
	AND $3, R0, R4
	LSR $2, R0
	SUBS ZR, R0

subVV_loop1:
	CBZ R4, subVV_loop4
	MOVD.P 8(R1), R5
	MOVD.P 8(R2), R6
	SBCS R6, R5
	MOVD.P R5, 8(R3)
	SUB $1, R4
	CBNZ R4, subVV_loop1

subVV_loop4:
	CBZ R0, subVV_done
	LDP.P 32(R1), (R4, R5)
	LDP -16(R1), (R6, R7)
	LDP.P 32(R2), (R8, R9)
	LDP -16(R2), (R10, R11)
	SBCS R8, R4
	SBCS R9, R5
	SBCS R10, R6
	SBCS R11, R7
	STP.P (R4, R5), 32(R3)
	STP (R6, R7), -16(R3)
	SUB $1, R0
	CBNZ R0, subVV_loop4

subVV_done:
	SBC R1, R1
	SUB R1, ZR, R1
	MOVD R1, c+72(FP)
	RET

// func lshVU(z, x []Word, s uint) (c Word)
TEXT ·lshVU(SB), NOSPLIT, $0-64
	MOVD z_len+8(FP), R0
	CBZ R0, lshVU_zero
	MOVD s+48(FP), R1
	MOVD x_base+24(FP), R2
	MOVD z_base+0(FP), R3
	ADD R0<<3, R2, R2
	ADD R0<<3, R3, R3
	MOVD.W -8(R2), R4
	MOVD $64, R5
	SUB R1, R5
	LSR R5, R4, R6
	LSL R1, R4
	MOVD R6, c+56(FP)
	SUB $1, R0
	AND $3, R0, R6
	LSR $2, R0

lshVU_loop1:
	CBZ R6, lshVU_loop4
	MOVD.W -8(R2), R7
	LSR R5, R7, R8
	ORR R4, R8
	LSL R1, R7, R4
	MOVD.W R8, -8(R3)
	SUB $1, R6
	CBNZ R6, lshVU_loop1

lshVU_loop4:
	CBZ R0, lshVU_done
	LDP.W -32(R2), (R9, R8)
	LDP 16(R2), (R7, R6)
	LSR R5, R6, R10
	ORR R4, R10
	LSL R1, R6, R4
	LSR R5, R7, R6
	ORR R4, R6
	LSL R1, R7, R4
	LSR R5, R8, R7
	ORR R4, R7
	LSL R1, R8, R4
	LSR R5, R9, R8
	ORR R4, R8
	LSL R1, R9, R4
	STP.W (R8, R7), -32(R3)
	STP (R6, R10), 16(R3)
	SUB $1, R0
	CBNZ R0, lshVU_loop4

lshVU_done:
	MOVD.W R4, -8(R3)
	RET

lshVU_zero:
	MOVD ZR, c+56(FP)
	RET

// func addMulVVW(z, x []Word, y Word) (c Word)
TEXT ·addMulVVW(SB), NOSPLIT, $0-64
	MOVD y+48(FP), R0
	MOVD ZR, R1
	MOVD z_len+8(FP), R2
	MOVD z_base+0(FP), R3
	MOVD x_base+24(FP), R4
	MOVD z_base+0(FP), R5
	AND $7, R2, R6
	LSR $3, R2

addMulVVW_loop1:
	CBZ R6, addMulVVW_loop8
	MOVD.P 8(R3), R7
	MOVD.P 8(R4), R8
	UMULH R0, R8, R9
	MUL R0, R8
	ADDS R1, R8
	ADC ZR, R9, R1
	ADDS R7, R8
	ADC ZR, R1
	MOVD.P R8, 8(R5)
	SUB $1, R6
	CBNZ R6, addMulVVW_loop1

addMulVVW_loop8:
	CBZ R2, addMulVVW_done
	LDP.P 64(R3), (R6, R7)
	LDP -48(R3), (R8, R9)
	LDP -32(R3), (R10, R11)
	LDP -16(R3), (R12, R13)
	LDP.P 64(R4), (R14, R15)
	LDP -48(R4), (R16, R17)
	LDP -32(R4), (R19, R20)
	LDP -16(R4), (R21, R22)
	UMULH R0, R14, R23
	MUL R0, R14
	ADDS R1, R14
	UMULH R0, R15, R24
	MUL R0, R15
	ADCS R23, R15
	UMULH R0, R16, R23
	MUL R0, R16
	ADCS R24, R16
	UMULH R0, R17, R24
	MUL R0, R17
	ADCS R23, R17
	UMULH R0, R19, R23
	MUL R0, R19
	ADCS R24, R19
	UMULH R0, R20, R24
	MUL R0, R20
	ADCS R23, R20
	UMULH R0, R21, R23
	MUL R0, R21
	ADCS R24, R21
	UMULH R0, R22, R24
	MUL R0, R22
	ADCS R23, R22
	ADC ZR, R24, R1
	ADDS R6, R14
	ADCS R7, R15
	ADCS R8, R16
	ADCS R9, R17
	ADCS R10, R19
	ADCS R11, R20
	ADCS R12, R21
	ADCS R13, R22
	ADC ZR, R1
	STP.P (R14, R15), 64(R5)
	STP (R16, R17), -48(R5)
	STP (R19, R20), -32(R5)
	STP (R21, R22), -16(R5)
	SUB $1, R2
	CBNZ R2, addMulVVW_loop8

addMulVVW_done:
	MOVD R1, c+56(FP)
	RET
