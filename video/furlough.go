package video

import (
	"context"
	"sort"
	"time"
)

func tailVideos(videos []*Video, maxSize uint64, call func(v *Video) error) (totalSize uint64, furloughedSize uint64, err error) {
	for _, v := range videos {
		totalSize += uint64(v.GetSize())
	}
	if maxSize >= totalSize {
		return
	}

	sort.Slice(videos, func(i, j int) bool { return videos[i].GetWeight() < videos[j].GetWeight() })
	for _, s := range videos {
		err := call(s)
		if err != nil {
			logger.Warnw("failed to execute function for video", "sd_hash", s.SDHash, "err", err)
			continue
		}
		furloughedSize += uint64(s.GetSize())
		logger.Debugf("furloughed: %v, left: %v", furloughedSize, totalSize-furloughedSize)
		if maxSize >= totalSize-furloughedSize {
			break
		}
	}

	return
}

// FurloughVideos deletes older videos locally, leaving them only on S3, keeping total size of
// local videos at maxSize.
func FurloughVideos(lib *Library, maxSize uint64) (uint64, uint64, error) {
	var items []*Video
	var err error
	if lib.remote == nil {
		items, err = lib.ListLocalOnly()
	} else {
		items, err = lib.ListLocal()
	}

	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, lib.Furlough)
}

// RetireVideos deletes older videos from S3, keeping total size of remote videos at maxSize.
func RetireVideos(lib *Library, maxSize uint64) (uint64, uint64, error) {
	items, err := lib.ListRemoteOnly()
	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, lib.Retire)
}

// RetireVideosLocal deletes videos from local filesystem, keeping total size of remote videos at maxSize.
func RetireVideosLocal(lib *Library, maxSize uint64) (uint64, uint64, error) {
	items, err := lib.ListRemoteOnly()
	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, func(v *Video) error {
		ll := logger.With("sd_hash", v.SDHash)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := lib.local.Delete(v.SDHash)
		if err != nil {
			ll.Warnw("failed to delete remote video", "err", err)
			return err
		}

		err = lib.queries.Delete(ctx, v.SDHash)
		if err != nil {
			ll.Warnw("failed to delete video record", "err", err)
			return err
		}

		ll.Infow("video retired", "url", v.URL, "size", v.GetSize(), "age", v.CreatedAt, "last_accessed", v.LastAccessed)
		return nil

	})
}
