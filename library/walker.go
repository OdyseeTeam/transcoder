package library

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/grafov/m3u8"
)

type StreamGetter func(path ...string) (io.ReadCloser, error)
type StreamProcessor func(fgName string, r io.ReadCloser) error

var SkipSegment = errors.New("skip fragment")

// WalkStream parses an HLS playlist, calling `getFn` to load and `processFn`
// for the master playlist located in `baseURI`, subplaylists and all segments contained within.
func WalkStream(baseURI string, getFn StreamGetter, processFn StreamProcessor) error {
	parsePlaylist := func(name string) (m3u8.Playlist, error) {
		r, err := getFn(baseURI, name)
		if err != nil {
			return nil, fmt.Errorf("error getting stream item %v: %w", name, err)
		}
		if r == nil {
			processFn(name, r)
			return nil, errors.New("empty playlist")
		}
		dr, err := read(r)
		if err != nil {
			return nil, fmt.Errorf("error reading stream item %v: %w", name, err)
		}

		err = processFn(name, io.NopCloser(dr))
		if err != nil {
			return nil, fmt.Errorf("error processing stream item %v: %w", name, err)
		}
		_, err = dr.Seek(0, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("error seeking in item %v: %w", name, err)
		}

		p, _, err := m3u8.DecodeFrom(dr, true)
		if err != nil {
			return nil, fmt.Errorf("error decoding stream item %v: %w", name, err)
		}
		return p, nil
	}

	pl, err := parsePlaylist(MasterPlaylistName)
	if err != nil {
		return err
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, varpl := range masterpl.Variants {
		p, err := parsePlaylist(varpl.URI)
		if err != nil {
			return err
		}
		mediapl := p.(*m3u8.MediaPlaylist)

		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			r, err := getFn(baseURI, seg.URI)
			if errors.Is(err, SkipSegment) {
				continue
			}
			if err != nil {
				return fmt.Errorf("error getting stream item %v: %w", seg.URI, err)
			}
			err = processFn(seg.URI, r)
			if r != nil {
				r.Close()
			}
			if err != nil {
				return fmt.Errorf("error processing stream item %v: %w", varpl.URI, err)
			}
		}
	}
	return nil
}

func read(r io.ReadCloser) (io.ReadSeeker, error) {
	d, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		return nil, err
	}
	dr := bytes.NewReader(d)
	return dr, nil
}
