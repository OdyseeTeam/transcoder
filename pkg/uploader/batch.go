package uploader

import (
	"context"
	"fmt"

	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/storage"
)

type BatchError struct {
	Stream storage.LightLocalStream
	Err    error
}

type BatchUploader struct {
	inlet chan batchItem
	stop  chan struct{}
	log   logging.KVLogger
}

type batchItem struct {
	ls       *storage.LightLocalStream
	progress chan float32
	errc     chan error
	done     chan struct{}
}

func StartBatchUploader(u *Uploader, concurrency int) BatchUploader {
	b := BatchUploader{
		inlet: make(chan batchItem, 1000),
		stop:  make(chan struct{}),
		log:   u.log,
	}

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case <-b.stop:
					return
				case item := <-b.inlet:
					ctx := context.Background()
					ls := item.ls
					if ls.Manifest.Tower == nil {
						item.errc <- fmt.Errorf("no tower credentials for stream %v", ls.SDHash)
					} else {
						item.progress <- 0.0
						b.log.Info("preparing to upload batch item", "ls", ls)
						err := u.Upload(ctx, ls.Path, ls.Manifest.Tower.CallbackURL, ls.Manifest.Tower.Token)
						if err != nil {
							b.log.Error("error uploading", "err", err)
							item.errc <- err
						} else {
							item.progress <- 100
							b.log.Info("batch item uploaded", "ls", ls)
						}
					}
					close(item.errc)
					close(item.done)
				}
			}
		}()
	}

	return b
}

func (b BatchUploader) Upload(ls *storage.LightLocalStream) (<-chan float32, <-chan struct{}, <-chan error) {
	item := batchItem{ls: ls, progress: make(chan float32, 1000), errc: make(chan error, 1), done: make(chan struct{})}
	b.inlet <- item
	return item.progress, item.done, item.errc
}

func (b BatchUploader) UploadDir(path string) error {
	streams := []*storage.LightLocalStream{}
	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			if !de.IsDir() || fullPath == path || !storage.SDHashRe.Match([]byte(de.Name())) {
				return nil
			}
			ls, err := storage.OpenLocalStream(fullPath)
			if err != nil {
				return err
			}
			err = ls.ReadManifest()
			if err != nil {
				return err
			}
			if ls.Manifest.Tower == nil {
				return fmt.Errorf("no tower credentials for stream %v", ls.SDHash)
			}
			streams = append(streams, ls)
			return nil
		}})
	if err != nil {
		return err
	}

	b.log.Info(fmt.Sprintf("%v streams discovered, uploading", len(streams)))
	for _, ls := range streams {
		p, done, errc := b.Upload(ls)
		go func() {
			for {
				select {
				case <-p:
				case <-done:
					err := <-errc
					if err != nil {
						b.log.Error("upload failed", "err", err)
					}
				}
			}
		}()
	}
	return nil
}
