package dispatcher

import (
	"math/rand"
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
	d := Start(20, &wl)

	SetLogger(logging.Create("dispatcher", logging.Prod))
	dones := []chan bool{}

	for range [500]bool{} {
		done := d.Dispatch(struct{ URL, SDHash string }{URL: randomString(25), SDHash: randomString(96)})
		dones = append(dones, done)
	}

	time.Sleep(1 * time.Second)

	s.Equal(500, len(wl.seenTasks))
	s.Equal(500, wl.doCalled)
	for _, done := range dones {
		s.True(Done(done))
	}
	d.Stop()
	time.Sleep(1 * time.Second)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
