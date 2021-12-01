package video

import (
	"sort"
)

func tailVideos(items []*Video, maxSize uint64, call func(v *Video) error) (totalSize uint64, furloughedSize uint64, err error) {
	for _, v := range items {
		totalSize += uint64(v.GetSize())
	}
	if maxSize >= totalSize {
		return
	}

	sort.Slice(items, func(i, j int) bool { return items[i].GetWeight() < items[j].GetWeight() })
	for _, s := range items {
		err := call(s)
		if err != nil {
			return totalSize, furloughedSize, err
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
