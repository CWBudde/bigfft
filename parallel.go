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
// It was measured, not guessed. BenchmarkMulFFTParallelSweep runs every
// operand size both ways in one binary; with the threshold disabled, on a
// 12th Gen i7-1255U at GOMAXPROCS=12, 14 interleaved repetitions gave
//
//	operand   transform array   parallel vs serial
//	 50 kbit         4096 w      +10.6%  (p=0.027)  slower
//	100 kbit         7168 w        ~     (p=0.70)   no difference
//	150 kbit        11776 w      -21.0%  (p<0.001)  faster
//	200 kbit        14848 w      -32.0%  (p<0.001)  faster
//
// so the crossover lies between 7k and 12k words. The threshold is set at the
// low end of that gap: everything measurably faster is admitted, and the one
// size that was measurably slower is excluded by a wide margin. Below it the
// transform is small enough that goroutine hand-off and the loss of a warm
// cache eat the gain.
const parallelWordThreshold = 8 << 10

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
