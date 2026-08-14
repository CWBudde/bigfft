//go:build arm64 && !purego

#include "textflag.h"

// func shiftMod(z, x *Word, n, shift uintptr)
//
// This is fermat.Shift expressed as one leaf kernel. n excludes the high
// representative word; shift is normalized by the Go wrapper to [0,2*n*64).
// z and x must be disjoint, as they are at every production call site.
TEXT ·shiftMod(SB), NOSPLIT|NOFRAME, $0-32
	MOVD z+0(FP), R0
	MOVD x+8(FP), R1
	MOVD n+16(FP), R2
	MOVD shift+24(FP), R3
	MOVD $1, R7
	ADD R2<<3, R0, R8
	MOVD R7, (R8)

	// Split the period into its positive and negative halves. R5=kw and
	// R6=kb remain live through the word construction and bit shift.
	LSL $6, R2, R4
	CMP R4, R3
	BHS negative

positive:
	LSR $6, R3, R5
	AND $63, R3, R6

	// z[:kw] = 0.
	MOVD R5, R7
	MOVD R0, R8
pos_zero:
	CBZ R7, pos_copy_setup
	MOVD.P ZR, 8(R8)
	SUB $1, R7
	B pos_zero

pos_copy_setup:
	// z[kw:n] = x[:n-kw].
	SUB R5, R2, R7
	ADD R5<<3, R0, R8
	MOVD R1, R9
pos_copy:
	CBZ R7, pos_sub_setup
	MOVD.P 8(R9), R10
	MOVD.P R10, 8(R8)
	SUB $1, R7
	B pos_copy

pos_sub_setup:
	// z[:kw+1] -= x[n-kw:n+1].
	MOVD R0, R8
	SUB R5, R2, R7
	ADD R7<<3, R1, R9
	ADD $1, R5, R7
	SUBS ZR, R7 // set C=1 (no incoming borrow)
pos_sub:
	MOVD.P 8(R8), R10
	MOVD.P 8(R9), R11
	SBCS R11, R10
	MOVD R10, -8(R8)
	SUB $1, R7
	CBNZ R7, pos_sub
	CSET CC, R10 // borrow, 0 or 1

	// Propagate that borrow through z[kw+1:n+1].
	ADD $1, R5, R7
	ADD R7<<3, R0, R8
	SUB R5, R2, R7 // number of words including the sentinel
	CMP R10, ZR   // C is "no borrow"
pos_borrow:
	MOVD (R8), R9
	SBCS ZR, R9
	MOVD.P R9, 8(R8)
	BCS add_back_one
	SUB $1, R7
	CBNZ R7, pos_borrow
	B add_back_one

negative:
	SUB R4, R3
	LSR $6, R3, R5
	AND $63, R3, R6

	// z[kw+1:n] = 0 (the sentinel z[n] remains one).
	ADD $1, R5, R7
	ADD R7<<3, R0, R8
	SUB R7, R2, R7
neg_zero:
	CBZ R7, neg_copy_setup
	MOVD.P ZR, 8(R8)
	SUB $1, R7
	B neg_zero

neg_copy_setup:
	// z[:kw+1] = x[n-kw:n+1].
	SUB R5, R2, R7
	ADD R7<<3, R1, R9
	MOVD R0, R8
	ADD $1, R5, R7
neg_copy:
	MOVD.P 8(R9), R10
	MOVD.P R10, 8(R8)
	SUB $1, R7
	CBNZ R7, neg_copy

	// z[kw:n] -= x[:n-kw].
	ADD R5<<3, R0, R8
	MOVD R1, R9
	SUB R5, R2, R7
	SUBS ZR, R7 // set C=1 (no incoming borrow)
neg_sub:
	MOVD.P 8(R8), R10
	MOVD.P 8(R9), R11
	SBCS R11, R10
	MOVD R10, -8(R8)
	SUB $1, R7
	CBNZ R7, neg_sub
	CSET CC, R10
	ADD R2<<3, R0, R8
	MOVD (R8), R9
	SUB R10, R9
	MOVD R9, (R8)

add_back_one:
	// Undo the -1 used by the word-rotation construction.
	ADD R2<<3, R0, R8
	MOVD (R8), R9
	CBZ R9, add_low_one
	SUB $1, R9
	MOVD R9, (R8)
	B bit_shift

add_low_one:
	// Add one across n+1 words. Usually the first ADCS clears carry.
	MOVD R0, R8
	ADD $1, R2, R7
	CMP ZR, ZR // set C=1
add_one_loop:
	MOVD (R8), R9
	ADCS ZR, R9
	MOVD.P R9, 8(R8)
	BCC bit_shift
	SUB $1, R7
	CBNZ R7, add_one_loop

bit_shift:
	CBZ R6, normalize
	// In-place lshVU over n+1 words. Input and output pointers advance
	// independently from the high end so no source word is overwritten early.
	ADD $1, R2, R7
	ADD R7<<3, R0, R8
	MOVD R8, R11
	MOVD.W -8(R8), R9
	MOVD $64, R10
	SUB R6, R10
	LSL R6, R9
	SUB $1, R7
shift_loop:
	CBZ R7, shift_final
	MOVD.W -8(R8), R12
	LSR R10, R12, R13
	ORR R9, R13
	LSL R6, R12, R9
	MOVD.W R13, -8(R11)
	SUB $1, R7
	B shift_loop
shift_final:
	MOVD.W R9, -8(R11)

normalize:
	ADD R2<<3, R0, R8
	MOVD (R8), R10 // c = z[n]
	CBZ R10, done
	MOVD (R0), R9
	CMP R10, R9
	BLO norm_sub_all
	SUB R10, R9
	MOVD R9, (R0)
	MOVD ZR, (R8)
	B done

norm_sub_all:
	// z -= c across all n+1 words (subVW). The first word subtracts c;
	// subsequent words subtract only the propagated 0/1 borrow.
	MOVD R0, R8
	ADD $1, R2, R7
	MOVD (R8), R9
	SUBS R10, R9
	MOVD.P R9, 8(R8)
	SUB $1, R7
norm_sub_loop:
	CBZ R7, norm_after_sub
	MOVD (R8), R9
	SBCS ZR, R9
	MOVD.P R9, 8(R8)
	SUB $1, R7
	B norm_sub_loop

norm_after_sub:
	CMP $1, R10
	BLS norm_check_one
	ADD R2<<3, R0, R8
	MOVD (R8), R9
	SUB $1, R10, R11
	SUB R11, R9
	MOVD R9, (R8)

norm_check_one:
	ADD R2<<3, R0, R8
	MOVD (R8), R9
	CMP $1, R9
	BNE norm_add_one
	MOVD ZR, (R8)
	B done

norm_add_one:
	MOVD R0, R8
	ADD $1, R2, R7
	CMP ZR, ZR // set C=1
norm_add_loop:
	MOVD (R8), R9
	ADCS ZR, R9
	MOVD.P R9, 8(R8)
	BCC done
	SUB $1, R7
	CBNZ R7, norm_add_loop

done:
	RET
