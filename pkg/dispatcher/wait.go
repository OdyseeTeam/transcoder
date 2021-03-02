package dispatcher

import (
	"context"
	"fmt"
	"time"
)

func WaitUntilTrue(ctx context.Context, between time.Duration, f func() bool) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out")
		default:
			if f() {
				return nil
			}
			time.Sleep(between)
		}
	}
}
