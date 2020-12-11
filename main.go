package main

import (
	"math/rand"
	"time"

	"github.com/lbryio/transcoder/api"

	"github.com/alecthomas/kong"
)

var CLI struct {
	Serve struct {
		Bind      string `arg optional name:"bind" help:"Address to listen on." default:":8080"`
		DBPath    string `arg optional name:"data_path" help:"Path to store database files." type:"existingdir" default:"."`
		VideoPath string `arg optional name:"video_path" help:"Path to store video." type:"existingdir" default:"."`
	} `cmd help:"Start transcoding server."`
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "serve":
		api.StartHTTP(CLI.Serve.Bind, CLI.Serve.VideoPath)
	default:
		panic(ctx.Command())
	}
}
