package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/OdyseeTeam/transcoder/encoder"
	"github.com/OdyseeTeam/transcoder/internal/config"
	"github.com/OdyseeTeam/transcoder/internal/version"
	"github.com/OdyseeTeam/transcoder/ladder"
	"github.com/OdyseeTeam/transcoder/library"
	ldb "github.com/OdyseeTeam/transcoder/library/db"
	"github.com/OdyseeTeam/transcoder/manager"
	"github.com/OdyseeTeam/transcoder/pkg/conductor"
	"github.com/OdyseeTeam/transcoder/pkg/conductor/metrics"
	"github.com/OdyseeTeam/transcoder/pkg/conductor/tasks"
	"github.com/OdyseeTeam/transcoder/pkg/dispatcher"
	"github.com/OdyseeTeam/transcoder/pkg/logging"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"
	"github.com/OdyseeTeam/transcoder/pkg/mfr"
	"github.com/OdyseeTeam/transcoder/pkg/migrator"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"
	"github.com/OdyseeTeam/transcoder/storage"

	"github.com/alecthomas/kong"
	"github.com/fasthttp/router"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"
)

var logger *zap.Logger

var CLI struct {
	migrator.CLI
	Conductor struct {
		HttpBind string `optional:"" help:"Address for HTTP server to listen on" default:"0.0.0.0:8080"`
	} `cmd:"" help:"Start conductor server"`
	Worker struct {
		StreamsDir  string `optional:"" help:"Directory for storing downloaded files"`
		OutputDir   string `optional:"" help:"Directory for storing encoder output files"`
		BlobServer  string `optional:"" help:"LBRY blobserver address."`
		Concurrency int    `optional:"" help:"Number of task slots" default:"5"`
		HttpBind    string `optional:"" help:"Address for prom metrics HTTP server to listen on" default:"0.0.0.0:8080"`
	} `cmd:"" help:"Start worker"`
	ValidateStreams struct {
		Remove  bool   `optional:"" help:"Remove broken streams from the database"`
		Storage string `help:"Storage name"`
		Offset  int32  `optional:"" help:"Starting stream index"`
		Limit   int32  `optional:"" help:"Stream count" default:"999999999"`
	} `cmd:"" help:"Verify a list of streams supplied by stdin"`
	Debug bool `optional:"" help:"Enable debug logging" default:"false"`
}

func main() {
	ctx := kong.Parse(&CLI)

	if CLI.Debug {
		logger = logging.Create("", logging.Dev).Desugar()
	} else {
		logger = logging.Create("", logging.Prod).Desugar()
	}

	logger.Info("odysee transcoder", zap.String("version", version.Version))
	switch ctx.Command() {
	case "conductor":
		startConductor()
	case "worker":
		startWorker()
	// case "migrate-up":
	// 	migrateUp()
	// case "migrate-down":
	// 	migrateDown()
	case "validate-streams":
		validateStreams()
	default:
		panic(ctx.Command())
	}
}

func startConductor() {
	log := logger.Sugar()
	cfg, err := config.ReadConductorConfig()
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
		resolve.SetLogger(logging.Create("resolve", logging.Prod))
	}

	libDB, err := migrator.ConnectDB(migrator.DefaultDBConfig().DSN(cfg.Library.DSN).AppName("library"), ldb.MigrationsFS)
	if err != nil {
		log.Fatal("library db initialization failed", err)
	}

	cleanStopChans := []chan struct{}{}

	storages, err := storage.InitS3Drivers(cfg.Storages)
	if err != nil {
		log.Fatal(err)
	}

	lib := library.New(library.Config{
		DB:       libDB,
		Storages: storages,
		Log:      zapadapter.NewKV(nil),
	})

	for _, scfg := range cfg.Storages {
		log.Infow("s3 storage configured", "name", scfg.Name, "endpoint", scfg.Endpoint)
		if scfg.MaxSize != "" {
			ch := library.SpawnLibraryCleaning(lib, scfg.Name, library.StringToSize(scfg.MaxSize))
			cleanStopChans = append(cleanStopChans, ch)
		}
	}

	if cfg.AdaptiveQueue.MinHits < 0 {
		log.Fatal("min hits cannot be below zero")
	}
	mgr := manager.NewManager(lib, uint(cfg.AdaptiveQueue.MinHits)) // nolint:gosec

	httpStopChan, _ := mgr.StartHttpServer(manager.HttpServerConfig{
		ManagerToken: cfg.Library.ManagerToken,
		Bind:         CLI.Conductor.HttpBind,
	})

	redisOpts, err := asynq.ParseRedisURI(cfg.Redis)
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

	for _, ch := range cleanStopChans {
		close(ch)
	}

	mgr.Pool().Stop()
	log.Infof("manager shut down")
}

func startWorker() {
	log := logger.Sugar()

	cfg, err := config.ReadWorkerConfig()
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
		resolve.SetLogger(logging.Create("resolve", logging.Prod))
	}

	if CLI.Worker.StreamsDir != "" {
		if err := makeWorkDir(CLI.Worker.StreamsDir); err != nil {
			log.Fatal(err)
		}
	}
	if CLI.Worker.OutputDir != "" {
		if err := makeWorkDir(CLI.Worker.OutputDir); err != nil {
			log.Fatal(err)
		}
	}

	storage, err := storage.InitS3Driver(cfg.Storage)
	if err != nil {
		log.Fatal("s3 driver initialization failed", err)
	}
	log.Infow("s3 storage configured", "name", cfg.Storage.Name, "endpoint", cfg.Storage.Endpoint)

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

	if cfg.EdgeToken == "" {
		log.Warn("edge token not set")
	} else {
		resolve.SetEdgeToken(cfg.EdgeToken)
	}

	redisOpts, err := asynq.ParseRedisURI(cfg.Redis)
	if err != nil {
		log.Fatal(err)
	}

	runner, err := tasks.NewEncoderRunner(
		storage, enc, tasks.NewResultWriter(redisOpts),
		tasks.WithLogger(zapadapter.NewKV(log.Desugar())),
		tasks.WithOutputDir(CLI.Worker.OutputDir),
		tasks.WithStreamsDir(CLI.Worker.StreamsDir),
	)
	if err != nil {
		log.Fatal(err)
	}

	metricsStopChan := make(chan struct{})
	startMetricsServer(CLI.Worker.HttpBind, metricsStopChan)

	log.Infow("starting worker")
	go conductor.StartWorker(redisOpts, CLI.Worker.Concurrency, runner, zapadapter.New(log.Desugar()))

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-stopChan
	log.Infof("caught an %v signal, shutting down...", sig)
	runner.Cleanup()
	close(metricsStopChan)
}

func validateStreams() {
	log := logger.Sugar()
	cfg, err := config.ReadConductorConfig()
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
		library.SetLogger(logging.Create("library", logging.Prod))
	}

	libDB, err := migrator.ConnectDB(migrator.DefaultDBConfig().DSN(cfg.Library.DSN).AppName("library"), ldb.MigrationsFS)
	if err != nil {
		log.Fatal("library db initialization failed", err)
	}

	storages, err := storage.InitS3Drivers(cfg.Storages)
	if err != nil {
		log.Fatal(storages)
	}

	lib := library.New(library.Config{
		DB:       libDB,
		Storages: storages,
		Log:      zapadapter.NewKV(nil),
	})
	valid, broken, err := lib.ValidateStreams(
		CLI.ValidateStreams.Storage, CLI.ValidateStreams.Offset, CLI.ValidateStreams.Limit, CLI.ValidateStreams.Remove)
	if err != nil {
		log.Fatal("failed to validate streams:", err)
	}
	fmt.Printf("%v streams checked, %v valid, %v broken\n", len(valid)+len(broken), len(valid), len(broken))
}

func startMetricsServer(bind string, stopChan chan struct{}) error {
	log := logger.Sugar()
	router := router.New()

	metrics.RegisterWorkerMetrics()
	router.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))

	log.Info("starting worker http server", "addr", bind)
	httpServer := &fasthttp.Server{
		Handler:          router.Handler,
		Name:             "worker",
		DisableKeepalive: true,
	}

	go func() {
		err := httpServer.ListenAndServe(bind)
		if err != nil {
			log.Fatalw("http server error", "err", err)
		}
	}()
	go func() {
		<-stopChan
		log.Infow("shutting down worker http server", "addr", bind)
		httpServer.Shutdown()
	}()

	return nil
}

func makeWorkDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to clean up work dir: %w", err)
	}
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create work dir: %w", err)
	}
	return nil
}
