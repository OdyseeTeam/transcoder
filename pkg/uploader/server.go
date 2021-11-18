package uploader

import (
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path"

	"github.com/fasthttp/router"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/storage"
	"github.com/valyala/fasthttp"
)

type fileHandler struct {
	uploadPath   string
	authCallback func(*fasthttp.RequestCtx) bool
	doneCallback func(storage.LightLocalStream)
}

type AuthCallback func(*fasthttp.RequestCtx) bool
type DoneCallback func(storage.LightLocalStream)

const (
	fileField     = "packaged_stream_file"
	checksumField = "packaged_stream_checksum"
)

// NewUploadServer will create a fasthttp server for receiving tarred streams.
func NewUploadServer(uploadPath string, acb AuthCallback, dcb DoneCallback) *fasthttp.Server {
	r := router.New()
	AttachFileHandler(r, "", uploadPath, acb, dcb)
	return &fasthttp.Server{
		Handler:            manager.MetricsMiddleware(manager.CORSMiddleware(r.Handler)),
		Name:               "tower",
		MaxRequestBodySize: 10 * 1024 * 1024 * 1024,
	}
}

func AttachFileHandler(r *router.Router, prefix, uploadPath string, acb AuthCallback, dcb DoneCallback) {
	h := fileHandler{
		uploadPath:   uploadPath,
		authCallback: acb,
		doneCallback: dcb,
	}
	r.Group(prefix).POST(`/{sd_hash:[a-z0-9]{96}}`, h.Handle)
}

// Handle will receive and unpack a tarred stream file, validating its checksum.
func (h *fileHandler) Handle(ctx *fasthttp.RequestCtx) {
	var (
		checksum        []byte
		checksumEncoded string
		incomingFile    multipart.File
	)

	sdHash := ctx.UserValue("sd_hash").(string)
	dstPath := path.Join(h.uploadPath, sdHash)

	token := string(ctx.Request.Header.Peek("X-Auth-Token"))

	ctx.SetUserValue("token", token)
	ctx.SetUserValue("ref", sdHash)

	if !h.authCallback(ctx) {
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.SetBodyString("authentication failed")
		return
	}

	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		// ctx.SetStatusCode(http.StatusForbidden)
		// ctx.SetBodyString("stream already exists")
		// return
		// TODO: This is not the best practice and potentially unsafe
		os.RemoveAll(dstPath)
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
		checksumEncoded = checksums[0]
	}
	checksum, err = hex.DecodeString(checksumEncoded)
	if err != nil {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("erroneous checksum in %v", checksumField))
		return
	}

	if ff, ok := form.File[fileField]; !ok {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf("no field %v", fileField))
		return
	} else {
		incomingFile, err = ff[0].Open()
		if err != nil {
			ctx.SetStatusCode(http.StatusInternalServerError)
			ctx.SetBodyString(fmt.Sprintf("error opening received file: %v", err))
			return
		}
	}

	ls, err := unpackStream(incomingFile, dstPath)
	incomingFile.Close()
	if err != nil {
		os.RemoveAll(dstPath)
		ctx.SetStatusCode(http.StatusInternalServerError)
		ctx.SetBodyString(fmt.Sprintf("error unpacking stream: %v", err))
		return
	} else if !ls.ChecksumValid(checksum) {
		os.RemoveAll(dstPath)
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf(
			"provided checksum %s doesn't match calculated checksum %s", hex.EncodeToString(checksum), ls.ChecksumString()))
		return
	}

	ls.SDHash = sdHash
	h.doneCallback(*ls)
	ctx.SetStatusCode(http.StatusAccepted)
}
