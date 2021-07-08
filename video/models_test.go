package video

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVideo(t *testing.T) {
	var v *Video
	var url string
	var remote bool

	v = &Video{Path: "ashsadasldkhaw", RemotePath: "ashsadasldkhaw"}
	url, remote = v.GetLocation()
	assert.False(t, remote)
	assert.Equal(t, "ashsadasldkhaw/master.m3u8", url)

	v = &Video{Path: "ashsadasldkhaw", RemotePath: ""}
	url, remote = v.GetLocation()
	assert.False(t, remote)
	assert.Equal(t, "ashsadasldkhaw/master.m3u8", url)

	v = &Video{Path: "", RemotePath: "ashsadasldkhaw"}
	url, remote = v.GetLocation()
	assert.True(t, remote)
	assert.Equal(t, "remote://ashsadasldkhaw/master.m3u8", url)
}
