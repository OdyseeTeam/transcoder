package storage

type NullDriver struct{}

func (d NullDriver) Put(stream *LocalStream) (*RemoteStream, error) {
	logger.Warn("storage driver not configured")
	return &RemoteStream{url: "null:" + stream.sdHash}, nil
}

func (d NullDriver) Delete(sdHash string) error {
	logger.Warn("storage driver not configured")
	return nil
}

func (d NullDriver) GetFragment(s, n string) (StreamFragment, error) {
	logger.Warn("storage driver not configured")
	return nil, nil
}
