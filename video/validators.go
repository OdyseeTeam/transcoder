package video

import "github.com/lbryio/transcoder/pkg/claim"

var enabledChannels = []string{
	"lbry://@davidpakman#7",
	"lbry://@specialoperationstest#3",
	"lbry://@EarthTitan#0",
}

// ValidateIncomingVideo checks if supplied video can be accepted for processing.
func ValidateIncomingVideo(uri string) (*claim.Claim, error) {
	// Validate here if video exists on LBRY (resolve)
	// Validate here if video is in the whitelist (claim.signing_channel)
	c, err := claim.Resolve(uri)
	if err != nil {
		return nil, err
	}
	channelEnabled := false
	ll := logger.With("canonical_url", c.CanonicalURL)
	if c.SigningChannel == nil {
		ll.Debug("missing signing channel")
		return nil, ErrNoSigningChannel
	}
	for _, cn := range enabledChannels {
		if c.SigningChannel.CanonicalURL == cn {
			channelEnabled = true
			break
		}
	}
	if !channelEnabled {
		ll.Debugw("channel transcoding not enabled", "channel", c.SigningChannel.CanonicalURL)
		return nil, ErrChannelNotEnabled
	}
	ll.Debug("channel transcoding enabled")
	return c, nil
}
