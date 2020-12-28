package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/api"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
	"go.uber.org/zap"
)

var logger = zap.NewExample().Sugar()

var CLI struct {
	Serve struct {
		Bind      string `optional name:"bind" help:"Address to listen on." default:":8080"`
		DataPath  string `optional name:"data-path" help:"Path to store database files and configs." type:"existingdir" default:"."`
		VideoPath string `optional name:"video-path" help:"Path to store video." type:"existingdir" default:"."`
		Debug     bool   `optional name:"debug" help:"Debug mode."`
	} `cmd help:"Start transcoding server."`
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "serve":
		vdb := db.OpenDB(path.Join(CLI.Serve.DataPath, "video.sqlite"))
		vdb.MigrateUp(video.InitialMigration)
		qdb := db.OpenDB(path.Join(CLI.Serve.DataPath, "queue.sqlite"))
		qdb.MigrateUp(queue.InitialMigration)

		lib := video.NewLibrary(vdb)
		q := queue.NewQueue(qdb)

		channels := []string{}
		channelsFilePath := path.Join(CLI.Serve.DataPath, "enabled_channels.cfg")
		cf, err := os.Open(channelsFilePath)
		if err != nil {
			logger.Fatalw("cannot open channels file", "path", channelsFilePath, "err", err)
		}
		scanner := bufio.NewScanner(cf)
		for scanner.Scan() {
			channels = append(channels, scanner.Text())
		}
		logger.Debugw("found channels", "channels", fmt.Sprintf("%v", channels))
		video.LoadEnabledChannels(channels)

		go video.SpawnProcessing(CLI.Serve.VideoPath, q, lib)
		err = api.NewServer(
			api.Configure().
				Debug(CLI.Serve.Debug).
				Addr(CLI.Serve.Bind).
				VideoPath(CLI.Serve.VideoPath).
				VideoManager(api.NewManager(q, lib)),
		).Start()
		if err != nil {
			logger.Fatal(err)
		}
	default:
		logger.Fatal(ctx.Command())
	}
}
