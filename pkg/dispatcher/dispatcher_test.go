package dispatcher

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type DispatcherSuite struct {
	suite.Suite
}

type testWorker struct {
	sync.Mutex
	seenTasks []string
}

func (w *testWorker) Work(t Task) error {
	w.Lock()
	defer w.Unlock()
	pl := t.Payload.(struct{ URL, SDHash string })
	w.seenTasks = append(w.seenTasks, pl.URL+pl.SDHash)
	t.SetResult(pl.URL + pl.SDHash)
	return nil
}

func TestDispatcherSuite(t *testing.T) {
	suite.Run(t, new(DispatcherSuite))
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
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomdata.Alphanumeric(25), SDHash: randomdata.Alphanumeric(96)})
		results = append(results, r)
	}

	time.Sleep(3000 * time.Millisecond)

	for _, r := range results {
		v := <-r.Value()
		s.Require().Equal(25+96, len(v.(string)))
		s.Require().True(r.Done())
	}
	s.Equal(500, len(worker.seenTasks))

	d.Stop()
}

func (s *DispatcherSuite) TestBlockingDispatch() {
	defer goleak.VerifyNone(s.T())

	worker := testWorker{seenTasks: []string{}}
	d := Start(5, &worker, 0)

	results := []*Result{}

	for range [20]bool{} {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomdata.Alphanumeric(25), SDHash: randomdata.Alphanumeric(96)})
		results = append(results, r)
	}

	for _, r := range results {
		v := <-r.Value()
		s.Require().Equal(25+96, len(v.(string)))
		s.Require().True(r.Done())
	}
	s.Equal(20, len(worker.seenTasks))

	d.Stop()
}

func (s *DispatcherSuite) TestDispatcherLeaks() {
	worker := testWorker{seenTasks: []string{}}
	results := [10000]*Result{}
	d := Start(20, &worker, 1000)
	grCount := runtime.NumGoroutine()

	SetLogger(logging.Create("dispatcher", logging.Prod))

	for i := 0; i < 10000; i++ {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomdata.SillyName(), SDHash: randomdata.Alphanumeric(64)})
		results[i] = r
	}

	time.Sleep(500 * time.Millisecond)
	s.Equal(grCount+10000, runtime.NumGoroutine())

	for _, r := range results {
		<-r.Value()
	}
	time.Sleep(3000 * time.Millisecond)
	s.Equal(grCount, runtime.NumGoroutine())

	d.Stop()
}
