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
)

type DispatcherSuite struct {
	suite.Suite
	db *db.DB
}

type testWorkload struct {
	sync.Mutex
	doCalled  int
	seenTasks []Task
}

func (wl *testWorkload) Do(t Task) error {
	wl.Lock()
	wl.doCalled++
	wl.seenTasks = append(wl.seenTasks, t)
	wl.Unlock()
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
	d := New()
	wl := testWorkload{seenTasks: []Task{}}
	d.Start(20, &wl)

	SetLogger(logging.Create("dispatcher", logging.Prod))

	grc := runtime.NumGoroutine()

	for range [500]bool{} {
		d.Dispatch(Task{URL: randomString(25), SDHash: randomString(96)})
	}

	time.Sleep(1 * time.Second)
	d.Stop()

	s.Equal(runtime.NumGoroutine(), grc)
	s.Equal(500, len(wl.seenTasks))
	s.Equal(500, wl.doCalled)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
