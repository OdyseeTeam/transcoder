package video

import (
	"strings"

	"github.com/lbryio/transcoder/pkg/claim"
)

var enabledChannels = []string{}

func LoadEnabledChannels(channels []string) {
	enabledChannels = channels
	logger.Infow("loaded enabled channels", "count", len(enabledChannels))
}

// ValidateIncomingVideo checks if supplied video can be accepted for processing.
func ValidateIncomingVideo(uri string) (*claim.Claim, error) {
	// Validate here if video exists on LBRY (resolve)
	// Validate here if video is in the whitelist (claim.signing_channel)
	c, err := claim.Resolve(uri)
	if err != nil {
		return nil, err
	}
	return c, ValidateByClaim(c)
}

func ValidateByClaim(c *claim.Claim) error {
	channelEnabled := false
	ll := logger.With("canonical_url", c.CanonicalURL)
	if c.SigningChannel == nil {
		ll.Debug("missing signing channel")
		return ErrNoSigningChannel
	}
	for _, cn := range enabledChannels {
		if strings.ToLower(c.SigningChannel.CanonicalURL) == strings.ToLower("lbry://"+cn) {
			channelEnabled = true
			break
		}
	}
	if !channelEnabled {
		ll.Debugw("channel transcoding not enabled", "channel", c.SigningChannel.CanonicalURL)
		return ErrChannelNotEnabled
	}
	ll.Debug("channel transcoding enabled")
	return nil
}
