package uploader

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/karrick/godirwalk"
)

// packStream takes a sourceDir and puts it into a TAR archive at tarPath,
// returning SHA256 checksum of stream files.
func packStream(sourceDir, tarPath string) ([]byte, error) {
	l := log.With("source_dir", sourceDir)

	tarfile, err := os.Create(tarPath)
	if err != nil {
		return nil, err
	}
	defer tarfile.Close()

	tw := tar.NewWriter(tarfile)

	info, err := os.Stat(sourceDir)
	if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%v is not a directory", sourceDir)
	}

	hash := sha256.New()

	err = godirwalk.Walk(sourceDir, &godirwalk.Options{
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				l.Debug("skipping directory entry", "name", de.Name())
				if fullPath != sourceDir {
					return fmt.Errorf("%v is a directory while only files are expected here", fullPath)
				}
				return nil
			}
			l.Debug("proceeding with entry", "full_path", fullPath)
			fi, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(fi, de.Name())
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
			l.Debug("entry packaged", "name", de.Name(), "size", n)
			return nil
		}})
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
func unpackStream(tarReader io.ReadCloser, dstPath string) ([]byte, error) {
	if err := os.MkdirAll(dstPath, os.ModePerm); err != nil {
		return nil, err
	}

	hash := sha256.New()
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
			of, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return nil, err
			}
			w := io.MultiWriter(of, hash)
			if _, err := io.Copy(w, tr); err != nil {
				return nil, err
			}
			of.Close()
		}
	}
	return hash.Sum(nil), nil
}
