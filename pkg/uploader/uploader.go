package uploader

import (
	"bytes"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"

	"github.com/lbryio/transcoder/pkg/logging"
)

var log logging.KVLogger = logging.NoopKVLogger{}

func buildUploadRequest(tarPath, targetURL string, checksum []byte) (*http.Request, error) {
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

	req, err := http.NewRequest(http.MethodPost, targetURL, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}
