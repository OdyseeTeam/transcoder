package claim

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/transcoder/pkg/timer"
)

var (
	lbrytvAPI = "https://api.lbry.tv/api/v1/proxy"
	cdnServer = "https://cdn.lbryplayer.xyz/api/v3/streams"
)

var (
	lbrytvClient   = ljsonrpc.NewClient(lbrytvAPI)
	downloadClient = &http.Client{
		Timeout: 1200 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 120 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}

	ErrStreamNotFound = errors.New("could not resolve stream URI")
)

type WriteCounter struct {
	Loaded, Size   uint64
	Started        time.Time
	URL            string
	progressLogged map[int]bool
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Loaded += uint64(n)
	progress := int(float64(wc.Loaded) / float64(wc.Size) * 100)

	if progress > 0 && progress%25 == 0 && !wc.progressLogged[progress] {
		wc.progressLogged[progress] = true
		rate := int64(float64(wc.Loaded) / time.Since(wc.Started).Seconds())
		logger.Debugw(
			"download progress",
			"url", wc.URL,
			"size", wc.Size, "progress", int(progress), "rate", rate)
	}
	return n, nil
}

type Claim struct {
	*ljsonrpc.Claim
	SDHash string
}

func Resolve(uri string) (*Claim, error) {
	resolved, err := lbrytvClient.Resolve(uri)
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
	c.SDHash = h
	return c, err
}

// Download retrieves a video stream from the lbrytv CDN and saves it to a temporary file.
func (c *Claim) Download(dest string) (*os.File, int64, error) {
	var readLen int64

	req, err := http.NewRequest("GET", c.cdnURL(), nil)
	logger.Infow("downloading stream", "url", c.cdnURL())

	tmr := timer.Start()

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, readLen, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readLen, fmt.Errorf("http response not ok: %v", resp.StatusCode)
	}
	defer resp.Body.Close()

	if err = os.MkdirAll(dest, os.ModePerm); err != nil {
		return nil, readLen, err
	}

	out, err := ioutil.TempFile(dest, c.streamFileName())
	if err != nil {
		return nil, readLen, err
	}

	wc := &WriteCounter{
		Size:           uint64(resp.ContentLength),
		Started:        time.Now(),
		URL:            c.cdnURL(),
		progressLogged: map[int]bool{},
	}
	if readLen, err = io.Copy(out, io.TeeReader(resp.Body, wc)); err != nil {
		out.Close()
		os.Remove(out.Name())
		return nil, readLen, err
	}
	tmr.Stop()
	rate := int64(float64(readLen) / tmr.Duration())
	logger.Infow("stream downloaded", "url", c.cdnURL(), "rate", rate, "size", readLen, "seconds_spent", tmr.DurationInt())
	return out, readLen, nil
}

func (c *Claim) cdnURL() string {
	return fmt.Sprintf("%s/free/%s/%s/%s", cdnServer, c.Name, c.ClaimID, c.SDHash[:6])
}

func (c *Claim) getSDHash() (string, error) {
	src := c.Value.GetStream().GetSource()
	if src == nil {
		return "", errors.New("stream doesn't have source data")
	}
	return hex.EncodeToString(src.SdHash), nil
}

func (c *Claim) streamFileName() string {
	return c.SDHash
}

func SetCDNServer(s string) {
	cdnServer = s
}
