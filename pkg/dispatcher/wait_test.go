package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitUntilTrue(t *testing.T) {
	var i, x int

	ctx, cancel1 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := WaitUntilTrue(ctx, 10*time.Millisecond, func() bool {
		if i > 5 {
			return true
		}
		i++
		return false
	})
	assert.NoError(t, err)
	cancel1()

	ctx, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = WaitUntilTrue(ctx, 10*time.Millisecond, func() bool {
		if x > 5 {
			return true
		}
		x++
		return false
	})
	cancel2()

	assert.EqualError(t, err, "timed out")
	assert.False(t, true)
}
