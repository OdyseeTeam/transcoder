package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/library"
	ldb "github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/conductor"
	"github.com/lbryio/transcoder/pkg/conductor/tasks"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/mfr"
	"github.com/lbryio/transcoder/pkg/migrator"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/queue"

	"github.com/alecthomas/kong"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var logger *zap.Logger

var CLI struct {
	queue.MigratorCLI
	Conductor struct {
		HttpBind string `optional:"" help:"Address for HTTP server to listen on" default:"0.0.0.0:8080"`
	} `cmd:"" help:"Start conductor server"`
	Worker struct {
		StreamsDir  string `optional:"" help:"Directory for storing downloaded files"`
		OutputDir   string `optional:"" help:"Directory for storing encoder output files"`
		BlobServer  string `optional:"" name:"blob-server" help:"LBRY blobserver address."`
		Concurrency int    `optional:"" help:"Number of task slots" default:"5"`
	} `cmd:"" help:"Start worker"`
	Redis string `optional:"" help:"Redis server address"`
	Debug bool   `optional:"" help:"Enable debug logging" default:"false"`
}

func main() {
	ctx := kong.Parse(&CLI)

	if CLI.Debug {
		logger = logging.Create("", logging.Dev).Desugar()
	} else {
		logger = logging.Create("", logging.Prod).Desugar()
	}

	switch ctx.Command() {
	case "conductor":
		startConductor()
	case "worker":
		startWorker()
	default:
		panic(ctx.Command())
	}
}

func startConductor() {
	log := logger.Sugar()
	cfg, err := readConfig("conductor")
	if err != nil {
		log.Fatal("unable to read config", err)
	}

	if !CLI.Debug {
		encoder.SetLogger(logging.Create("encoder", logging.Prod))
		manager.SetLogger(logging.Create("claim", logging.Prod))
		storage.SetLogger(logging.Create("storage", logging.Prod))
		ladder.SetLogger(logging.Create("ladder", logging.Prod))
		mfr.SetLogger(logging.Create("mfr", logging.Prod))
		dispatcher.SetLogger(logging.Create("dispatcher", logging.Prod))
	}

	s3cfg := cfg.GetStringMapString("s3")
	libCfg := cfg.GetStringMapString("library")

	libDB, err := migrator.ConnectDB(migrator.DefaultDBConfig().DSN(libCfg["dsn"]).AppName("library"), ldb.MigrationsFS)
	if err != nil {
		log.Fatal("library db initialization failed", err)
	}

	s3storage, err := storage.InitS3Driver(
		storage.S3Configure().
			Endpoint(s3cfg["endpoint"]).
			Credentials(s3cfg["key"], s3cfg["secret"]).
			Bucket(s3cfg["bucket"]).
			Name(s3cfg["name"]),
	)
	if err != nil {
		log.Fatal("s3 driver initialization failed", err)
	}
	log.Infow("s3 storage configured", "endpoint", s3cfg["endpoint"], "bucket", s3cfg["bucket"])

	lib := library.New(library.Config{
		DB:      libDB,
		Storage: s3storage,
		Log:     zapadapter.NewKV(nil),
	})

	cleanStopChan := library.SpawnLibraryCleaning(lib, s3storage.Name(), library.StringToSize(s3cfg["maxsize"]))

	adQueue := cfg.GetStringMapString("adaptivequeue")
	minHits, _ := strconv.Atoi(adQueue["minhits"])
	mgr := manager.NewManager(lib, minHits)

	httpStopChan, _ := mgr.StartHttpServer(manager.HttpServerConfig{
		ManagerToken: libCfg["managertoken"],
		Bind:         CLI.Conductor.HttpBind,
	})

	var redisURI string
	if CLI.Redis != "" {
		redisURI = CLI.Redis
	} else {
		redisURI = cfg.GetString("redis")
	}
	redisOpts, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		log.Fatal(err)
	}
	cnd, err := conductor.NewConductor(redisOpts, mgr.Requests(), lib, conductor.WithLogger(zapadapter.NewKV(log.Desugar())))
	if err != nil {
		log.Fatal(err)
	}
	cnd.Start()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-stopChan
	log.Infof("caught an %v signal, shutting down...", sig)

	close(httpStopChan)

	cnd.Stop()

	log.Infof("conductor stopped")

	close(cleanStopChan)
	log.Infof("storage cleanup shut down")

	mgr.Pool().Stop()
	log.Infof("manager shut down")
}

func startWorker() {
	log := logger.Sugar()

	cfg, err := readConfig("worker")
	if err != nil {
		log.Fatal("unable to read config", err)
	}

	s3opts := cfg.GetStringMapString("s3")

	if CLI.Worker.StreamsDir != "" {
		err = os.MkdirAll(CLI.Worker.StreamsDir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}
	if CLI.Worker.OutputDir != "" {
		err = os.MkdirAll(CLI.Worker.OutputDir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	s3cfg := storage.S3Configure().
		Endpoint(s3opts["endpoint"]).
		Credentials(s3opts["key"], s3opts["secret"]).
		Bucket(s3opts["bucket"]).
		Name(s3opts["name"])
	if s3opts["createbucket"] == "true" {
		s3cfg = s3cfg.CreateBucket()
	}
	s3storage, err := storage.InitS3Driver(s3cfg)
	if err != nil {
		log.Fatal("s3 driver initialization failed", err)
	}
	log.Infow("s3 storage configured", "endpoint", s3opts["endpoint"])

	enc, err := encoder.NewEncoder(encoder.Configure().
		Log(zapadapter.NewKV(log.Desugar())),
	)
	if err != nil {
		log.Fatal("encoder initialization failed", err)
	}

	if CLI.Worker.BlobServer != "" {
		log.Infow("blob server set", "address", CLI.Worker.BlobServer)
		resolve.SetBlobServer(CLI.Worker.BlobServer)
	}

	var redisURI string
	if CLI.Redis != "" {
		redisURI = CLI.Redis
	} else {
		redisURI = cfg.GetString("redis")
	}
	redisOpts, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		log.Fatal(err)
	}

	runner, err := tasks.NewEncoderRunner(
		s3storage, enc, tasks.NewResultWriter(redisOpts),
		tasks.WithLogger(zapadapter.NewKV(log.Desugar())),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Infow("starting worker")
	go conductor.StartWorker(redisOpts, CLI.Worker.Concurrency, runner, zapadapter.New(log.Desugar()))

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-stopChan
	log.Infof("caught an %v signal, shutting down...", sig)
	runner.Cleanup()
}

func readConfig(name string) (*viper.Viper, error) {
	cfg := viper.New()
	cfg.SetConfigName(name)

	ex, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cfg.AddConfigPath(filepath.Dir(ex))
	cfg.AddConfigPath(".")

	return cfg, cfg.ReadInConfig()
}
