package storage

import (
	"context"
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/OdyseeTeam/transcoder/internal/config"
	"github.com/OdyseeTeam/transcoder/library"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/suite"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	minioAccessKey = "s3-test"
	minioSecretKey = randomdata.Alphanumeric(24)

	fragments = []string{library.MasterPlaylistName, "stream_0.m3u8", "stream_1.m3u8", "stream_2.m3u8", "stream_3.m3u8"}
)

type s3Container struct {
	testcontainers.Container
	URI string
}

type s3suite struct {
	suite.Suite
	s3container *s3Container
	s3driver    *S3Driver
}

func TestS3suite(t *testing.T) {
	suite.Run(t, new(s3suite))
}

func (s *s3suite) SetupSuite() {
	var err error

	s.s3container, err = setupS3(context.Background())
	s.Require().NoError(err)

	s3driver, err := InitS3Driver(config.S3Config{
		Name:         "test",
		Endpoint:     s.s3container.URI,
		Region:       "us-east-1",
		Key:          minioAccessKey,
		Secret:       minioSecretKey,
		Bucket:       "storage-s3-test",
		CreateBucket: true,
	})
	s.Require().NoError(err)
	s.s3driver = s3driver
}

func (s *s3suite) SetupTest() {

}

func (s *s3suite) TestPut() {
	s3drv := s.s3driver
	stream := s.putStream()

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
}

func (s *s3suite) TestDelete() {
	s3drv := s.s3driver
	stream := s.putStream()

	err := s3drv.Delete(stream.TID())
	s.Require().NoError(err)

	for _, n := range fragments {
		p, err := s3drv.GetFragment(stream.TID(), n)
		s.NotNil(err)
		var noSuchKey *types.NoSuchKey
		s.True(errors.As(err, &noSuchKey))
		s.Nil(p)
	}
}

func (s *s3suite) TestDeleteFragments() {
	s3drv := s.s3driver
	stream := s.putStream()

	err := s3drv.DeleteFragments(stream.TID(), fragments)
	s.Require().NoError(err)

	for _, n := range fragments {
		p, err := s3drv.GetFragment(stream.TID(), n)
		s.NotNil(err)
		var noSuchKey *types.NoSuchKey
		s.True(errors.As(err, &noSuchKey))
		s.Nil(p)
	}
}

func (s *s3suite) putStream() *library.Stream {
	streamsPath := s.T().TempDir()
	sdHash := randomdata.Alphanumeric(96)
	library.PopulateHLSPlaylist(s.T(), streamsPath, sdHash)

	stream := library.InitStream(path.Join(streamsPath, sdHash), "")
	err := stream.GenerateManifest("url", "channel", sdHash)
	s.Require().NoError(err)

	err = s.s3driver.Put(stream, false)
	s.Require().NoError(err)
	return stream
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
