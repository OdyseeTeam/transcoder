package video

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
)

const (
	lbrytvAPI = "https://api.lbry.tv/api/v1/proxy"
	cdnServer = "https://cdn.lbryplayer.xyz/api/v3/streams"
)

var (
	client = ljsonrpc.NewClient(lbrytvAPI)

	ErrStreamNotFound = errors.New("could not resolve stream URI")
)

type Claim struct {
	*ljsonrpc.Claim
	sdHash string
}

func Resolve(uri string) (*Claim, error) {
	resolved, err := client.Resolve(uri)
	if err != nil {
		return nil, err
	}

	claim := (*resolved)[uri]
	if claim.CanonicalURL == "" {
		return nil, ErrStreamNotFound
	}
	wc, err := wrapClaim(&claim)
	if err != nil {
		return nil, err
	}
	return wc, nil
}

func wrapClaim(lc *ljsonrpc.Claim) (*Claim, error) {
	c := &Claim{lc, ""}
	h, err := c.getSDHash()
	if err != nil {
		return nil, err
	}
	c.sdHash = h
	return c, err
}

// Download retrieves a video stream from the lbrytv CDN and saves it to a temporary file.
func (c *Claim) Download() (*os.File, int64, error) {
	var readLen int64

	req, err := http.NewRequest("GET", c.cdnURL(), nil)
	logger.Debugw("download stream", "url", c.cdnURL())

	client := &http.Client{Timeout: time.Second * 30}
	resp, err := client.Do(req)
	if err != nil {
		return nil, readLen, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readLen, fmt.Errorf("http response not ok: %v", resp.StatusCode)
	}
	defer resp.Body.Close()

	if err = os.MkdirAll(tempStreamPath, os.ModePerm); err != nil {
		return nil, readLen, err
	}

	out, err := ioutil.TempFile(tempStreamPath, c.streamFileName())
	if err != nil {
		return nil, readLen, err
	}

	if readLen, err = io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(out.Name())
		return nil, readLen, err
	}
	return out, readLen, nil
}

func (c *Claim) cdnURL() string {
	return fmt.Sprintf("%s/free/%s/%s/%s", cdnServer, c.Name, c.ClaimID, c.sdHash[:6])
}

func (c *Claim) getSDHash() (string, error) {
	src := c.Value.GetStream().GetSource()
	if src == nil {
		return "", errors.New("stream doesn't have source data")
	}
	return hex.EncodeToString(src.SdHash), nil
}

func (c *Claim) streamFileName() string {
	return fmt.Sprintf("%s_%s", c.ChannelName, c.sdHash[:6])
}
