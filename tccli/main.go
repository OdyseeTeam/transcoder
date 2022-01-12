package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/client"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
)

var CLI struct {
	GetFragmentUrl struct {
		Server string `optional name:"server" help:"Transcoding server" default:"use-tower1.transcoder.odysee.com:8080"`
		URL    string `name:"url" help:"LBRY URL"`
		SDHash string `name:"sd-hash" help:"SD hash"`
		Name   string `optional name:"name" help:"Fragment file name" default:"master.m3u8"`
	} `cmd help:"Get fragment URL"`
	GetVideoUrl struct {
		Server string `optional name:"server" help:"Transcoding server" default:"use-tower1.transcoder.odysee.com:8080"`
		URL    string `name:"url" help:"LBRY URL"`
	} `cmd help:"Get video URL"`
	GenerateManifests struct {
		VideoDir string `help:"Directory containing videos"`
		DBPath   string `help:"Path to the SQLite DB file"`
		URL      string `name:"url" help:"LBRY URL"`
	} `cmd help:"Generate manifest files for videos"`
	Transcode struct {
		URL string `arg:"" help:"LBRY URL"`
	} `cmd help:"Download and transcode a specified video"`
}

func main() {
	ctx := kong.Parse(&CLI)
	log := zapadapter.NewKV(logging.Create("cli", logging.Dev).Desugar())

	switch ctx.Command() {
	case "get-fragment-url":
		c := client.New(
			client.Configure().VideoPath(path.Join("./transcoder-client", "")).
				Server("http://" + CLI.GetFragmentUrl.Server).
				LogLevel(client.Dev),
		)

		fmt.Println(c.BuildUrl(c.GetPlaybackPath(CLI.GetFragmentUrl.URL, CLI.GetFragmentUrl.SDHash)))
	case "get-video-url":
		fmt.Printf("http://%s/api/v2/video/%s\n", CLI.GetVideoUrl.Server, url.PathEscape(strings.TrimSpace(CLI.GetVideoUrl.URL)))
	case "generate-manifests":
		var count int64
		vdb := db.OpenDB(CLI.GenerateManifests.DBPath)
		libCfg := video.Configure().
			LocalStorage(storage.Local(CLI.GenerateManifests.VideoDir)).
			DB(vdb)
		lib := video.NewLibrary(libCfg)
		dirs, err := godirwalk.ReadDirnames(CLI.GenerateManifests.VideoDir, nil)
		if err != nil {
			panic(err)
		}
		for _, dir := range dirs {
			v, err := lib.Get(dir)
			if err != nil {
				log.Info("cannot retrieve video", "sd_hash", dir, "err", err)
				continue
			}
			ls, err := storage.OpenLocalStream(path.Join(CLI.GenerateManifests.VideoDir, dir), storage.Manifest{URL: v.URL, SDHash: v.SDHash})
			if err != nil {
				log.Info("cannot open local stream", "sd_hash", dir, "err", err)
				continue
			}
			err = ls.FillManifest()
			if err != nil {
				log.Info("cannot fill manifest", "sd_hash", dir, "err", err)
				continue
			}
			log.Info("processed stream", "dir", dir)
			count++
		}
		log.Info("manifests written", "count", count)
	case "transcode <url>":
		var inPath, outPath string
		var m storage.Manifest

		if strings.HasPrefix(CLI.Transcode.URL, "file://") {
			inPath = strings.TrimPrefix(CLI.Transcode.URL, "file://")
			outPath = inPath
		} else {
			tmpDir, err := ioutil.TempDir(".", "")
			if err != nil {
				panic(err)
			}
			rr, err := manager.ResolveRequest(CLI.Transcode.URL)
			if err != nil {
				panic(err)
			}
			f, _, err := rr.Download(tmpDir)
			if err != nil {
				panic(err)
			}
			f.Close()
			inPath, _ = filepath.Abs(f.Name())
			outPath = url.PathEscape(rr.URI)
			m = storage.Manifest{URL: rr.URI, ChannelURL: rr.ChannelURI, SDHash: rr.SDHash}
			defer os.RemoveAll(tmpDir)
		}

		e, err := encoder.NewEncoder(encoder.Configure().Log(log))
		if err != nil {
			panic(err)
		}
		t := time.Now()
		r, err := e.Encode(inPath, outPath)
		if err != nil {
			panic(err)
		}
		for p := range r.Progress {
			fmt.Printf("%.2f ", p.GetProgress())
		}
		fmt.Printf("done in %.2f seconds\n", time.Since(t).Seconds())
		m.Formats = r.Formats
		ls, err := storage.OpenLocalStream(outPath, m)
		if err != nil {
			panic(err)
		}
		err = ls.FillManifest()
		if err != nil {
			panic(err)
		}
	default:
		panic(ctx.Command())
	}
}
