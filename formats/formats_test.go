package formats

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetFormats(t *testing.T) {
	testInputs := []struct {
		w, h   int
		target []Format
	}{
		{720, 480, []Format{H264.Format(SD480), H264.Format(SD360), H264.Format(SD240)}},
		{1920, 1080, []Format{H264.Format(HD1080), H264.Format(HD720), H264.Format(SD480), H264.Format(SD360), H264.Format(SD240)}},
		{800, 600, []Format{H264.Format(SD480), H264.Format(SD360), H264.Format(SD240), H264.Format(Resolution{800, 600})}},
	}

	for _, ti := range testInputs {
		t.Run(fmt.Sprintf("%vx%v", ti.w, ti.h), func(t *testing.T) {
			require.Equal(t, ti.target, TargetFormats(H264, ti.w, ti.h))
		})
	}
}
