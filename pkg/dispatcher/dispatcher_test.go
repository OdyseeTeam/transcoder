package dispatcher

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/pkg/logging"

	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type DispatcherSuite struct {
	suite.Suite
	db *db.DB
}

type testWorkload struct {
	sync.Mutex
	doCalled  int
	seenTasks []string
}

func (wl *testWorkload) Do(t Task) error {
	wl.Lock()
	wl.doCalled++
	pl := t.Payload.(struct{ URL, SDHash string })
	wl.seenTasks = append(wl.seenTasks, pl.URL+pl.SDHash)
	wl.Unlock()
	return nil
}

type slowWorkload struct {
	sync.Mutex
	doCalled  int
	seenTasks []string
}

func (wl *slowWorkload) Do(t Task) error {
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

	wl := testWorkload{seenTasks: []string{}}
	d := Start(20, &wl, 1000)

	SetLogger(logging.Create("dispatcher", logging.Prod))
	results := []*Result{}

	for range [500]bool{} {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
		results = append(results, r)
	}

	time.Sleep(100 * time.Millisecond)

	s.Equal(500, len(wl.seenTasks))
	s.Equal(500, wl.doCalled)
	for _, r := range results {
		s.Require().True(r.Done())
	}

	d.Stop()
}

func (s *DispatcherSuite) TestBlockingDispatch() {
	defer goleak.VerifyNone(s.T())

	wl := testWorkload{seenTasks: []string{}}
	d := Start(5, &wl, 0)

	results := []*Result{}

	for range [20]bool{} {
		r := d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
		results = append(results, r)
	}

	time.Sleep(100 * time.Millisecond)

	s.Equal(20, len(wl.seenTasks))
	s.Equal(20, wl.doCalled)
	for _, r := range results {
		s.Require().True(r.Done())
	}

	d.Stop()
}

func (s *DispatcherSuite) TestDispatcherLeaks() {
	wl := testWorkload{seenTasks: []string{}}
	d := Start(20, &wl, 1000)

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
