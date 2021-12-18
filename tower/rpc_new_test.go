package tower

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"

	"github.com/stretchr/testify/suite"
)

type rpcSuite struct {
	suite.Suite
	tower  *towerRPC
	worker *workerRPC
}

func TestRPCSuite(t *testing.T) {
	suite.Run(t, new(rpcSuite))
}

func (s *rpcSuite) SetupTest() {
	var err error
	s.tower, err = newTowerRPC(
		"amqp://guest:guest@localhost/",
		"postgres://postgres:odyseeteam@localhost/postgres",
		zapadapter.NewKV(nil))
	s.Require().NoError(err)
	s.Require().NoError(s.tower.deleteQueues())
	s.Require().NoError(s.tower.declareQueues())
}

func (s *rpcSuite) TearDownTest() {
	s.NoError(s.tower.deleteQueues())
	s.tower.publisher.StopPublishing()
	s.tower.consumer.StopConsuming("", true)
}

func (s *rpcSuite) TestWorkRequests() {
	var activeTaskCounter int
	workersSeen := map[string]bool{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		// Simulate shipping out tasks and reading progress
		activeTasks, err := s.tower.startConsumingWorkRequests()
		s.Require().NoError(err)
		defer wg.Done()
		for at := range activeTasks {
			activeTaskCounter++
			workersSeen[at.workerID] = true
			wg.Add(1)
			go func(at *activeTask) {
				defer wg.Done()
				// This represents the total task timeout. If no progress received during it, the task will be canceled.
				timeout := 5 * time.Second
				ctx, cancel := context.WithCancel(context.Background())
				t := time.AfterFunc(timeout, cancel)
				var total float32
				at.SendPayload(MsgTranscodingTask{Ref: randomdata.Alphanumeric(8)})
			ProgressLoop:
				for {
					select {
					case p := <-at.progress:
						total += p.Percent
						t.Reset(timeout)
					case <-ctx.Done():
						s.FailNowf("unexpected timeout waiting for task progress", "%s timed out", at.workerID)
					case <-at.done:
						break ProgressLoop
					}
				}
				cancel()
				s.EqualValues(550, total)
			}(at)
			if activeTaskCounter >= 30 {
				break
			}
		}
	}()

	// Fire up workers, send out work requests
	for i := 0; i < 10; i++ {
		rpc, err := newrpc("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
		s.Require().NoError(err)
		s.worker = &workerRPC{rpc: rpc}
		s.worker.id = fmt.Sprintf("testworker-%v", i)

		taskChan, err := s.worker.startWorking(3) // this is sending out work requests
		s.Require().NoError(err)

		go func() {
			// Simulate work
			for mtt := range taskChan {
				for i := 0; i <= 10; i++ {
					mtt.progress <- taskProgress{Percent: float32(i * 10)}
					time.Sleep(50 * time.Millisecond)
				}
				mtt.result <- taskResult{}
			}
		}()
	}

	wg.Wait()
}

func (s *rpcSuite) TestWorkRequestReject() {
	rpc, err := newrpc("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	worker := &workerRPC{rpc: rpc}
	worker.id = "testworker-1"

	_, err = worker.startWorking(1) // this is sending out work requests
	s.Require().NoError(err)
	worker.Stop()

	time.Sleep(4 * time.Second)
	activeTasks, err := s.tower.startConsumingWorkRequests()
	s.Require().NoError(err)
	at := <-activeTasks
	at.SendPayload(MsgTranscodingTask{Ref: randomdata.Alphanumeric(8)})
	time.Sleep(4 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	select {
	case <-at.reject:
	case <-at.progress:
		s.FailNow("unexpected progress received")
	case <-ctx.Done():
		s.FailNowf("timed out waiting for task rejection", "%s timed out", at.workerID)
	case <-at.done:
		s.FailNow("unexpected done signal received")
	}
}
