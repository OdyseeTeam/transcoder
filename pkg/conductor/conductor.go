package conductor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lbryio/transcoder/library"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/conductor/tasks"
	"github.com/lbryio/transcoder/pkg/logging"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
)

type Conductor struct {
	rdb            redis.UniversalClient
	asynqClient    *asynq.Client
	asynqInspector *asynq.Inspector
	library        *library.Library
	incoming       <-chan *manager.TranscodingRequest
	stopChan       chan struct{}
	options        *ConductorOptions
}

type ConductorOptions struct {
	Logger logging.KVLogger
}

func WithLogger(logger logging.KVLogger) func(options *ConductorOptions) {
	return func(options *ConductorOptions) {
		options.Logger = logger
	}
}

func NewConductor(
	redisOpts asynq.RedisConnOpt, incoming <-chan *manager.TranscodingRequest, library *library.Library,
	optionFuncs ...func(*ConductorOptions),
) (*Conductor, error) {
	options := &ConductorOptions{
		Logger: logging.NoopKVLogger{},
	}
	for _, optionFunc := range optionFuncs {
		optionFunc(options)
	}
	c := &Conductor{
		asynqClient:    asynq.NewClient(redisOpts),
		asynqInspector: asynq.NewInspector(redisOpts),
		rdb:            redisOpts.MakeRedisClient().(redis.UniversalClient),
		stopChan:       make(chan struct{}),
		options:        options,
		incoming:       incoming,
		library:        library,
	}
	return c, nil
}

func StartWorker(redisOpts asynq.RedisConnOpt, concurrency int, runner *tasks.EncoderRunner, log logging.Logger) {
	srv := asynq.NewServer(
		redisOpts,
		asynq.Config{
			// Specify how many concurrent workers to use
			Concurrency: concurrency,
			// Optionally specify multiple queues with different priority.
			// Queues: map[string]int{
			// 	"critical": 6,
			// 	"default":  3,
			// 	"low":      1,
			// },
			Logger: log,
		},
	)

	// mux maps a type to a handler
	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeTranscodingRequest, runner.Run)

	if err := srv.Run(mux); err != nil {
		log.Fatal("could not run server: %v", err)
	}
}

func (c *Conductor) Start() {
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		for {
			select {
			case <-t.C:
				err := c.PutLoad()
				if err != nil {
					c.options.Logger.Error("work cycle failed", "err", err)
				}
			case <-c.stopChan:
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case <-c.stopChan:
				return
			default:
				err := c.ProcessNextResult()
				if err != nil {
					c.options.Logger.Error("result cycle failed", "err", err)
				}
			}
		}
	}()
}

func (c *Conductor) Stop() {
	close(c.stopChan)
	c.rdb.Close()
	c.asynqClient.Close()
	c.asynqInspector.Close()
}

func (c *Conductor) PutLoad() error {
	servers, err := c.asynqInspector.Servers()
	if err != nil {
		return err
	}
	spares := 0
	for _, s := range servers {
		c.options.Logger.Info("inspecting worker", "wid", s.Host, "concurrency", s.Concurrency, "active", len(s.ActiveWorkers))
		spares += s.Concurrency - len(s.ActiveWorkers)
	}
	for i := 0; i < spares; i++ {
		err := c.DispatchNextTask()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Conductor) DispatchNextTask() error {
	req := &tasks.TranscodingRequest{}
	trReq := <-c.incoming
	req.URL = trReq.URI
	req.SDHash = trReq.SDHash
	t, err := tasks.NewTranscodingTask(*req)
	if err != nil {
		return fmt.Errorf("task creation error: %w", err)
	}
	info, err := c.asynqClient.Enqueue(
		t,
		asynq.Unique(48*time.Hour),
		asynq.Timeout(48*time.Hour),
		asynq.Retention(72*time.Hour),
		// asynq.Queue("critical"),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return c.DispatchNextTask()
	}
	if err != nil {
		return fmt.Errorf("task enqueue error: %w", err)
	}
	c.options.Logger.Info("enqueued task", "tid", info.ID, "queue", info.Queue)
	return nil
}

func (c *Conductor) ProcessNextResult() error {
	res := &tasks.TranscodingResult{}
	r, err := c.rdb.BLPop(context.Background(), 0, tasks.QueueTranscodingResults).Result()
	if err != nil {
		return fmt.Errorf("message reading error: %w", err)
	}
	c.options.Logger.Debug("result message received", "body", r[1])
	err = res.FromString(r[1])
	if err != nil {
		return fmt.Errorf("message parsing error: %w", err)
	}
	if err := c.library.AddRemoteStream(*res.Stream); err != nil {
		// ll.Info("error adding remote stream", "err", err, "stream", d.RemoteStream)
		// metrics.TranscodingRequestsErrors.With(labels).Inc()
		return fmt.Errorf("failed to add remote stream: %w", err)
	}
	c.options.Logger.Info(
		"remote stream added", "url", res.Stream.URL(), "tid", res.Stream.TID(), "sd_hash", res.Stream.SDHash())
	return nil
}
