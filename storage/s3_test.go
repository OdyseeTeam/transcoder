package storage

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/odyseeteam/transcoder/library"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/suite"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	minioAccessKey = "s3-test"
	minioSecretKey = randomdata.Alphanumeric(24)
)

type s3Container struct {
	testcontainers.Container
	URI string
}

type s3suite struct {
	suite.Suite
	s3container *s3Container
	sdHash      string
	streamsPath string
}

func TestS3suite(t *testing.T) {
	suite.Run(t, new(s3suite))
}

func (s *s3suite) SetupSuite() {
	var err error

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
			Credentials(minioAccessKey, minioSecretKey).
			Bucket("storage-s3-test").
			CreateBucket(). // This should be skipped for production/wasabi
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

type TestLogConsumer struct {
	Msgs []string
}

func (g *TestLogConsumer) Accept(l testcontainers.Log) {
	g.Msgs = append(g.Msgs, string(l.Content))
}

func setupS3(ctx context.Context) (*s3Container, error) {
	p, err := nat.NewPort("tcp", "9000")
	if err != nil {
		return nil, err
	}
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor:   wait.ForHTTP("/minio/health/ready").WithPort(p),
		Env: map[string]string{
			"MINIO_ROOT_USER":     minioAccessKey,
			"MINIO_ROOT_PASSWORD": minioSecretKey,
		},
		Entrypoint: []string{"sh"},
		Cmd:        []string{"-c", fmt.Sprintf("mkdir -p /data/%s && /usr/bin/minio server /data", "")},
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
