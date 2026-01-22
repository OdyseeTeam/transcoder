package diskmon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func mockStatfs(blocks uint64, bsize int64, bavail uint64) func(string, *unix.Statfs_t) error {
	return func(path string, stat *unix.Statfs_t) error {
		stat.Blocks = blocks
		stat.Bsize = bsize
		stat.Bavail = bavail
		return nil
	}
}

func mockStatfsError(err error) func(string, *unix.Statfs_t) error {
	return func(path string, stat *unix.Statfs_t) error {
		return err
	}
}

func mockStatfsSequence(usages []float64) func(string, *unix.Statfs_t) error {
	var idx int64
	return func(path string, stat *unix.Statfs_t) error {
		i := atomic.AddInt64(&idx, 1) - 1
		if int(i) >= len(usages) {
			i = int64(len(usages) - 1)
		}
		usage := usages[i]
		stat.Blocks = 1000
		stat.Bsize = 1024
		stat.Bavail = uint64((100 - usage) / 100 * 1000)
		return nil
	}
}

func TestGetDiskUsage(t *testing.T) {
	tests := []struct {
		name          string
		blocks        uint64
		bsize         int64
		bavail        uint64
		expectedUsage float64
		expectError   bool
	}{
		{
			name:          "empty disk",
			blocks:        1000,
			bsize:         1024,
			bavail:        1000,
			expectedUsage: 0,
		},
		{
			name:          "half full",
			blocks:        1000,
			bsize:         1024,
			bavail:        500,
			expectedUsage: 50,
		},
		{
			name:          "90 percent full",
			blocks:        1000,
			bsize:         1024,
			bavail:        100,
			expectedUsage: 90,
		},
		{
			name:          "completely full",
			blocks:        1000,
			bsize:         1024,
			bavail:        0,
			expectedUsage: 100,
		},
		{
			name:          "zero blocks",
			blocks:        0,
			bsize:         1024,
			bavail:        0,
			expectedUsage: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := statfsFunc
			defer func() { statfsFunc = original }()

			statfsFunc = mockStatfs(tt.blocks, tt.bsize, tt.bavail)

			usage, err := GetDiskUsage("/any/path")
			require.NoError(t, err)
			assert.InDelta(t, tt.expectedUsage, usage, 0.1)
		})
	}
}

func TestGetDiskUsageError(t *testing.T) {
	original := statfsFunc
	defer func() { statfsFunc = original }()

	statfsFunc = mockStatfsError(errors.New("statfs failed"))

	_, err := GetDiskUsage("/any/path")
	require.Error(t, err)
}

func TestWaitForDiskSpace(t *testing.T) {
	tests := []struct {
		name          string
		usageSequence []float64
		threshold     int
		maxWait       time.Duration
		checkInterval time.Duration
		cancelAfter   time.Duration
		expectError   bool
		expectTimeout bool
		expectCtxErr  bool
	}{
		{
			name:          "below threshold immediately",
			usageSequence: []float64{80.0},
			threshold:     90,
			maxWait:       time.Minute,
			checkInterval: 10 * time.Millisecond,
			expectError:   false,
		},
		{
			name:          "drops below after waiting",
			usageSequence: []float64{95.0, 92.0, 85.0},
			threshold:     90,
			maxWait:       time.Minute,
			checkInterval: 10 * time.Millisecond,
			expectError:   false,
		},
		{
			name:          "times out",
			usageSequence: []float64{95.0, 95.0, 95.0, 95.0, 95.0},
			threshold:     90,
			maxWait:       35 * time.Millisecond,
			checkInterval: 10 * time.Millisecond,
			expectError:   true,
			expectTimeout: true,
		},
		{
			name:          "context cancelled",
			usageSequence: []float64{95.0, 95.0, 95.0},
			threshold:     90,
			maxWait:       time.Minute,
			checkInterval: 10 * time.Millisecond,
			cancelAfter:   25 * time.Millisecond,
			expectError:   true,
			expectCtxErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := statfsFunc
			defer func() { statfsFunc = original }()

			statfsFunc = mockStatfsSequence(tt.usageSequence)

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.cancelAfter > 0 {
				ctx, cancel = context.WithCancel(ctx)
				time.AfterFunc(tt.cancelAfter, cancel)
			}

			cfg := Config{
				Enabled:       true,
				Path:          "/any/path",
				Threshold:     tt.threshold,
				CheckInterval: tt.checkInterval,
				MaxWait:       tt.maxWait,
			}

			err := WaitForDiskSpace(ctx, cfg, logging.NoopKVLogger{})

			if tt.expectError {
				require.Error(t, err)
				if tt.expectTimeout {
					assert.ErrorIs(t, err, ErrTimeout)
				}
				if tt.expectCtxErr {
					assert.ErrorIs(t, err, context.Canceled)
				}
			} else {
				require.NoError(t, err)
			}

			if cancel != nil {
				cancel()
			}
		})
	}
}

func TestWaitForDiskSpaceStatfsError(t *testing.T) {
	original := statfsFunc
	defer func() { statfsFunc = original }()

	statfsFunc = mockStatfsError(errors.New("statfs failed"))

	cfg := Config{
		Enabled:       true,
		Path:          "/any/path",
		Threshold:     90,
		CheckInterval: 10 * time.Millisecond,
		MaxWait:       time.Minute,
	}

	err := WaitForDiskSpace(context.Background(), cfg, logging.NoopKVLogger{})
	require.NoError(t, err, "should fail open on statfs error")
}
