package uploader

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/storage"
)

const TarExtension = "tar"

var log logging.KVLogger = logging.NoopKVLogger{}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type UploaderConfig struct {
	client  httpDoer
	log     logging.KVLogger
	retries int
	backOff time.Duration
}

type Uploader struct {
	*UploaderConfig
}

func DefaultUploaderConfig() *UploaderConfig {
	return &UploaderConfig{
		client:  http.DefaultClient,
		retries: 5,
		backOff: 1 * time.Second,
		log:     &logging.NoopKVLogger{},
	}
}

// NewUploader ...
func NewUploader(config *UploaderConfig) *Uploader {
	u := Uploader{
		UploaderConfig: config,
	}

	return &u
}

func (c *UploaderConfig) Logger(logger logging.KVLogger) *UploaderConfig {
	c.log = logger
	return c
}

func (c *UploaderConfig) Client(client httpDoer) *UploaderConfig {
	c.client = client
	return c
}

func (c *UploaderConfig) Retries(retries int) *UploaderConfig {
	c.retries = retries
	return c
}

func (c *UploaderConfig) BackOff(backOff time.Duration) *UploaderConfig {
	c.backOff = backOff
	return c
}

func (u *Uploader) Upload(ctx context.Context, dir, url, token string) error {
	tarPath := path.Base(dir) + "." + TarExtension
	defer os.Remove(tarPath)

	ls, err := storage.OpenLocalStream(dir)
	if err != nil {
		return err
	}
	csum, err := packStream(ls, tarPath)
	if err != nil {
		return err
	}
	req, err := buildUploadRequest(ctx, tarPath, url, token, csum)
	if err != nil {
		return err
	}

	u.log.Debug("uploading file", "path", dir, "url", url)

	var res *http.Response
	uploaded := false
	for i := 0; i < u.retries; i++ {
		time.Sleep(time.Duration(i) * u.backOff)
		res, err = u.client.Do(req)
		if err != nil {
			u.log.Info("upload failed", "err", err)
			continue
		}
		if res.StatusCode != http.StatusAccepted {
			body, _ := ioutil.ReadAll(res.Body)
			err = fmt.Errorf("upload failed to %s: response code %v, response: %s", req.URL, res.StatusCode, body)
			u.log.Info("upload failed", "err", err)
			continue
		}
		u.log.Info("uploaded", "status_code", res.StatusCode)
		uploaded = true
		break
	}

	if !uploaded {
		return err
	}
	return nil
}

func buildUploadRequest(ctx context.Context, tarPath, targetURL, token string, checksum []byte) (*http.Request, error) {
	r, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}

	defer r.Close()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	fw, err := writer.CreateFormFile(fileField, path.Base(tarPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(fw, r); err != nil {
		return nil, err
	}

	cfw, err := writer.CreateFormField(checksumField)
	if err != nil {
		return nil, err
	}

	cfw.Write([]byte(hex.EncodeToString(checksum)))

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Auth-Token", token)
	return req, nil
}
