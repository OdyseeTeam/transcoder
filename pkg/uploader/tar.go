package uploader

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lbryio/transcoder/storage"
)

// packStream takes a sourceDir and puts it into a TAR archive at tarPath,
// returning SHA256 checksum of stream files.
func packStream(stream *storage.LightLocalStream, tarPath string) ([]byte, error) {
	l := log.With("source_dir", stream.Path)

	tarfile, err := os.Create(tarPath)
	if err != nil {
		return nil, err
	}
	defer tarfile.Close()

	tw := tar.NewWriter(tarfile)

	hash := storage.GetHash()

	err = stream.Walk(func(fi fs.FileInfo, fullPath, name string) error {
		header, err := tar.FileInfoHeader(fi, name)
		if err != nil {
			return err
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(fullPath)
		if err != nil {
			return err
		}
		defer file.Close()

		w := io.MultiWriter(tw, hash)
		n, err := io.Copy(w, file)
		if err != nil {
			return err
		}
		l.Debug("entry packaged", "name", name, "size", n)
		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// unpackStream unpacks TAR file at tarPath into dstPath,
// calculating SHA256 checksum of files.
func unpackStream(tarReader io.ReadCloser, dstPath string) (*storage.LightLocalStream, error) {
	var size int64

	if err := os.MkdirAll(dstPath, os.ModePerm); err != nil {
		return nil, err
	}

	hash := storage.GetHash()
	tr := tar.NewReader(tarReader)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		target := filepath.Join(dstPath, header.Name)
		// Guard against https://snyk.io/research/zip-slip-vulnerability
		if !strings.HasPrefix(target, filepath.Clean(dstPath)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			return nil, fmt.Errorf("directory encountered in the archive: %s", target)
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return nil, err
			}
			w := io.MultiWriter(outFile, hash)
			n, err := io.Copy(w, tr)
			outFile.Close()
			if err != nil {
				return nil, err
			}
			size += n
		}
	}

	return &storage.LightLocalStream{Path: dstPath, Checksum: hash.Sum(nil), Size: size}, nil
}
