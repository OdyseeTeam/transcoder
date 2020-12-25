package video

import (
	"strings"

	"github.com/lbryio/transcoder/pkg/claim"
)

var enabledChannels = []string{
	"@davidpakman#7",
	"@specialoperationstest#3",
	"@EarthTitan#0",
	"@deqodeurs#8",
	"@Vivresainement#f",
	"@SaltyCracker#a",
	"@filsdepangolin#e",
	"@SilvanoTrotta#f",
	"@eevblog#7",
	"@DollarVigilante#b",
	"@radio-quebec#a",
	"@NiceChord#5",
	"@AgoraTVNEWS#5",
	"@samueleckert#4",
	"@TranslatedPressDE#b",
	"@oliverjanich#b",
	"@NTDFrancais#5",
	"@corbettreport#0",
	"@Miniver#4",
	"@Bombards_Body_Language#f",
	"@sarahwestall#0",
	"@kouki#2",
	"@LBRY-Español#8",
	"@thecrowhouse#2",
	"@bitcoin#9f",
	"@timcast#c9",
	"@Styxhexenhammer666#2",
	"@JoeySaladinoShow#e",
	"@The-S#2",
	"@JIGGYTOM#4",
	"@ComputingForever#9",
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
