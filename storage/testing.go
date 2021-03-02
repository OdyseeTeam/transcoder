package storage

const (
	OpDelete = iota
	OpGetFragment
	OpPut
)

type StorageOp struct {
	Op     int
	SDHash string
}

type DummyStorage struct {
	LocalStorage
	Ops []StorageOp
}

func Dummy() *DummyStorage {
	return &DummyStorage{LocalStorage: LocalStorage{"/tmp/dummy_storage"}, Ops: []StorageOp{}}
}

func (s *DummyStorage) Delete(sdHash string) error {
	s.Ops = append(s.Ops, StorageOp{OpDelete, sdHash})
	return nil
}

func (s *DummyStorage) GetFragment(sdHash, name string) (StreamFragment, error) {
	s.Ops = append(s.Ops, StorageOp{OpGetFragment, sdHash})
	return nil, nil
}

func (s *DummyStorage) Put(lstream *LocalStream) (*RemoteStream, error) {
	s.Ops = append(s.Ops, StorageOp{OpGetFragment, lstream.sdHash})
	return &RemoteStream{url: "http://dummy/url"}, nil
}
