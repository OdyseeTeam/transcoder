package dispatcher

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/lbryio/transcoder/pkg/logging"

	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type DispatcherSuite struct {
	suite.Suite
}

type testWorker struct {
	sync.Mutex
	called    int
	seenTasks []string
}

func (worker *testWorker) Work(t Task) error {
	worker.Lock()
	worker.called++
	pl := t.Payload.(struct{ URL, SDHash string })
	worker.seenTasks = append(worker.seenTasks, pl.URL+pl.SDHash)
	worker.Unlock()
	t.SetResult(pl.URL + pl.SDHash)
	return nil
}

type slowWorker struct {
	sync.Mutex
	called    int
	seenTasks []string
}

func (worker *slowWorker) Work(t Task) error {
	time.Sleep(1 * time.Second)
	return nil
}

func TestDispatcherSuite(t *testing.T) {
	suite.Run(t, new(DispatcherSuite))
}

func (s *DispatcherSuite) SetupSuite() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func (s *DispatcherSuite) SetupTest() {
}

func (s *DispatcherSuite) TestDispatcher() {
	defer goleak.VerifyNone(s.T())

	worker := testWorker{seenTasks: []string{}}
	d := Start(20, &worker, 1000)

	SetLogger(logging.Create("dispatcher", logging.Prod))
	results := []*Result{}

	for range [500]bool{} {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
		results = append(results, r)
	}

	time.Sleep(100 * time.Millisecond)

	s.Equal(500, len(worker.seenTasks))
	s.Equal(500, worker.called)
	for _, r := range results {
		s.Require().True(r.Done())
		s.Require().Equal(25+96, len(r.Value().(string)))
	}

	d.Stop()
}

func (s *DispatcherSuite) TestBlockingDispatch() {
	defer goleak.VerifyNone(s.T())

	worker := testWorker{seenTasks: []string{}}
	d := Start(5, &worker, 0)

	results := []*Result{}

	for range [20]bool{} {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
		results = append(results, r)
	}

	time.Sleep(100 * time.Millisecond)

	s.Equal(20, len(worker.seenTasks))
	s.Equal(20, worker.called)
	for _, r := range results {
		s.Require().True(r.Done())
	}

	d.Stop()
}

func (s *DispatcherSuite) TestDispatcherLeaks() {
	worker := testWorker{seenTasks: []string{}}
	d := Start(20, &worker, 1000)

	grc := runtime.NumGoroutine()

	SetLogger(logging.Create("dispatcher", logging.Prod))
	for range [10000]bool{} {
		d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
	}

	time.Sleep(5 * time.Second)
	s.Equal(grc, runtime.NumGoroutine())
	d.Stop()
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
