//go:build arm64 && !purego

#include "textflag.h"

// func butterflyWords(sum, diff, x, y []Word) (carry, borrow Word)
//
// ARM64 has a single NZCV carry state, while the butterfly needs independent
// addition and subtraction chains. Process four words per block: keep each
// chain uninterrupted within the block, then materialize its carry/borrow as
// a 0/1 general register before switching NZCV to the other chain. This keeps
// the operands in registers and loads every source word only once.
TEXT ·butterflyWords(SB), NOSPLIT|NOFRAME, $0-112
	MOVD sum_base+0(FP), R0
	MOVD diff_base+24(FP), R1
	MOVD x_base+48(FP), R2
	MOVD y_base+72(FP), R3
	MOVD x_len+56(FP), R4
	MOVD ZR, R5 // saved addition carry, 0 or 1
	MOVD ZR, R6 // saved subtraction borrow, 0 or 1

block4:
	CMP $4, R4
	BLT tail

	LDP 0(R2), (R7, R8)
	LDP 16(R2), (R9, R10)
	LDP 0(R3), (R11, R12)
	LDP 16(R3), (R13, R14)

	// CMP $1, carry sets C exactly when carry == 1.
	CMP $1, R5
	ADCS R11, R7, R15
	ADCS R12, R8, R16
	ADCS R13, R9, R17
	ADCS R14, R10, R19
	ADC ZR, ZR, R5
	STP (R15, R16), 0(R0)
	STP (R17, R19), 16(R0)

	// CMP borrow, ZR computes 0-borrow, so C means "no borrow".
	CMP R6, ZR
	SBCS R11, R7
	SBCS R12, R8
	SBCS R13, R9
	SBCS R14, R10
	CSET CC, R6
	STP (R7, R8), 0(R1)
	STP (R9, R10), 16(R1)

	ADD $32, R0
	ADD $32, R1
	ADD $32, R2
	ADD $32, R3
	SUB $4, R4
	B block4

tail:
	CBZ R4, done

tailword:
	MOVD.P 8(R2), R7
	MOVD.P 8(R3), R8

	CMP $1, R5
	ADCS R8, R7, R9
	ADC ZR, ZR, R5
	MOVD.P R9, 8(R0)

	CMP R6, ZR
	SBCS R8, R7
	CSET CC, R6
	MOVD.P R7, 8(R1)

	SUB $1, R4
	CBNZ R4, tailword

done:
	MOVD R5, carry+96(FP)
	MOVD R6, borrow+104(FP)
	RET
