package video

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIncomingVideo(t *testing.T) {
	LoadEnabledChannels(
		[]string{
			"@davidpakman#7",
			"@specialoperationstest#3",
		})
	urlsEnabled := []string{
		"lbry://@davidpakman#7/vaccination-delays-and-more-biden-picks#8",
		"lbry://@specialoperationstest#3/fear-of-death-inspirational#a",
	}
	urlsDisabled := []string{
		"lbry://@TRUTH#2/what-do-you-know-what-do-you-believe#2",
		"lbry://@samtime#1/airpods-max-parody-ehh-pods-max#7",
		"lbry://what#1",
	}
	for _, u := range urlsEnabled {
		_, err := ValidateIncomingVideo(u)
		assert.NoError(t, err)
	}
	for _, u := range urlsDisabled {
		_, err := ValidateIncomingVideo(u)
		assert.Equal(t, ErrChannelNotEnabled, err)
	}
}
