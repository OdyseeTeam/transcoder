package uploader

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/valyala/fasthttp"
)

const (
	fileField     = "packaged_stream_file"
	checksumField = "packaged_stream_checksum"
)

type FileHandler struct {
	uploadPath string
	checkAuth  func(*fasthttp.RequestCtx) bool
}

// ...
func (h *FileHandler) Handle(ctx *fasthttp.RequestCtx) {
	var checksum []byte
	sdHash := ctx.UserValue("sd_hash").(string)
	dstPath := path.Join(h.uploadPath, sdHash)

	if !h.checkAuth(ctx) {
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.SetBodyString("authentication failed")
		return
	}

	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.SetBodyString("stream already exists")
		return
	}

	form, err := ctx.MultipartForm()
	if err != nil {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("%v", err))
		return
	}
	defer ctx.Request.RemoveMultipartFormFiles()

	if checksums, ok := form.Value[checksumField]; !ok {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("no checksum supplied in %v", checksumField))
		return
	} else {
		checksum, err = hex.DecodeString(checksums[0])
		if err != nil {
			ctx.SetStatusCode(http.StatusBadRequest)
			ctx.SetBodyString(fmt.Sprintf("erroneous checksum in %v", checksumField))
			return
		}
	}
	if files, ok := form.File[fileField]; !ok {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("no field %v", fileField))
		return
	} else {
		f, err := files[0].Open()
		if err != nil {
			ctx.SetStatusCode(http.StatusInternalServerError)
			ctx.SetBodyString(fmt.Sprintf("error opening received file: %v", err))
			return
		}
		cs, err := unpackStream(f, dstPath)
		f.Close()

		if err != nil {
			os.RemoveAll(dstPath)
			ctx.SetStatusCode(http.StatusInternalServerError)
			ctx.SetBodyString(fmt.Sprintf("error unpacking stream: %v", err))
			return
		} else if !bytes.Equal(checksum, cs) {
			os.RemoveAll(dstPath)
			ctx.SetStatusCode(http.StatusBadRequest)
			ctx.SetBodyString(fmt.Sprintf(
				"provided checksum %s doesn't match calculated checksum %s", hex.EncodeToString(checksum), hex.EncodeToString(cs)))
			return
		}
		ctx.SetStatusCode(http.StatusAccepted)
	}
}
