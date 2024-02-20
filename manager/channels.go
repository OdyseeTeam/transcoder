package manager

import (
	"sync"
	"time"

	"github.com/odyseeteam/transcoder/library"
	db "github.com/odyseeteam/transcoder/library/db"
)

type channelList struct {
	sync.Mutex
	items map[string]db.ChannelPriority
}

func newChannelList() *channelList {
	return &channelList{
		Mutex: sync.Mutex{},
		items: map[string]db.ChannelPriority{},
	}
}

func (c *channelList) StartLoadingChannels(lib *library.Library) {
	for range time.Tick(5 * time.Second) {
		channels, err := lib.GetAllChannels()
		if err != nil {
			logger.Error("error loading channels", "err", err)
		}
		c.Load(channels)
	}
}

func (c *channelList) Load(channels []db.Channel) {
	c.Lock()
	defer c.Unlock()
	for _, ch := range channels {
		c.items[ch.ClaimID] = ch.Priority
	}
}

func (c *channelList) GetPriority(r *TranscodingRequest) db.ChannelPriority {
	c.Lock()
	defer c.Unlock()
	if ch, ok := c.items[r.ChannelClaimID]; !ok {
		return db.ChannelPriorityLow
	} else {
		return ch
	}
}
