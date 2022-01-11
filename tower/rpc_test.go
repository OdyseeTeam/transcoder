package tower

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/queue"

	"github.com/stretchr/testify/suite"
)

type rpcSuite struct {
	suite.Suite
	tower     *towerRPC
	db        *sql.DB
	dbCleanup queue.TestDBCleanup
}

func TestRPCSuite(t *testing.T) {
	suite.Run(t, new(rpcSuite))
}

func (s *rpcSuite) SetupTest() {
	db, dbCleanup, err := queue.CreateTestDB()
	s.Require().NoError(err)
	s.db = db
	s.dbCleanup = dbCleanup

	s.tower = CreateTestTowerRPC(s.T(), db)

	s.Require().NoError(s.tower.deleteQueues())
	s.Require().NoError(s.tower.declareQueues())
}

func (s *rpcSuite) TearDownTest() {
	s.NoError(s.tower.deleteQueues())
	s.tower.publisher.StopPublishing()
	s.tower.consumer.StopConsuming("", true)
	s.dbCleanup()
}

func (s *rpcSuite) Test_generateULID() {
	seen := map[string]bool{}
	for i := 0; i < 100000; i++ {
		v := s.tower.generateULID()
		s.Require().NotEmpty(v)
		seen[v] = true
	}
	s.Equal(100000, len(seen))
}

func (s *rpcSuite) TestWorkRequests() {
	var activeTaskCounter int
	workers, capacity := 10, 3
	workersSeen := map[string]bool{}
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Fire up workers, send out work requests
	for i := 0; i < workers; i++ {
		w, err := newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
		s.Require().NoError(err)
		w.id = fmt.Sprintf("testworker-%v", i)

		taskChan, err := w.startWorking(capacity) // this is sending out work requests
		s.Require().NoError(err)

		go func() {
			// Simulate work
			for wt := range taskChan {
				for i := 0; i <= 10; i++ {
					wt.progress <- taskProgress{Percent: float32(i * 10)}
					time.Sleep(50 * time.Millisecond)
				}
				wt.result <- taskResult{remoteStream: &storage.RemoteStream{URL: randomdata.Alphanumeric(25), Manifest: &storage.Manifest{URL: randomdata.Alphanumeric(25)}}}
			}
		}()
	}
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
				timeout := 3 * time.Second
				ctx, cancel := context.WithCancel(context.Background())
				t := time.AfterFunc(timeout, cancel)
				var total float32
				at.SendPayload(&MsgTranscodingTask{SDHash: randomdata.Alphanumeric(8), URL: "lbry://what"})
			ProgressLoop:
				for {
					select {
					case p := <-at.progress:
						total += p.Percent
						t.Reset(timeout)
					case <-ctx.Done():
						s.FailNowf("unexpected timeout waiting for task progress", "%s timed out", at.workerID)
					case <-at.success:
						break ProgressLoop
					}
				}
				cancel()
				s.EqualValues(550, total)
			}(at)
			if activeTaskCounter >= workers*capacity {
				break
			}
		}
	}()
	wg.Wait()
}

func (s *rpcSuite) TestWorkRequestReject() {
	w, err := newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	w.id = "testworker-1"

	taskChan, err := w.startWorking(1) // this is sending out work requests
	s.Require().NoError(err)

	go func() {
		// Simulate work, it won't proceed until the value is read
		<-taskChan
		time.Sleep(5 * time.Second)
	}()

	activeTasks, err := s.tower.startConsumingWorkRequests()
	s.Require().NoError(err)
	at := <-activeTasks
	at.SendPayload(&MsgTranscodingTask{SDHash: randomdata.Alphanumeric(8), URL: "lbry://what"})
	time.Sleep(4 * time.Second)
	w.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	select {
	case e := <-at.errors:
		s.Require().Equal(e.Error, "worker exiting")
	case <-at.progress:
		s.FailNow("unexpected progress received")
	case <-at.success:
		s.FailNow("unexpected done signal received")
	case <-ctx.Done():
		s.FailNowf("timed out waiting for task rejection", at.workerID)
	}
}

func (s *rpcSuite) TestServerGoingAway() {
	tower := CreateTestTowerRPC(s.T(), s.db)
	w, err := newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	w.id = "testworker-1"

	taskChan, err := w.startWorking(3) // this is sending out work requests
	s.Require().NoError(err)

	payload := &MsgTranscodingTask{SDHash: randomdata.Alphanumeric(96), URL: "lbry://what"}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		// Simulate work, it won't proceed until the value is read
		task := <-taskChan
		wg.Done()
		time.Sleep(5 * time.Second)
		task.progress <- taskProgress{Stage: StageDownloading, Percent: 10}
		task.result <- taskResult{remoteStream: &storage.RemoteStream{URL: payload.SDHash}}
		// select {
		// case e := <-at.errors:
		// 	s.FailNow("worker unexpectedly errored", e.Error)
		// case p = <-at.progress:
		// case d = <-at.done:
		// case <-ctx.Done():
		// 	s.FailNowf("timed out waiting for task progress", at.workerID)
		// }
	}()

	activeTasks, err := tower.startConsumingWorkRequests()
	s.Require().NoError(err)
	at := <-activeTasks

	at.SendPayload(payload)
	wg.Wait()
	tower.Stop()

	tl, err := newTaskList(queue.New(s.db))
	s.Require().NoError(err)
	tower, err = newTowerRPC("amqp://guest:guest@localhost/", tl, zapadapter.NewKV(nil))
	s.Require().NoError(err)
	activeTasks, err = tower.startConsumingWorkRequests()
	s.Require().NoError(err)

	at = <-activeTasks
	s.Equal(w.id, at.workerID)
	s.Equal(payload, at.exPayload)
	s.True(at.restored)

	var (
		p MsgWorkerProgress
		d MsgWorkerSuccess
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	select {
	case e := <-at.errors:
		s.FailNow("worker unexpectedly errored", e.Error)
	case p = <-at.progress:
	case d = <-at.success:
	case <-ctx.Done():
		s.FailNowf("timed out waiting for task progress", at.workerID)
	}
	s.NotNil(p, "no task progress received")
	s.NotNil(d, "no task completion received")
}

func (s *rpcSuite) TestWorkerGoingAway() {
	w, err := newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	w.id = "testworker-1"

	taskChan, err := w.startWorking(3) // this is sending out work requests
	s.Require().NoError(err)

	payload := &MsgTranscodingTask{SDHash: randomdata.Alphanumeric(96), URL: "lbry://what"}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		<-taskChan
		wg.Done()
		time.Sleep(5 * time.Second)
	}()

	activeTasks, err := s.tower.startConsumingWorkRequests()
	s.Require().NoError(err)
	at := <-activeTasks
	at.SendPayload(payload)

	wg.Wait()
	w.Stop()

	w, err = newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	w.id = "testworker-1"

	taskChan, err = w.startWorking(3) // this is sending out work requests
	s.Require().NoError(err)

	task := <-taskChan

	s.Equal(*at.exPayload, task.payload)
}

func (s *rpcSuite) TestRetry() {
	w, err := newWorkerRPC("amqp://guest:guest@localhost/", zapadapter.NewKV(nil))
	s.Require().NoError(err)
	w.id = "testworker-1"

	taskChan, err := w.startWorking(1)
	s.Require().NoError(err)
	payload := &MsgTranscodingTask{SDHash: randomdata.Alphanumeric(96), URL: "lbry://what"}

	go func() {
		task := <-taskChan
		task.progress <- taskProgress{Stage: StageDownloading, Percent: 10}
		task.errors <- taskError{err: errors.New("a minor error")}

		for taskRetry := range taskChan {
			time.Sleep(2 * time.Second)
			if taskRetry.payload.TaskID == task.payload.TaskID {
				taskRetry.progress <- taskProgress{Stage: StageDownloading, Percent: 20}
				taskRetry.errors <- taskError{err: errors.New("cannot proceed at all"), fatal: true}
			} else {
				taskRetry.result <- taskResult{remoteStream: &storage.RemoteStream{URL: taskRetry.payload.SDHash}}
			}
		}
	}()

	activeTaskChan, err := s.tower.startConsumingWorkRequests()
	s.Require().NoError(err)

	var retriedTask *activeTask
	stopChan := make(chan struct{})
	errors := make(chan MsgWorkerError)
	progress := make(chan MsgWorkerProgress)
	manageTask := func(at *activeTask) {
		for {
			select {
			case progress <- <-at.progress:
			case errors <- <-at.errors:
				return
			case <-at.success:
				// s.FailNow("unexpected done signal received")
				return
			case <-stopChan:
				return
			}
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		task := <-activeTaskChan
		task.SendPayload(payload)
		go manageTask(task)
		for {
			select {
			case task := <-activeTaskChan:
				if task.exPayload != nil {
					retriedTask = task
					wg.Done()
					task.SendPayload(task.exPayload)
					go manageTask(task)
				} else {
					task.SendPayload(&MsgTranscodingTask{SDHash: randomdata.Alphanumeric(96), URL: "lbry://what"})
					go manageTask(task)
				}
			case <-stopChan:
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case e := <-errors:
		s.Equal("a minor error", e.Error)
		s.False(e.Fatal)
	case p := <-progress:
		s.EqualValues(10, p.Percent)
	case <-ctx.Done():
		s.FailNow("timed out waiting for task progress")
	}

	wg.Wait()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
taskWatch:
	for {
		select {
		case e := <-retriedTask.errors:
			s.Equal("cannot proceed at all", e.Error)
			s.True(e.Fatal)
			break taskWatch
		case p := <-retriedTask.progress:
			s.EqualValues(20, p.Percent)
		case <-ctx.Done():
			s.FailNow("timed out waiting for retried task progress")
		}
	}

	time.Sleep(1 * time.Second)
	t, err := s.tower.tasks.q.GetTask(context.Background(), retriedTask.id)
	s.Require().NoError(err)
	s.Equal("cannot proceed at all", t.Error.String)
	s.True(t.Fatal.Bool)
	s.EqualValues(20, t.StageProgress.Int32)
}
