package bigfft

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// maxParallelism is the user-supplied worker cap. Zero, the default, means
// "as many workers as GOMAXPROCS". It is read on every Mul, so it may be
// changed at any time, including while other multiplications are running.
var maxParallelism atomic.Int64

// SetMaxParallelism limits the number of goroutines a single multiplication
// may use.
//
// A value of n <= 0 restores the default, which is runtime.GOMAXPROCS(0).
// A value of 1 makes the library fully serial: no goroutine is started and
// no worker scratch space is allocated. Values above GOMAXPROCS are honoured
// but rarely useful.
//
// Parallelism only ever changes the schedule, never the arithmetic: for any
// setting of n the result of Mul is bit-for-bit identical. Small operands are
// multiplied serially regardless of n, because the synchronisation costs more
// than the work it distributes; see parallelWorkers.
//
// SetMaxParallelism is safe to call concurrently with Mul, but a
// multiplication already in flight keeps the value it started with.
func SetMaxParallelism(n int) {
	if n < 0 {
		n = 0
	}
	maxParallelism.Store(int64(n))
}

// MaxParallelism reports the effective worker limit, that is, the largest
// number of goroutines a single multiplication may use. It resolves the
// default to runtime.GOMAXPROCS(0) rather than reporting zero.
func MaxParallelism() int {
	if n := int(maxParallelism.Load()); n > 0 {
		return n
	}
	if n := runtime.GOMAXPROCS(0); n > 0 {
		return n
	}
	return 1
}

// parallelWordThreshold is the size, in machine words of one transform array
// ((n+1)<<k), below which a multiplication stays serial.
//
// It was measured, not guessed. BenchmarkMulFFTParallelSweep runs every operand
// size both ways in one binary with the gate disabled on the parallel side; on
// a 12th Gen i7-1255U pinned to four P-cores at GOMAXPROCS=4, 30 interleaved
// repetitions gave
//
//	operand   transform array   parallel vs serial
//	 50 kbit         4096 w      +9.13%  slower
//	 75 kbit         5632 w      +3.14%  slower
//	100 kbit         7168 w      -2.05%  faster
//	125 kbit         8704 w      -5.59%
//	150 kbit        10240 w     -12.03%
//	200 kbit        13312 w     -16.05%
//
// (all p = 0.000), so the crossover lies between 5632 and 7168 words. Below it
// the transform is small enough that goroutine hand-off and the loss of a warm
// cache eat the gain.
//
// The value is nevertheless 8192 rather than 7168, and deliberately so: 8192
// words of transform array is 1792 words per operand, and fftThreshold is 1800,
// so everything a lower gate would admit sits below the size at which Mul enters
// the FFT at all. Lowering it would affect direct mulFFT callers only. The
// figures this comment carried until 2026 were also stale in the other
// direction — they predated fftSizeThreshold[8] going from 1<<18 to 1<<19, which
// moved 150 kbit operands from k=9 to k=8 and from 11776 to 10240 words.
//
// See BENCHMARKS.md § parallelWordThreshold, which also records the measurement
// that closed the "separate threshold for the parallel path" idea: a parallel
// FFT overtakes math/big at 115 kbit, and fftThreshold is 115.2 kbit.
//
// It is a var only so that the calibration harness can sweep it; production
// code never assigns to it.
var parallelWordThreshold = 8 << 10

// parallelWorkers reports how many workers a transform of 1<<k coefficients
// of n+1 words should use. It returns 1 when the problem is too small to be
// worth splitting, when the user asked for serial execution, or when there
// are fewer coefficients than workers.
func parallelWorkers(k uint, n int) int {
	w := MaxParallelism()
	if w <= 1 {
		return 1
	}
	if (n+1)<<k < parallelWordThreshold {
		return 1
	}
	if K := 1 << k; w > K {
		w = K
	}
	return w
}

// parMinSpawnSize is the smallest recursion size (log2 of the number of
// coefficients in the subtree) at which fourier still forks a goroutine for
// its second half. Below it both halves run in the calling goroutine so that
// the deep, cache-resident part of the recursion is not chopped up.
//
// The fork budget already halves at every level, so with w workers the split
// stops after log2(w) levels on its own: at most 4 levels for a 12-core
// machine. This floor only matters for the small-k, high-w corner, and 5
// (32 coefficients per subtree, times n+1 words each) keeps every forked
// subtree well above L2.
const parMinSpawnSize = 5

// parRange splits [0, n) into w contiguous chunks and runs f on each, chunk i
// in worker slot i, then waits for all of them. The last chunk runs in the
// calling goroutine, so only w-1 goroutines are started, and w == 1 starts
// none at all and calls f inline. Chunks are contiguous so that each worker
// walks its own stretch of memory forwards.
//
// f must confine itself to worker slot i's scratch space and to indices in
// [lo, hi): that is what makes the whole scheme safe.
func parRange(n, w int, f func(worker, lo, hi int)) {
	if w <= 1 || n <= 1 {
		f(0, 0, n)
		return
	}
	if w > n {
		w = n
	}
	var wg sync.WaitGroup
	wg.Add(w - 1)
	lo := 0
	for i := range w {
		hi := n * (i + 1) / w
		if i == w-1 {
			f(i, lo, hi)
		} else {
			go func(i, lo, hi int) {
				defer wg.Done()
				f(i, lo, hi)
			}(i, lo, hi)
		}
		lo = hi
	}
	wg.Wait()
}
