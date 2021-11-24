package tower

import "errors"

var ErrRequestExists = errors.New("request is already being processed")
