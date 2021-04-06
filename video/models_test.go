package video

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVideo(t *testing.T) {
	var v *Video
	var url string
	var remote bool

	v = &Video{Path: "/dasfoyw/master.m3u8", RemotePath: "http://s3/dasfoyw/master.m3u8"}
	url, remote = v.GetLocation()
	assert.False(t, remote)
	assert.Equal(t, v.Path, url)

	v = &Video{Path: "/dasfoyw/master.m3u8", RemotePath: ""}
	url, remote = v.GetLocation()
	assert.False(t, remote)
	assert.Equal(t, v.Path, url)

	v = &Video{Path: "", RemotePath: "http://s3/dasfoyw/master.m3u8"}
	url, remote = v.GetLocation()
	assert.True(t, remote)
	assert.Equal(t, v.RemotePath, url)
}
