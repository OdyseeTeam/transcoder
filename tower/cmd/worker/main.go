package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/tower"
	"go.uber.org/zap"

	"github.com/alecthomas/kong"
)

var CLI struct {
	Start struct {
		RMQAddr string `optional help:"RabbitMQ server address" default:"amqp://guest:guest@localhost/"`
		Workers int    `optional help:"Encoding workers to spawn" default:16`
		Threads int    `optional help:"Encoding threads per encoding worker" default:2`
		WorkDir string `optional help:"Directory for storing downloaded and transcoded files" default:"./"`
	} `cmd help:"Start transcoding worker"`
	Debug bool `optional help:"Enable debug logging" default:false`
}

func main() {
	var logger *zap.Logger
	ctx := kong.Parse(&CLI)

	if CLI.Debug {
		logger, _ = zap.NewProductionConfig().Build()
	} else {
		logger, _ = zap.NewDevelopmentConfig().Build()
	}

	log := logger.Sugar()

	switch ctx.Command() {
	case "start":
		c, err := tower.NewWorker(tower.DefaultWorkerConfig().
			Logger(zapadapter.NewKV(logger.Named("tower.worker"))).
			PoolSize(CLI.Start.Workers).
			WorkDir(CLI.Start.WorkDir).
			RMQAddr(CLI.Start.RMQAddr),
		)
		if err != nil {
			log.Fatal(err)
		}

		log.Infow("starting tower worker", "tower_server", CLI.Start.RMQAddr)
		go c.StartSendingStatus()
		err = c.StartWorkers()
		if err != nil {
			log.Fatal(err)
		}

		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

		sig := <-stopChan
		log.Infof("caught an %v signal, shutting down...", sig)
		c.Stop()
	default:
		panic(ctx.Command())
	}
}
