package testservices

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-redis/redis/v8"
	dockertest "github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

type S3Options struct {
	AccessKey string
	SecretKey string
	Endpoint  string
}

type Teardown func() error

// Redis will spin up a redis container and return a connection options
// plus a tear down function that needs to be called to spin the container down.
func Redis() (*redis.Options, Teardown, error) {
	var err error
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to docker: %w", err)
	}

	resource, err := pool.Run("redis", "7", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not start resource: %w", err)
	}

	redisOpts := &redis.Options{
		Addr: fmt.Sprintf("localhost:%s", resource.GetPort("6379/tcp")),
	}

	if err = pool.Retry(func() error {
		db := redis.NewClient(redisOpts)
		err := db.Ping(context.Background()).Err()
		return err
	}); err != nil {
		return nil, nil, fmt.Errorf("could not connect to redis: %w", err)
	}

	return redisOpts, func() error {
		if err = pool.Purge(resource); err != nil {
			return fmt.Errorf("could not purge resource: %w", err)
		}
		return nil
	}, nil
}

// Minio will spin up a Minio container and return a connection options
// plus a tear down function that needs to be called to spin the container down.
func Minio() (*S3Options, Teardown, error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, nil, err
	}

	options := &dockertest.RunOptions{
		Repository: "minio/minio",
		Tag:        "latest",
		Cmd:        []string{"server", "/data"},
		// PortBindings: map[dc.Port][]dc.PortBinding{
		// 	"9000/tcp": []dc.PortBinding{{HostPort: "9000"}},
		// },
		Env: []string{"MINIO_ACCESS_KEY=MYACCESSKEY", "MINIO_SECRET_KEY=MYSECRETKEY"},
	}

	resource, err := pool.RunWithOptions(
		options,
		func(config *docker.HostConfig) {
			// set AutoRemove to true so that stopped container goes away by itself
			config.AutoRemove = true
			// config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not start resource: %w", err)
	}

	endpoint := fmt.Sprintf("localhost:%s", resource.GetPort("9000/tcp"))
	// or you could use the following, because we mapped the port 9000 to the port 9000 on the host
	// endpoint := "localhost:9000"

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	// the minio client does not do service discovery for you (i.e. it does not check if connection can be established), so we have to use the health check
	if err := pool.Retry(func() error {
		url := fmt.Sprintf("http://%s/minio/health/live", endpoint)
		resp, err := http.Get(url) // #nosec G107
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status code not OK")
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("Could not connect to docker: %w", err)
	}

	opts := &S3Options{
		AccessKey: "MYACCESSKEY",
		SecretKey: "MYSECRETKEY",
		Endpoint:  endpoint,
	}
	return opts, func() error {
		return pool.Purge(resource)
	}, nil
}
