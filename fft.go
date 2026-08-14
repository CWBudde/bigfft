// Package bigfft implements multiplication of big.Int using FFT.
//
// The implementation is based on the Schönhage-Strassen method
// using integer FFT modulo 2^n+1.
package bigfft

import (
	"math/big"
	"sync"
	"unsafe"
)

const _W = int(unsafe.Sizeof(big.Word(0)) * 8)

type nat []big.Word

func (n nat) String() string {
	v := new(big.Int)
	v.SetBits(n)
	return v.String()
}

// fftThreshold is the size (in words) above which FFT is used over
// Karatsuba from math/big.
//
// TestCalibrate seems to indicate a threshold of 60kbits on 32-bit
// arches and 110kbits on 64-bit arches.
var fftThreshold = 1800

// Mul computes the product x*y and returns z.
// It can be used instead of the Mul method of
// *big.Int from math/big package.
func Mul(x, y *big.Int) *big.Int {
	xwords := len(x.Bits())
	ywords := len(y.Bits())
	if xwords > fftThreshold && ywords > fftThreshold {
		return mulFFT(x, y)
	}
	return new(big.Int).Mul(x, y)
}

func mulFFT(x, y *big.Int) *big.Int {
	var xb, yb nat = x.Bits(), y.Bits()
	zb := fftmul(xb, yb)
	z := new(big.Int)
	z.SetBits(zb)
	if x.Sign()*y.Sign() < 0 {
		z.Neg(z)
	}
	return z
}

// A FFT size of K=1<<k is adequate when K is about 2*sqrt(N) where
// N = x.Bitlen() + y.Bitlen().

func fftmul(x, y nat) nat {
	k, m := fftSize(x, y)
	xp := polyFromNat(x, k, m)
	yp := polyFromNat(y, k, m)
	// A single scratch arena is allocated here and threaded through every
	// stage of the transform. It dies when fftmul returns: the nat produced
	// by Int() is freshly allocated and does not alias it (see poly.Int).
	s := newFFTScratch(k, valueSize(k, m, 2))
	rp := xp.mul(&yp, s)
	return rp.Int()
}

// fourierTemps is the pair of temporary registers needed by one running
// instance of the fourier recursion. It is passed in by the caller rather
// than allocated inside fourier so that a parallel implementation can hand
// a private pair to each worker goroutine.
type fourierTemps struct {
	tmp  fermat
	tmp2 fermat
}

// fftScratch is the working memory of a single fftmul call.
//
// Layout, with N = n+1 words per coefficient and K = 1<<k coefficients.
// Three word regions of N*K words each, plus a handful of small buffers,
// are carved out of one backing array:
//
//	region a  (N*K)  input staging for the forward transform of p, then
//	                 (after that transform has run) the same for q, then
//	                 the destination of the inverse transform.
//	region b  (N*K)  values of the forward transform of p, then, in place,
//	                 the pointwise product p*q.
//	region c  (N*K)  values of the forward transform of q.
//	u    (N)         untwisting register of the inverse transform.
//	fts  (W*2*N)     one tmp/tmp2 pair per worker for the fourier recursion.
//	mbufs(W*8*n)     one destination scratch for fermat.Mul per worker, used
//	                 by polValues.mul.
//
// W is the worker count chosen by parallelWorkers for this problem size; it
// is 1 whenever the library is running serially, in which case the layout is
// exactly the serial one. ft and mbuf name worker 0's regions and share their
// storage with fts[0] and mbufs[0].
//
// Why the sharing above is safe:
//
//   - The forward transform reads its input array only inside fourier and
//     never again, so region a is dead the moment Transform returns. The
//     two operand transforms are independent, hence they can stage into
//     the same region one after the other, and the inverse transform,
//     which runs strictly later, can use it as its output.
//   - The pointwise product is computed index by index into mbuf and only
//     then copied over p's values, so writing the result on top of region
//     b never clobbers an operand that is still needed: iteration i reads
//     only index i of each operand.
//   - The poly returned by the inverse transform aliases region a. It is
//     consumed by poly.Int, which accumulates into a freshly made nat and
//     returns a prefix of it (see trim), so the value handed back by
//     fftmul is disjoint from the arena and the arena may be collected.
//
// Header arrays are carved the same way: one []fermat of 4*K supplies the
// staging, p-values, q-values and product-values headers, and one []nat of
// K supplies the coefficient headers of the reconstructed polynomial.
type fftScratch struct {
	k uint
	n int
	w int // number of workers this arena is dimensioned for

	a, b, c []big.Word

	hIn []fermat // headers over region a
	hP  []fermat // headers over region b
	hQ  []fermat // headers over region c
	hR  []fermat // headers over region b (product, in place over hP)
	hA  []nat    // headers over region a, for the reconstructed poly

	u fermat

	fts   []fourierTemps // W pairs; fts[0] is ft
	mbufs []fermat       // W buffers of 8*n words; mbufs[0] is mbuf

	ft   fourierTemps // worker 0's pair, aliases fts[0]
	mbuf fermat       // worker 0's buffer, aliases mbufs[0]
}

func newFFTScratch(k uint, n int) *fftScratch {
	K := 1 << k
	N := n + 1
	W := parallelWorkers(k, n)
	s := &fftScratch{k: k, n: n, w: W}

	// Every sub-slice below is capacity-bounded. This is not just hygiene:
	// math/big decides whether it may reuse a destination buffer with a
	// test on the address of the last word of its capacity, so leaving the
	// capacities running to the end of the arena would make big.Int.Mul
	// believe mbuf aliases its operands and allocate a fresh buffer for
	// every pointwise product.
	w := make([]big.Word, 3*N*K+N+W*2*N+W*8*n)
	s.a, w = w[:N*K:N*K], w[N*K:]
	s.b, w = w[:N*K:N*K], w[N*K:]
	s.c, w = w[:N*K:N*K], w[N*K:]
	s.u, w = fermat(w[:N:N]), w[N:]
	s.fts = make([]fourierTemps, W)
	for i := range s.fts {
		s.fts[i].tmp, w = fermat(w[:N:N]), w[N:]
		s.fts[i].tmp2, w = fermat(w[:N:N]), w[N:]
	}
	s.mbufs = make([]fermat, W)
	for i := range s.mbufs {
		s.mbufs[i], w = fermat(w[:8*n:8*n]), w[8*n:]
	}
	s.ft, s.mbuf = s.fts[0], s.mbufs[0]

	f := make([]fermat, 4*K)
	s.hIn, s.hP, s.hQ, s.hR = f[0:K], f[K:2*K], f[2*K:3*K], f[3*K:4*K]
	for i := 0; i < K; i++ {
		lo, hi := i*N, (i+1)*N
		s.hIn[i] = fermat(s.a[lo:hi:hi])
		s.hP[i] = fermat(s.b[lo:hi:hi])
		s.hQ[i] = fermat(s.c[lo:hi:hi])
		s.hR[i] = fermat(s.b[lo:hi:hi])
	}
	s.hA = make([]nat, K)
	return s
}

// zeroWords clears x. The compiler turns this loop into a memclr.
func zeroWords(x []big.Word) {
	for i := range x {
		x[i] = 0
	}
}

// fftSizeThreshold[i] is the maximal size (in bits) where we should use
// fft size i.
//
// Entry 8 was raised from 1<<18 to 1<<19 in 2026: at the old boundary an FFT
// of length 1<<8 beat the 1<<9 the table switched to by 20%, and the two only
// reached parity at twice that size. It is the lowest entry the public Mul can
// select, and moving it is worth -22% at 150 kbit per operand. The entries
// below it show the same shape but lie entirely under fftThreshold, so Mul
// never selects them; the ones above oscillate and were left alone. See
// BENCHMARKS.md.
var fftSizeThreshold = [...]int64{
	0, 0, 0,
	4 << 10, 8 << 10, 16 << 10, // 5
	32 << 10, 64 << 10, 1 << 19, 1 << 20, 3 << 20, // 10
	8 << 20, 30 << 20, 100 << 20, 300 << 20, 600 << 20,
}

// returns the FFT length k, m the number of words per chunk
// such that m << k is larger than the number of words
// in x*y.
func fftSize(x, y nat) (k uint, m int) {
	words := len(x) + len(y)
	bits := int64(words) * int64(_W)
	k = uint(len(fftSizeThreshold))
	for i := range fftSizeThreshold {
		if fftSizeThreshold[i] > bits {
			k = uint(i)
			break
		}
	}
	// The 1<<k chunks of m words must have N bits so that
	// 2^N-1 is larger than x*y. That is, m<<k > words
	m = words>>k + 1
	return
}

// valueSize returns the length (in words) to use for polynomial
// coefficients, to compute a correct product of polynomials P*Q
// where deg(P*Q) < K (== 1<<k) and where coefficients of P and Q are
// less than b^m (== 1 << (m*_W)).
// The chosen length (in bits) must be a multiple of 1 << (k-extra).
func valueSize(k uint, m int, extra uint) int {
	// The coefficients of P*Q are less than b^(2m)*K
	// so we need W * valueSize >= 2*m*W+K
	n := 2*m*_W + int(k) // necessary bits
	K := 1 << (k - extra)
	if K < _W {
		K = _W
	}
	n = ((n / K) + 1) * K // round to a multiple of K
	return n / _W
}

// poly represents an integer via a polynomial in Z[x]/(x^K+1)
// where K is the FFT length and b^m is the computation basis 1<<(m*_W).
// If P = a[0] + a[1] x + ... a[n] x^(K-1), the associated natural number
// is P(b^m).
type poly struct {
	k uint  // k is such that K = 1<<k.
	m int   // the m such that P(b^m) is the original number.
	a []nat // a slice of at most K m-word coefficients.
}

// polyFromNat slices the number x into a polynomial
// with 1<<k coefficients made of m words.
func polyFromNat(x nat, k uint, m int) poly {
	p := poly{k: k, m: m}
	length := len(x)/m + 1
	p.a = make([]nat, length)
	for i := range p.a {
		if len(x) < m {
			p.a[i] = make(nat, m)
			copy(p.a[i], x)
			break
		}
		p.a[i] = x[:m]
		x = x[m:]
	}
	return p
}

// Int evaluates back a poly to its integer value.
func (p *poly) Int() nat {
	length := len(p.a)*p.m + 1
	if na := len(p.a); na > 0 {
		length += len(p.a[na-1])
	}
	n := make(nat, length)
	m := p.m
	np := n
	for i := range p.a {
		l := len(p.a[i])
		c := addVV(np[:l], np[:l], p.a[i])
		if np[l] < ^big.Word(0) {
			np[l] += c
		} else {
			addVW(np[l:], np[l:], c)
		}
		np = np[m:]
	}
	n = trim(n)
	return n
}

func trim(n nat) nat {
	for i := range n {
		if n[len(n)-1-i] != 0 {
			return n[:len(n)-i]
		}
	}
	return nil
}

// Mul multiplies p and q modulo X^K-1, where K = 1<<p.k.
// The product is done via a Fourier transform.
func (p *poly) Mul(q *poly) poly {
	// extra=2 because:
	// * some power of 2 is a K-th root of unity when n is a multiple of K/2.
	// * 2 itself is a square (see fermat.ShiftHalf)
	return p.mul(q, newFFTScratch(p.k, valueSize(p.k, p.m, 2)))
}

// mul is Mul using a caller-supplied scratch arena.
func (p *poly) mul(q *poly, s *fftScratch) poly {
	n := s.n
	if s.k != p.k || n != valueSize(p.k, p.m, 2) {
		panic("bigfft: scratch arena does not match poly")
	}

	// The two forward transforms are mathematically independent, but they
	// are not independent here: both stage their input into region a
	// (s.hIn) before transforming out of it. Running them concurrently
	// would need a second N*K staging region, a 33% increase in the working
	// set, to buy at most a factor of two. The transforms are instead
	// parallelised internally (see fourierTmp), which needs no extra
	// storage and scales past two workers, so they stay sequential here.
	pv := p.transform(n, s.hIn, s.hP, s.fts)
	qv := q.transform(n, s.hIn, s.hQ, s.fts)
	rv := pv.mul(&qv, s.hR, s.mbufs)
	r := rv.invTransform(s.hIn, s.hA, s.u, s.fts)
	r.m = p.m
	return r
}

// A polValues represents the value of a poly at the powers of a
// K-th root of unity θ=2^(l/2) in Z/(b^n+1)Z, where b^n = 2^(K/4*l).
type polValues struct {
	k      uint     // k is such that K = 1<<k.
	n      int      // the length of coefficients, n*_W a multiple of K/4.
	values []fermat // a slice of K (n+1)-word values
}

// Transform evaluates p at θ^i for i = 0...K-1, where
// θ is a K-th primitive root of unity in Z/(b^n+1)Z.
func (p *poly) Transform(n int) polValues {
	s := newFFTScratch(p.k, n)
	return p.transform(n, s.hIn, s.hP, s.fts)
}

// transform is Transform using caller-supplied buffers: input is staging
// space, clobbered and dead on return, and values receives the result.
// Both hold 1<<p.k coefficients of n+1 words. fts holds one temporary pair
// per worker; its length sets the degree of parallelism.
func (p *poly) transform(n int, input, values []fermat, fts []fourierTemps) polValues {
	k := p.k
	// Staging is a pure memory pass over N*K words, one coefficient per
	// index and no sharing between indices, so it shards trivially.
	parRange(len(input), len(fts), func(_, lo, hi int) {
		for i := lo; i < hi; i++ {
			if i < len(p.a) {
				c := copy(input[i], p.a[i])
				zeroWords(input[i][c:])
			} else {
				zeroWords(input[i])
			}
		}
	})
	// Now compute q(ω^i) for i = 0 ... K-1. fourier writes every word of
	// values, so it needs no clearing.
	fourierTmp(values, input, false, n, k, fts)
	return polValues{k, n, values}
}

// InvTransform reconstructs p (modulo X^K - 1) from its
// values at θ^i for i = 0..K-1.
func (v *polValues) InvTransform() poly {
	s := newFFTScratch(v.k, v.n)
	return v.invTransform(s.hIn, s.hA, s.u, s.fts)
}

// invTransform is InvTransform using caller-supplied buffers. p receives
// the inverse transform (and must not overlap v.values), a receives the
// coefficient headers of the result, which alias p, and u is a scratch
// register of n+1 words disjoint from p.
func (v *polValues) invTransform(p []fermat, a []nat, u fermat, fts []fourierTemps) poly {
	k, n := v.k, v.n

	// Perform an inverse Fourier transform to recover p. fourier writes
	// every word of p, so it needs no clearing.
	fourierTmp(p, v.values, true, n, k, fts)
	// Divide by K, and untwist q to recover p. Index i touches only p[i],
	// so the loop shards; each shard needs its own n+1 word register, and
	// the fourier temporaries are free again by now, so worker j borrows
	// fts[j].tmp rather than claiming more arena.
	parRange(len(p), len(fts), func(w, lo, hi int) {
		reg := u
		if w > 0 {
			reg = fts[w].tmp
		}
		for i := lo; i < hi; i++ {
			reg.Shift(p[i], -int(k))
			copy(p[i], reg)
			a[i] = nat(p[i])
		}
	})
	return poly{k: k, m: 0, a: a}
}

// NTransform evaluates p at θω^i for i = 0...K-1, where
// θ is a (2K)-th primitive root of unity in Z/(b^n+1)Z
// and ω = θ².
func (p *poly) NTransform(n int) polValues {
	k := p.k
	if len(p.a) >= 1<<k {
		panic("Transform: len(p.a) >= 1<<k")
	}
	// θ is represented as a shift.
	θshift := (n * _W) >> k
	// p(x) = a_0 + a_1 x + ... + a_{K-1} x^(K-1)
	// p(θx) = q(x) where
	// q(x) = a_0 + θa_1 x + ... + θ^(K-1) a_{K-1} x^(K-1)
	//
	// Twist p by θ to obtain q.
	tbits := make([]big.Word, (n+1)<<k)
	twisted := make([]fermat, 1<<k)
	src := make(fermat, n+1)
	for i := range twisted {
		twisted[i] = fermat(tbits[i*(n+1) : (i+1)*(n+1)])
		if i < len(p.a) {
			for i := range src {
				src[i] = 0
			}
			copy(src, p.a[i])
			twisted[i].Shift(src, θshift*i)
		}
	}

	// Now computed q(ω^i) for i = 0 ... K-1
	valbits := make([]big.Word, (n+1)<<k)
	values := make([]fermat, 1<<k)
	for i := range values {
		values[i] = fermat(valbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(values, twisted, false, n, k)
	return polValues{k, n, values}
}

// InvTransform reconstructs a polynomial from its values at
// roots of x^K+1. The m field of the returned polynomial
// is unspecified.
func (v *polValues) InvNTransform() poly {
	k := v.k
	n := v.n
	θshift := (n * _W) >> k

	// Perform an inverse Fourier transform to recover q.
	qbits := make([]big.Word, (n+1)<<k)
	q := make([]fermat, 1<<k)
	for i := range q {
		q[i] = fermat(qbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(q, v.values, true, n, k)

	// Divide by K, and untwist q to recover p.
	u := make(fermat, n+1)
	a := make([]nat, 1<<k)
	for i := range q {
		u.Shift(q[i], -int(k)-i*θshift)
		copy(q[i], u)
		a[i] = nat(q[i])
	}
	return poly{k: k, m: 0, a: a}
}

// fourier performs an unnormalized Fourier transform
// of src, a length 1<<k vector of numbers modulo b^n+1
// where b = 1<<_W.
func fourier(dst []fermat, src []fermat, backward bool, n int, k uint) {
	fts := []fourierTemps{{tmp: make(fermat, n+1), tmp2: make(fermat, n+1)}}
	fourierTmp(dst, src, backward, n, k, fts)
}

// fourierTmp is fourier using caller-supplied temporaries. len(fts) is the
// number of workers the transform may use, and each worker needs its own
// pair, which is why they are not allocated here. len(fts) == 1 is the
// serial transform: no goroutine is started.
//
// Parallelism here is purely a schedule. Both the recursion split and the
// butterfly loop below partition their outputs into disjoint index ranges
// and read their inputs without writing them, so every word of dst is
// computed by the same code from the same values as in the serial case, and
// the result is bit-identical.
func fourierTmp(dst []fermat, src []fermat, backward bool, n int, k uint, fts []fourierTemps) {
	var rec func(dst, src []fermat, size uint, fts []fourierTemps)

	// The recursion function of the FFT.
	// The root of unity used in the transform is ω=1<<(ω2shift/2).
	// The source array may use shifted indices (i.e. the i-th
	// element is src[i << idxShift]).
	rec = func(dst, src []fermat, size uint, fts []fourierTemps) {
		idxShift := k - size
		ω2shift := (4 * n * _W) >> size
		if backward {
			ω2shift = -ω2shift
		}

		// Easy cases.
		if len(src[0]) != n+1 || len(dst[0]) != n+1 {
			panic("len(src[0]) != n+1 || len(dst[0]) != n+1")
		}
		switch size {
		case 0:
			copy(dst[0], src[0])
			return
		case 1:
			dst[0].Add(src[0], src[1<<idxShift]) // dst[0] = src[0] + src[1]
			dst[1].Sub(src[0], src[1<<idxShift]) // dst[1] = src[0] - src[1]
			return
		}

		// Let P(x) = src[0] + src[1<<idxShift] * x + ... + src[K-1 << idxShift] * x^(K-1)
		// The P(x) = Q1(x²) + x*Q2(x²)
		// where Q1's coefficients are src with indices shifted by 1
		// where Q2's coefficients are src[1<<idxShift:] with indices shifted by 1

		// Split destination vectors in halves.
		// Below the cutoff the subtree is deep enough to be worth keeping
		// in one goroutine and one cache: drop the worker budget so that
		// neither the split nor the butterfly loop below distributes it.
		if size < parMinSpawnSize && len(fts) > 1 {
			fts = fts[:1]
		}

		dst1 := dst[:1<<(size-1)]
		dst2 := dst[1<<(size-1):]
		// Transform Q1 and Q2 in the halves. The two subtrees write to
		// disjoint halves of dst and only read src, so they may run
		// concurrently; each is handed half of the worker budget, which
		// stops the forking after log2(len(fts)) levels all by itself.
		if len(fts) >= 2 && size >= parMinSpawnSize {
			h := len(fts) / 2
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec(dst2, src[1<<idxShift:], size-1, fts[h:])
			}()
			rec(dst1, src, size-1, fts[:h])
			wg.Wait()
		} else {
			rec(dst1, src, size-1, fts)
			rec(dst2, src[1<<idxShift:], size-1, fts)
		}

		// Reconstruct P's transform from transforms of Q1 and Q2.
		// dst[i]            is dst1[i] + ω^i * dst2[i]
		// dst[i + 1<<(k-1)] is dst1[i] + ω^(i+K/2) * dst2[i]
		//
		// Iteration i touches only dst1[i] and dst2[i], so this loop
		// shards across the worker budget as well. That matters: the
		// butterfly work is the same at every level of the recursion, so
		// leaving the top levels serial would cap the speedup at about
		// k/(2+log2 w) no matter how many cores are available.
		//
		// The serial case calls butterflies directly. Going through a
		// closure instead would allocate one closure per recursion node —
		// 765 allocations per 200kbit multiplication, measured — for no
		// gain, since there is nothing to distribute.
		if len(fts) == 1 {
			butterflies(dst1, dst2, 0, len(dst1), ω2shift, &fts[0])
			return
		}
		parRange(len(dst1), len(fts), func(w, lo, hi int) {
			butterflies(dst1, dst2, lo, hi, ω2shift, &fts[w])
		})
	}
	rec(dst, src, k, fts)
}

// butterflies reconstructs dst1[i] and dst2[i] from the transforms of the two
// halves, for i in [lo, hi). Iterations are independent, so a shard of the
// range may run in its own goroutine as long as it brings its own temporaries.
func butterflies(dst1, dst2 []fermat, lo, hi, ω2shift int, ft *fourierTemps) {
	tmp, tmp2 := ft.tmp, ft.tmp2
	for i := lo; i < hi; i++ {
		tmp.ShiftHalf(dst2[i], i*ω2shift, tmp2) // ω^i * dst2[i]
		butterfly(dst1[i], dst2[i], dst1[i], tmp)
	}
}

// Mul returns the pointwise product of p and q.
func (p *polValues) Mul(q *polValues) (r polValues) {
	n := p.n
	values := make([]fermat, len(p.values))
	bits := make([]big.Word, len(p.values)*(n+1))
	for i := range values {
		values[i] = fermat(bits[i*(n+1) : (i+1)*(n+1)])
	}
	return p.mul(q, values, []fermat{make(fermat, 8*n)})
}

// mul is Mul writing into caller-supplied storage. values may alias
// p.values (each product is computed into bufs[w] first, and iteration i
// reads only index i of the operands), but must not alias any of the bufs.
//
// The K products are independent, so the loop is sharded across len(bufs)
// workers, one 8n-word destination scratch each. Contiguous shards keep each
// worker on its own stretch of the values arrays.
func (p *polValues) mul(q *polValues, values []fermat, bufs []fermat) (r polValues) {
	r.k, r.n = p.k, p.n
	r.values = values
	parRange(len(values), len(bufs), func(w, lo, hi int) {
		buf := bufs[w]
		for i := lo; i < hi; i++ {
			z := buf.Mul(p.values[i], q.values[i])
			copy(values[i], z)
		}
	})
	return
}
