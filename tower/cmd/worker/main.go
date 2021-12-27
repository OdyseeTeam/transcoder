package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/alecthomas/kong"
)

const configName = "worker"

var CLI struct {
	Start struct {
		WorkerID   string `help:"Worker ID"`
		RMQAddr    string `optional:"" help:"RabbitMQ server address" default:"amqp://guest:guest@localhost/"`
		Workers    int    `optional:"" help:"Encoding workers to spawn" default:"16"`
		Threads    int    `optional:"" help:"Encoding threads per encoding worker" default:"2"`
		WorkDir    string `optional:"" help:"Directory for storing downloaded and transcoded files" default:"./"`
		BlobServer string `optional:"" name:"blob-server" help:"LBRY blobserver address."`
	} `cmd:"" help:"Start transcoding worker"`
	Debug bool `optional:"" help:"Enable debug logging" default:"false"`
}

func main() {
	var logger *zap.Logger
	ctx := kong.Parse(&CLI)

	if CLI.Debug {
		logger, _ = zap.NewDevelopmentConfig().Build()
	} else {
		logger, _ = zap.NewProductionConfig().Build()
	}

	log := logger.Sugar()

	switch ctx.Command() {
	case "start":

		cfg, err := readConfig()
		if err != nil {
			log.Fatal("unable to read config", err)
		}

		s3cfg := cfg.GetStringMapString("s3")

		err = os.MkdirAll(CLI.Start.WorkDir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}

		s3driver, err := storage.InitS3Driver(
			storage.S3Configure().
				Endpoint(s3cfg["endpoint"]).
				Credentials(s3cfg["key"], s3cfg["secret"]).
				Bucket(s3cfg["bucket"]))
		if err != nil {
			log.Fatal("s3 driver initialization failed", err)
		}
		log.Infow("s3 storage configured", "endpoint", s3cfg["endpoint"])

		c, err := tower.NewWorker(tower.DefaultWorkerConfig().
			WorkerID(CLI.Start.WorkerID).
			Logger(zapadapter.NewKV(logger.Named("tower.worker"))).
			PoolSize(CLI.Start.Workers).
			WorkDir(CLI.Start.WorkDir).
			RMQAddr(CLI.Start.RMQAddr).
			S3Driver(s3driver),
		)
		if err != nil {
			log.Fatal(err)
		}

		if CLI.Start.BlobServer != "" {
			log.Infow("blob server set", "address", CLI.Start.BlobServer)
			manager.SetBlobServer(CLI.Start.BlobServer)
		}

		log.Infow("starting tower worker", "tower_server", CLI.Start.RMQAddr)
		c.StartWorkers()

		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

		sig := <-stopChan
		log.Infof("caught an %v signal, shutting down...", sig)
		c.Stop()
	default:
		panic(ctx.Command())
	}
}

func readConfig() (*viper.Viper, error) {
	cfg := viper.New()
	cfg.SetConfigName(configName)

	exec, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cfg.AddConfigPath(filepath.Dir(exec))
	cfg.AddConfigPath(".")

	return cfg, cfg.ReadInConfig()
}
