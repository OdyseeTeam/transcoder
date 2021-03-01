package video

import (
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSweeperInc(t *testing.T) {
	s := NewSweeper()
	ids := [][2]string{{"abc", "asdasda"}, {"cde", "sdagkkj"}}

	var wg sync.WaitGroup
	for range [100]int{} {
		wg.Add(1)
		go func() {
			wg.Add(1)
			for range [100]int{} {
				s.Inc(ids[0][0], ids[0][1])
			}
			wg.Done()
		}()
		go func() {
			wg.Add(1)
			for range [250]int{} {
				s.Inc(ids[1][0], ids[1][1])
			}
			wg.Done()
		}()
		wg.Done()
	}
	wg.Wait()

	top := s.Top(2, 0)
	require.Len(t, top, 2)
	assert.EqualValues(t, uint64(100*250), top[0].Count)
	assert.EqualValues(t, uint64(100*100), top[1].Count)
	assert.Equal(t, ids[1][0], top[0].URL)
	assert.Equal(t, ids[1][1], top[0].SDHash)
	assert.Equal(t, ids[0][0], top[1].URL)
	assert.Equal(t, ids[0][1], top[1].SDHash)
}

func TestSweeperSweep(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	s := NewSweeper()
	ids := [][2]string{{"abc", "asdasda"}, {"cde", "sdagkkj"}, {"def", "asuyuia"}, {"ghi", "ewury"}}

	var wg sync.WaitGroup
	for range [100]int{} {
		go func() {
			wg.Add(1)
			for range [100]int{} {
				i := rand.Intn(len(ids))
				s.Inc(ids[i][0], ids[i][1])
			}
			wg.Done()
		}()
	}
	wg.Wait()

	require.Len(t, s.Top(3, 0), 3)
	require.Len(t, s.Top(5, 0), 4)

	counts := []int{}
	for _, i := range s.Top(4, 0) {
		counts = append(counts, int(i.Count))
	}
	assert.True(t, sort.IsSorted(sort.Reverse(sort.IntSlice(counts))))

	s.Sweep(s.Top(3, 0))
	assert.Len(t, s.Top(5, 0), 1)
}
