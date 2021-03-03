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

type fileLoader func(rootPath ...string) ([]byte, error)
type fileProcessor func(data []byte, name string) error

// HLSPlaylistDive processes HLS streams, calling `load` to load and `process`
// for each master/child playlists and all the files they reference.
func HLSPlaylistDive(rootPath string, loader fileLoader, processor fileProcessor) (int64, error) {
	var streamSize int64

	load := func(path ...string) (io.Reader, error) {
		data, err := loader(path...)
		if err != nil {
			return nil, err
		}

		streamSize += int64(len(data))
		err = processor(data, path[len(path)-1])
		if err != nil {
			return nil, errors.Wrapf(err, `error processing stream item "%v"`, path[len(path)-1])
		}
		return bytes.NewReader(data), err
	}

	data, err := load(rootPath, MasterPlaylistName)
	if err != nil {
		return streamSize, errors.Wrapf(err, `error loading playlist "%v"`, MasterPlaylistName)
	}

	pl, _, err := m3u8.DecodeFrom(data, true)
	if err != nil {
		return streamSize, errors.Wrapf(err, `error decoding playlist "%v"`, rootPath)
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		data, err := load(rootPath, plv.URI)
		if err != nil {
			return streamSize, errors.Wrapf(err, `error loading playlist variant "%v"`, plv.URI)
		}

		p, _, err := m3u8.DecodeFrom(data, true)
		if err != nil {
			return streamSize, errors.Wrapf(err, `error decoding playlist variant "%v"`, plv.URI)
		}
		mediapl := p.(*m3u8.MediaPlaylist)

		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			_, err := load(rootPath, seg.URI)
			if err != nil {
				return streamSize, errors.Wrapf(err, `error loading fragment "%v"`, seg.URI)
			}
		}
	}
	return streamSize, nil
}
