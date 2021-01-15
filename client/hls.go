package client

import (
	"bytes"
	"io"

	"github.com/grafov/m3u8"
	"github.com/pkg/errors"
)

const (
	MasterPlaylistName = "master.m3u8"
)

type fileLoader func(path ...string) (data []byte, err error)
type fileProcessor func(data []byte, name string) (err error)

func playlistDive(rootPath string, _load fileLoader, process fileProcessor) (int64, error) {
	var streamSize int64

	load := func(path ...string) (io.Reader, error) {
		data, err := _load(path...)
		if err != nil {
			return nil, err
		}

		streamSize += int64(len(data))
		err = process(data, path[len(path)-1])
		if err != nil {
			return nil, errors.Wrapf(err, `error processing stream item "%v"`, path[len(path)-1])
		}
		return bytes.NewReader(data), err
	}

	data, err := load(rootPath, "master.m3u8")
	if err != nil {
		return streamSize, err
	}

	pl, _, err := m3u8.DecodeFrom(data, true)
	if err != nil {
		return streamSize, err
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		data, err := load(rootPath, plv.URI)
		if err != nil {
			return streamSize, err
		}

		p, _, err := m3u8.DecodeFrom(data, true)
		if err != nil {
			return streamSize, err
		}
		mediapl := p.(*m3u8.MediaPlaylist)

		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			_, err := load(rootPath, seg.URI)
			if err != nil {
				return streamSize, err
			}
		}
	}
	return streamSize, nil
}
