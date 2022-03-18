package resolve

import "errors"

var (
	ErrTranscodingUnderway  = errors.New("transcoding is in progress")
	ErrTranscodingQueued    = errors.New("transcoding queued")
	ErrTranscodingForbidden = errors.New("transcoding is disabled for this channel")
	ErrChannelNotEnabled    = errors.New("transcoding is not enabled for this channel")

	ErrClaimNotFound    = errors.New("could not resolve stream URI")
	ErrNoSigningChannel = errors.New("no signing channel for stream")
)
