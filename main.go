package main

import (
	"math/rand"
	"path"
	"time"

	"github.com/lbryio/transcoder/api"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
)

var CLI struct {
	Serve struct {
		Bind      string `optional name:"bind" help:"Address to listen on." default:":8080"`
		DBPath    string `optional name:"data_path" help:"Path to store database files." type:"existingdir" default:"."`
		VideoPath string `optional name:"video_path" help:"Path to store video." type:"existingdir" default:"."`
		Debug     bool   `optional name:"debug" help:"Debug mode."`
	} `cmd help:"Start transcoding server."`
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "serve":
		vdb := db.OpenDB(path.Join(CLI.Serve.DBPath, "video.sqlite"))
		vdb.MigrateUp(video.InitialMigration)
		qdb := db.OpenDB(path.Join(CLI.Serve.DBPath, "queue.sqlite"))
		qdb.MigrateUp(queue.InitialMigration)

		lib := video.NewLibrary(vdb)
		q := queue.NewQueue(qdb)

		go video.SpawnProcessing(CLI.Serve.VideoPath, q, lib)
		api.InitHTTP(CLI.Serve.Bind, CLI.Serve.VideoPath, CLI.Serve.Debug, api.NewManager(q, lib))
	default:
		panic(ctx.Command())
	}
}
