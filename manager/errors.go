package manager

import "errors"

var (
	ErrTranscodingUnderway  = errors.New("transcoding is in progress")
	ErrTranscodingQueued    = errors.New("transcoding queued")
	ErrTranscodingForbidden = errors.New("transcoding this stream is not possible at this time")
	ErrTranscodingDisabled  = errors.New("transcoding is disabled for this stream")

	ErrChannelNotEnabled = errors.New("transcoding is not enabled for this channel")
	ErrStreamNotFound    = errors.New("could not resolve stream URI")
	ErrNoSigningChannel  = errors.New("no signing channel for stream")
)
