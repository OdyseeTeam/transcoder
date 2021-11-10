package encoder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lbryio/transcoder/formats"
)

func TestNewArguments(t *testing.T) {
	var (
		out = t.TempDir()
		fps = 30

		defaultFormats = []formats.Format{
			{
				Resolution: formats.SD144,
				Bitrate:    formats.Bitrate{FPS30: 100, FPS60: 160},
			},
		}
	)

	tests := []struct {
		name          string
		targetType    TargetType
		targetFormats []formats.Format
		wantArgs      Arguments
		wantErr       bool
	}{
		{
			"HLS",
			TargetTypeHLS, defaultFormats,
			HLSArguments(),
			false,
		},
		{
			"TS",
			TargetTypeTS, defaultFormats,
			TSArguments(),
			false,
		},
		{
			"DefaultToHLS",
			TargetTypeUnknown, defaultFormats,
			HLSArguments(),
			false,
		},
		{
			"EmptyFormats",
			TargetTypeUnknown, nil,
			HLSArguments(),
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := Target{test.targetFormats, test.targetType}
			gotArgs, gotErr := NewArguments(out, target, fps)
			if test.wantErr {
				assert.NotNil(t, gotErr)
			} else {
				assert.Nil(t, gotErr)
				assert.Equal(t, test.wantArgs.defaultArgs, gotArgs.defaultArgs)
			}
		})
	}
}
