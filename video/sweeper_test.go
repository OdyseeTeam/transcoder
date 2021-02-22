package video

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSweeperInc(t *testing.T) {
	s := NewSweeper()
	var wg sync.WaitGroup
	for range [100]int{} {
		go func() {
			wg.Add(1)
			for range [100]int{} {
				s.Inc("abc")
			}
			wg.Done()
		}()
	}
	wg.Wait()

	require.Len(t, s.Top(1, false), 1)
	assert.EqualValues(t, uint64(100*100), s.Top(1, true)[0].Count)

	require.Len(t, s.Top(1, false), 0)
}
