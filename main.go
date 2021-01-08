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
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/claim"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
)

var logger = logging.Create("main", logging.Dev)

var CLI struct {
	Serve struct {
		Bind      string `optional name:"bind" help:"Address to listen on." default:":8080"`
		DataPath  string `optional name:"data-path" help:"Path to store database files and configs." type:"existingdir" default:"."`
		VideoPath string `optional name:"video-path" help:"Path to store video." type:"existingdir" default:"."`
		Workers   int    `optional name:"workers" help:"Number of workers to start." type:"int" default:"10"`
		Debug     bool   `optional name:"debug" help:"Debug mode."`
	} `cmd help:"Start transcoding server."`
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "serve":
		if !CLI.Serve.Debug {
			api.SetLogger(logging.Create("api", logging.Prod))
			db.SetLogger(logging.Create("db", logging.Prod))
			queue.SetLogger(logging.Create("queue", logging.Prod))
			encoder.SetLogger(logging.Create("encoder", logging.Prod))
			video.SetLogger(logging.Create("video", logging.Prod))
			claim.SetLogger(logging.Create("video", logging.Prod))
		}
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

		poller := q.StartPoller(CLI.Serve.Workers)
		for i := 0; i < CLI.Serve.Workers; i++ {
			go video.SpawnProcessing(CLI.Serve.VideoPath, q, lib, poller)
		}
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
