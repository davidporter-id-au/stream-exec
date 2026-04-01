package integration

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-17: high concurrency, all records processed with no duplicates or omissions
func TestHighConcurrencyAllRecordsProcessed(t *testing.T) {
	const count = 200
	var lines []string
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf(`{"id":%d}`, i))
	}

	r := run(t, strings.Join(lines, "\n"), "--exec", "echo $id", "--concurrency", "20")
	require.Equal(t, 0, r.exitCode)

	// collect every output line that is purely numeric
	seen := map[int]int{}
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		line = strings.TrimSpace(line)
		if n, err := strconv.Atoi(line); err == nil {
			seen[n]++
		}
	}

	for i := 0; i < count; i++ {
		assert.Equal(t, 1, seen[i], "id %d should appear exactly once, got %d", i, seen[i])
	}
}

// TC-18: concurrent execution is actually parallel (wall-clock check)
func TestConcurrencyIsActuallyParallel(t *testing.T) {
	const workers = 10
	const jobs = 20
	const sleepMs = 100

	var lines []string
	for i := 0; i < jobs; i++ {
		lines = append(lines, fmt.Sprintf(`{"i":%d}`, i))
	}

	start := time.Now()
	r := run(t,
		strings.Join(lines, "\n"),
		"--exec", fmt.Sprintf("sleep 0.%d && echo $i", sleepMs/10),
		"--concurrency", strconv.Itoa(workers),
	)
	elapsed := time.Since(start)

	require.Equal(t, 0, r.exitCode)

	// With 10 workers and 20 jobs of 100ms each, wall time should be ~200ms.
	// Serial would be 2000ms. Allow generous 3x headroom for slow CI.
	serialTime := time.Duration(jobs*sleepMs) * time.Millisecond
	assert.Less(t, elapsed, serialTime/3,
		"expected parallel execution (~%dms), got %s — may be running serially",
		int(serialTime.Milliseconds())/workers, elapsed)
}

// TC-19: output order is not guaranteed with concurrency > 1
// This test documents that ordering is non-deterministic. It verifies all results
// are present, but does NOT require them to be in input order.
func TestConcurrentOutputOrderIsNonDeterministic(t *testing.T) {
	const count = 50
	var lines []string
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf(`{"seq":%d}`, i))
	}

	r := run(t, strings.Join(lines, "\n"), "--exec", "echo $seq", "--concurrency", "10")
	require.Equal(t, 0, r.exitCode)

	var got []int
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		line = strings.TrimSpace(line)
		if n, err := strconv.Atoi(line); err == nil {
			got = append(got, n)
		}
	}
	require.Len(t, got, count, "expected %d output lines", count)

	// All values should be present
	sorted := make([]int, len(got))
	copy(sorted, got)
	sort.Ints(sorted)
	for i, v := range sorted {
		assert.Equal(t, i, v)
	}
}

// TC-20: concurrency=1 produces output in input order
func TestSerialConcurrencyPreservesOrder(t *testing.T) {
	const count = 20
	var lines []string
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf(`{"seq":%d}`, i))
	}

	r := run(t, strings.Join(lines, "\n"), "--exec", "echo $seq", "--concurrency", "1")
	require.Equal(t, 0, r.exitCode)

	var got []int
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		line = strings.TrimSpace(line)
		if n, err := strconv.Atoi(line); err == nil {
			got = append(got, n)
		}
	}
	require.Len(t, got, count)
	for i, v := range got {
		assert.Equal(t, i, v, "line %d: expected %d got %d", i, i, v)
	}
}
