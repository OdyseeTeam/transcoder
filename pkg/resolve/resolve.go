package resolve

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/transcoder/pkg/timer"

	"github.com/nikooo777/lbry-blobs-downloader/downloader"
	"github.com/nikooo777/lbry-blobs-downloader/shared"
)

var (
	odyseeAPI  = "https://api.na-backend.odysee.com/api/v1/proxy"
	blobServer = "blobcache-eu.lbry.com"
	edgeToken  = "UNSET"

	lbrytvClient = ljsonrpc.NewClient(odyseeAPI)

	ErrNotReflected = errors.New("stream not fully reflected")
	ErrNetwork      = errors.New("network error")

	reClaimID = regexp.MustCompile("^[a-z0-9]{40}$")
)

type WriteCounter struct {
	Loaded, Size   uint64
	Started        time.Time
	URL            string
	progressLogged map[int]bool
}

type ResolvedStream struct {
	URI, Name, ClaimID, SDHash, ChannelURI,
	ChannelClaimID, NormalizedName string
	ChannelSupportAmount int64
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

func ResolveStream(uri string) (*ResolvedStream, error) {
	claim, err := Resolve(uri)
	if err != nil {
		return nil, err
	}

	if claim.SigningChannel == nil {
		return nil, ErrNoSigningChannel
	}

	src := claim.Value.GetStream().GetSource()
	if src == nil {
		return nil, errors.New("stream doesn't have source data")
	}
	h := hex.EncodeToString(src.SdHash)

	ch := strings.Replace(strings.ToLower(claim.SigningChannel.CanonicalURL), "#", ":", 1)
	sup, _ := strconv.ParseFloat(claim.SigningChannel.Meta.SupportAmount, 64)

	r := &ResolvedStream{
		URI:                  claim.CanonicalURL,
		SDHash:               h,
		Name:                 claim.Name,
		NormalizedName:       claim.NormalizedName,
		ClaimID:              claim.ClaimID,
		ChannelURI:           ch,
		ChannelClaimID:       claim.SigningChannel.ClaimID,
		ChannelSupportAmount: int64(math.Floor(sup)),
	}
	return r, nil
}

func Resolve(uri string) (*ljsonrpc.Claim, error) {
	lbrytvClient.SetRPCTimeout(10 * time.Second)

	if reClaimID.Match([]byte(uri)) {
		res, err := lbrytvClient.ClaimSearch(ljsonrpc.ClaimSearchArgs{
			ClaimID:  &uri,
			Page:     1,
			PageSize: 1,
		})
		if err != nil {
			if strings.Contains(err.Error(), "rpc call claim_search()") {
				return nil, fmt.Errorf("%w: %s", ErrNetwork, err)
			}
			return nil, err
		}
		if len(res.Claims) == 0 {
			return nil, ErrClaimNotFound
		}
		return &res.Claims[0], nil
	}
	resolved, err := lbrytvClient.Resolve(uri)
	if err != nil {
		if strings.Contains(err.Error(), "rpc call resolve()") {
			return nil, fmt.Errorf("%w: %s", ErrNetwork, err)
		}
		return nil, err
	}

	c, ok := (*resolved)[uri]
	if !ok {
		return nil, ErrClaimNotFound
	}
	return &c, nil
}

// Download retrieves a stream from LBRY CDN and saves it into dstDir folder under original name.
func (c *ResolvedStream) Download(dstDir string) (*os.File, int64, error) {
	UDPPort := 5568
	TCPPort := 5567
	HTTPPort := 5569

	// TODO: Fix this
	shared.ReflectorPeerServer = fmt.Sprintf("%s:%d", blobServer, TCPPort)
	shared.ReflectorQuicServer = fmt.Sprintf("%s:%d", blobServer, UDPPort)
	shared.ReflectorHttpServer = fmt.Sprintf("%s:%d", blobServer, HTTPPort)
	shared.EdgeToken = edgeToken

	var readLen int64
	dstFile := path.Join(dstDir, c.streamFileName())

	logger.Infow("downloading stream", "url", c.URI)
	t := timer.Start()

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return nil, 0, err
	}

	tmpBlobsPath := "tmp_" + c.SDHash
	sdBlob, err := downloader.DownloadStream(c.SDHash, false, downloader.HTTP, tmpBlobsPath, 3)
	// This is needed to cleanup after downloader fails midway
	defer os.RemoveAll(path.Join(os.TempDir(), c.streamFileName()+".tmp"))
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmpBlobsPath)

	if err := shared.BuildStream(sdBlob, c.streamFileName(), dstDir, tmpBlobsPath); err != nil {
		// This is needed to cleanup after BuildStream failing midway
		os.RemoveAll(path.Join(dstDir, c.streamFileName()))
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			return nil, 0, ErrNotReflected
		}
		return nil, 0, err
	}
	t.Stop()

	fi, err := os.Stat(dstFile)
	if err != nil {
		return nil, 0, err
	}
	readLen = fi.Size()

	f, err := os.Open(dstFile)
	if err != nil {
		return nil, 0, err
	}

	rate := int64(float64(readLen) / t.Duration())
	logger.Infow("stream downloaded", "url", c.URI, "rate", rate, "size", readLen, "seconds_spent", t.DurationInt())
	return f, readLen, nil
}

func (c *ResolvedStream) streamFileName() string {
	return c.SDHash
}

func SetBlobServer(s string) {
	blobServer = s
}

func SetEdgeToken(s string) {
	edgeToken = s
}
