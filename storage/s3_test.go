package storage

import (
	"context"
	"fmt"
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/docker/go-connections/nat"
	"github.com/lbryio/transcoder/library"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type s3Container struct {
	testcontainers.Container
	URI string
}

type s3suite struct {
	suite.Suite
	cleanup     func() error
	s3container *s3Container
	sdHash      string
	streamsPath string
}

func TestS3suite(t *testing.T) {
	suite.Run(t, new(s3suite))
}

func (s *s3suite) SetupSuite() {
	var err error

	rand.Seed(time.Now().UTC().UnixNano())

	s.s3container, err = setupS3(context.Background())
	s.Require().NoError(err)
}

func (s *s3suite) SetupTest() {
	s.streamsPath = s.T().TempDir()
	s.sdHash = randomdata.Alphanumeric(96)
	library.PopulateHLSPlaylist(s.T(), s.streamsPath, s.sdHash)
}

func (s *s3suite) TestPutDelete() {
	s3drv, err := InitS3Driver(
		S3Configure().
			Name("test").
			Endpoint(s.s3container.URI).
			Region("us-east-1").
			Credentials("s3-test", "s3-test").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)

	stream := library.InitStream(path.Join(s.streamsPath, s.sdHash), "")
	err = stream.GenerateManifest("url", "channel", s.sdHash)
	s.Require().NoError(err)

	err = s3drv.Put(stream, false)
	s.Require().NoError(err)

	sf, err := s3drv.GetFragment(stream.TID(), library.MasterPlaylistName)
	s.Require().NoError(err)
	s.Require().NotNil(sf)

	mf, err := s3drv.GetFragment(stream.TID(), library.ManifestName)
	s.Require().NoError(err)
	s.Require().NotNil(mf)

	err = s3drv.Put(stream, false)
	s.ErrorIs(err, ErrStreamExists)

	err = s3drv.Put(stream, true)
	s.NoError(err)

	err = s3drv.Delete(stream.TID())
	s.Require().NoError(err)

	deletedPieces := []string{"", library.MasterPlaylistName, "stream_0.m3u8", "stream_1.m3u8", "stream_2.m3u8", "stream_3.m3u8"}
	for _, n := range deletedPieces {
		p, err := s3drv.GetFragment(stream.TID(), n)
		s.NotNil(err)
		awsErr := err.(awserr.Error)
		s.Equal("NoSuchKey", awsErr.Code())
		s.Nil(p)
	}
}

func (s *s3suite) TearDownSuite() {
	// s.NoError(s.cleanup())
}

type TestLogConsumer struct {
	Msgs []string
}

func (g *TestLogConsumer) Accept(l testcontainers.Log) {
	g.Msgs = append(g.Msgs, string(l.Content))
	fmt.Println("YO", l.Content)
}

func setupS3(ctx context.Context) (*s3Container, error) {
	p, err := nat.NewPort("tcp", "9000")
	if err != nil {
		return nil, err
	}
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		// Cmd:          []string{"minio", "server", "/data"},
		// Cmd:          []string{"server", "--address", "0.0.0.0:9000", "./data"},
		// WaitingFor: wait.ForListeningPort("9000/tcp"),
		// WaitingFor: wait.ForLog("Finished loading IAM sub-system").WithStartupTimeout(10 * time.Second),
		// WaitingFor: wait.ForListeningPort(p),
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort(p), //.WithStartupTimeout(20 * time.Second),
		Env: map[string]string{
			"MINIO_ACCESS_KEY": "s3-test",
			"MINIO_SECRET_KEY": "s3-test",
		},
		Entrypoint: []string{"sh"},
		// Cmd:        []string{"-c", fmt.Sprintf("mkdir -p /data/%s && /usr/bin/minio server /data", bucket)},
		Cmd: []string{"-c", fmt.Sprintf("mkdir -p /data/%s && /usr/bin/minio server /data", "")},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          false,
	})
	if err != nil {
		return nil, err
	}

	g := TestLogConsumer{
		Msgs: []string{},
	}
	err = container.StartLogProducer(ctx)
	if err != nil {
		return nil, err
	}

	container.FollowOutput(&g)

	err = container.Start(ctx)
	if err != nil {
		return nil, err
	}

	err = container.StopLogProducer()
	if err != nil {
		return nil, err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(ctx, p)
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("http://%s:%s", ip, mappedPort.Port())

	return &s3Container{Container: container, URI: uri}, nil
}
