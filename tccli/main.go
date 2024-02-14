package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lbryio/transcoder/client"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/library"
	ldb "github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/migrator"
	"github.com/lbryio/transcoder/pkg/resolve"

	"github.com/alecthomas/kong"
	"github.com/panjf2000/ants/v2"
	"github.com/spf13/viper"
)

var CLI struct {
	GetFragmentUrl struct {
		Server string `optional:"" name:"server" help:"Transcoding server" default:"use-tower1.transcoder.odysee.com:8080"`
		URL    string `name:"url" help:"LBRY URL"`
		SDHash string `name:"sd-hash" help:"SD hash"`
		Name   string `optional:"" name:"name" help:"Fragment file name" default:"master.m3u8"`
	} `cmd:"" help:"Get fragment URL"`
	GetVideoUrl struct {
		Server string `optional:""  name:"server" help:"Transcoding server" default:"use-tower1.transcoder.odysee.com:8080"`
		URL    string `name:"url" help:"LBRY URL"`
	} `cmd:"" help:"Get video URL"`
	GenerateManifests struct {
		VideoDir string `help:"Directory containing videos"`
		DBPath   string `help:"Path to the SQLite DB file"`
	} `cmd:"" help:"Generate manifest files for videos"`
	Retire struct {
		VideoDir string `help:"Directory containing videos"`
		MaxSize  int    `help:"Max size of videos to keep in gigabytes"`
	} `cmd:"" help:"Generate manifest files for videos"`
	Genstream struct {
		Path string `arg:"" help:"Path containing transcoded stream"`
		URL  string `arg:"" help:"Stream URL"`
	} `cmd:"" help:"Generate stream"`
	Transcode struct {
		URL string `arg:"" help:"LBRY URL"`
	} `cmd:"" help:"Download and transcode a specified video"`
	ValidateStream struct {
		URL string `arg:"" help:"HTTP URL for stream to verify"`
	} `cmd:"" help:"Verify a specified stream"`
}

func main() {
	ctx := kong.Parse(&CLI)
	log := zapadapter.NewKV(logging.Create("cli", logging.Dev).Desugar())

	switch ctx.Command() {
	case "get-fragment-url":
		client.New(
			client.Configure().VideoPath(path.Join("./transcoder-client", "")).
				Server("http://" + CLI.GetFragmentUrl.Server).
				LogLevel(client.Dev),
		)

		// fmt.Println(c.BuildURL(c.GetPlaybackPath(CLI.GetFragmentUrl.URL, CLI.GetFragmentUrl.SDHash)))
	case "get-video-url":
		fmt.Printf("http://%s/api/v2/video/%s\n", CLI.GetVideoUrl.Server, url.PathEscape(strings.TrimSpace(CLI.GetVideoUrl.URL)))
	case "transcode <url>":
		var inPath, outPath string
		var rr *resolve.ResolvedStream

		if strings.HasPrefix(CLI.Transcode.URL, "file://") {
			inPath = strings.TrimPrefix(CLI.Transcode.URL, "file://")
			outPath = inPath + "_out"
		} else {
			tmpDir, err := os.MkdirTemp(".", "")
			if err != nil {
				panic(err)
			}
			rr, err = resolve.ResolveStream(CLI.Transcode.URL)
			if err != nil {
				panic(err)
			}
			f, _, err := rr.Download(tmpDir)
			if err != nil {
				panic(err)
			}
			f.Close()
			inPath, _ = filepath.Abs(f.Name())
			outPath = url.PathEscape(rr.Name)
			defer os.RemoveAll(tmpDir)
		}

		e, err := encoder.NewEncoder(encoder.Configure().Log(log).SpritegenPath("/dev/null"))
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
	case "genstream <path> <url>":
		rr, err := resolve.ResolveStream(CLI.Genstream.URL)
		if err != nil {
			panic(err)
		}
		ls := library.InitStream(CLI.Genstream.Path, "wasabi")
		err = ls.GenerateManifest(
			rr.URI, rr.ChannelURI, rr.SDHash,
			library.WithTimestamp(time.Now()),
			library.WithWorkerName("manual"),
		)
		if err != nil {
			panic(err)
		}

		cfg := viper.New()
		cfg.SetConfigName("conductor")
		cfg.AddConfigPath(".")
		err = cfg.ReadInConfig()
		if err != nil {
			panic(err)
		}

		libCfg := cfg.GetStringMapString("library")

		libDB, err := migrator.ConnectDB(migrator.DefaultDBConfig().DSN(libCfg["dsn"]).AppName("library"), ldb.MigrationsFS)
		if err != nil {
			panic(err)
		}
		lib := library.New(library.Config{
			DB:  libDB,
			Log: zapadapter.NewKV(nil),
		})
		if err := lib.AddRemoteStream(*ls); err != nil {
			fmt.Println("error adding remote stream", "err", err)
		}
	case "validate-stream <url>":
		res, err := library.ValidateStream(CLI.ValidateStream.URL, false, false)

		if err != nil {
			fmt.Printf("error validating stream: %s\n", err)
			return
		}
		fmt.Printf("%v parts present, %v missing", len(res.Present), len(res.Missing))
	case "validate-streams":
		wg := sync.WaitGroup{}
		results := make(chan *library.ValidationResult)

		go func() {
			for vr := range results {
				if len(vr.Missing) > 0 {
					fmt.Printf("%s broken\n", vr.URL)
				}
			}
		}()
		pipe := func(i interface{}) error {
			url := i.(string)
			vr, _ := library.ValidateStream(url, true, true)
			results <- vr
			return nil
		}

		scanner := bufio.NewScanner(os.Stdin)

		p, _ := ants.NewPoolWithFunc(10, func(i interface{}) {
			err := pipe(i)
			if err != nil {
				fmt.Println(err)
			}
			wg.Done()
		})
		defer p.Release()
		for scanner.Scan() {
			u := strings.TrimSpace(scanner.Text())
			if u == "" {
				break
			}
			wg.Add(1)
			_ = p.Invoke(u)
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}
		wg.Wait()
	default:
		panic(ctx.Command())
	}
}
