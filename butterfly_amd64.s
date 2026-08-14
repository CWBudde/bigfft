//go:build amd64 && !purego

#include "textflag.h"

// func butterflyWords(sum, diff, x, y []Word) (carry, borrow Word)
//
// Work two words at a time. Loading both operands once is the reason for the
// fused kernel; running two uninterrupted ADC/SBB pairs per block also halves
// the flag save/restore overhead needed when the independent chains share CF.
TEXT ·butterflyWords(SB), NOSPLIT, $0-112
	MOVQ sum_base+0(FP), DI
	MOVQ diff_base+24(FP), SI
	MOVQ x_base+48(FP), R8
	MOVQ y_base+72(FP), R9
	MOVQ x_len+56(FP), R14
	MOVQ R14, R15
	ANDQ $1, R15
	SHRQ $1, R14
	XORQ R12, R12 // saved carry, 0/-1
	XORQ R13, R13 // saved borrow, 0/-1

	TESTQ R15, R15
	JZ loop2
	MOVQ (R8), R10
	MOVQ (R9), AX
	ADDQ R12, R12
	MOVQ R10, CX
	ADCQ AX, CX
	SBBQ R12, R12
	MOVQ CX, (DI)
	ADDQ R13, R13
	SBBQ AX, R10
	SBBQ R13, R13
	MOVQ R10, (SI)
	LEAQ 8(DI), DI
	LEAQ 8(SI), SI
	LEAQ 8(R8), R8
	LEAQ 8(R9), R9

loop2:
	TESTQ R14, R14
	JZ done
	MOVQ 0(R8), R10
	MOVQ 8(R8), R11
	MOVQ 0(R9), AX
	MOVQ 8(R9), BX

	ADDQ R12, R12
	MOVQ R10, CX
	MOVQ R11, DX
	ADCQ AX, CX
	ADCQ BX, DX
	SBBQ R12, R12
	MOVQ CX, 0(DI)
	MOVQ DX, 8(DI)

	ADDQ R13, R13
	SBBQ AX, R10
	SBBQ BX, R11
	SBBQ R13, R13
	MOVQ R10, 0(SI)
	MOVQ R11, 8(SI)

	LEAQ 16(DI), DI
	LEAQ 16(SI), SI
	LEAQ 16(R8), R8
	LEAQ 16(R9), R9
	DECQ R14
	JMP loop2

done:
	NEGQ R12
	NEGQ R13
	MOVQ R12, carry+96(FP)
	MOVQ R13, borrow+104(FP)
	RET
