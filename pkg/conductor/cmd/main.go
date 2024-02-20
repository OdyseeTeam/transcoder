package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/odyseeteam/transcoder/encoder"
	"github.com/odyseeteam/transcoder/internal/version"
	"github.com/odyseeteam/transcoder/ladder"
	"github.com/odyseeteam/transcoder/library"
	ldb "github.com/odyseeteam/transcoder/library/db"
	"github.com/odyseeteam/transcoder/manager"
	"github.com/odyseeteam/transcoder/pkg/conductor"
	"github.com/odyseeteam/transcoder/pkg/conductor/metrics"
	"github.com/odyseeteam/transcoder/pkg/conductor/tasks"
	"github.com/odyseeteam/transcoder/pkg/dispatcher"
	"github.com/odyseeteam/transcoder/pkg/logging"
	"github.com/odyseeteam/transcoder/pkg/logging/zapadapter"
	"github.com/odyseeteam/transcoder/pkg/mfr"
	"github.com/odyseeteam/transcoder/pkg/migrator"
	"github.com/odyseeteam/transcoder/pkg/resolve"
	"github.com/odyseeteam/transcoder/storage"

	"github.com/alecthomas/kong"
	"github.com/fasthttp/router"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
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

	logger.Info("odysee transcoder", zap.String("version", version.Version))
	switch ctx.Command() {
	case "conductor":
		startConductor()
	case "worker":
		startWorker()
	case "migrate-up":
		migrateUp()
	case "migrate-down":
		migrateDown()
	case "validate-streams":
		validateStreams()
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

	if cfg.GetString("edgetoken") == "" {
		log.Warn("edge token not set")
	} else {
		resolve.SetEdgeToken(cfg.GetString("edgetoken"))
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
		library.SetLogger(logging.Create("library", logging.Prod))
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
	valid, broken, err := lib.ValidateStreams(
		CLI.ValidateStreams.Storage, CLI.ValidateStreams.Offset, CLI.ValidateStreams.Limit, CLI.ValidateStreams.Remove)
	if err != nil {
		log.Fatal("failed to validate streams:", err)
	}
	fmt.Printf("%v streams checked, %v valid, %v broken\n", len(valid)+len(broken), len(valid), len(broken))
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

func migrateUp() {

}

func migrateDown() {

}
