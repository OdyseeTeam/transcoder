package video

import "errors"

var (
	ErrTranscodingUnderway = errors.New("transcoding in progress")
	ErrChannelNotEnabled   = errors.New("transcoding was not enabled for this channel")
)
