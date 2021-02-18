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
			fmt.Println("done")
			return fmt.Errorf("timed out")
		default:
			if f() {
				fmt.Println("success")
				return nil
			}
			fmt.Println("sleeping")
			time.Sleep(between)
		}
	}
}
