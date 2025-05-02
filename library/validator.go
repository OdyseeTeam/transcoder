package library

import (
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/pkg/errors"
)

type ValidationResult struct {
	URL              string
	Present, Missing []string
}

func ValidateStream(baseURL string, failFast bool, skipSegments bool) (*ValidationResult, error) {
	vr := &ValidationResult{
		URL:     baseURL,
		Missing: []string{},
		Present: []string{},
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	err := WalkStream(baseURL,
		func(p ...string) (io.ReadCloser, error) {
			var (
				r   *http.Response
				err error
			)
			url := strings.Join(p, "/")
			if path.Ext(p[len(p)-1]) == ".ts" {
				if skipSegments {
					return nil, ErrSkipSegment
				}
				r, err = http.Head(url) // #nosec G107
			} else {
				r, err = http.Get(url) // #nosec G107
			}
			logger.Debugf("checked %s [%v]", p, r.StatusCode)
			if err != nil {
				return nil, err
			}
			if r.StatusCode != http.StatusOK {
				return nil, nil
			}
			return r.Body, nil
		},
		func(fgName string, r io.ReadCloser) error {
			if r == nil {
				logger.Debugf("missing: %s", fgName)
				vr.Missing = append(vr.Missing, fgName)
				if failFast {
					return errors.New("broken stream")
				}
			} else {
				vr.Present = append(vr.Present, fgName)
			}
			return nil
		},
	)
	return vr, err
}
