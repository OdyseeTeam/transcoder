package video

var enabledChannels = []string{
	"lbry://@davidpakman#7",
	"lbry://@specialoperationstest#3",
}

// ValidateIncomingVideo checks if supplied video can be accepted for processing.
func ValidateIncomingVideo(uri string) (*Claim, error) {
	// Validate here if video exists on LBRY (resolve)
	// Validate here if video is in the whitelist (claim.signing_channel)
	c, err := Resolve(uri)
	if err != nil {
		return nil, err
	}
	channelEnabled := false
	for _, cn := range enabledChannels {
		if c.SigningChannel.CanonicalURL == cn {
			channelEnabled = true
			break
		}
	}
	if !channelEnabled {
		return nil, ErrChannelNotEnabled
	}
	return c, nil
}
