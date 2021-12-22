package tower

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sync"

	"github.com/lbryio/transcoder/tower/queue"
)

type activeTask struct {
	id       string
	workerID string
	payload  chan MsgTranscodingTask
	progress chan MsgWorkerProgress
	errChan  chan error
	done     chan MsgWorkerResult
	tl       *taskList
}

type workerTask struct {
	payload  MsgTranscodingTask
	progress chan taskProgress
	errChan  chan error
	result   chan taskResult
}

type taskList struct {
	sync.RWMutex
	active map[string]*activeTask
	q      *queue.Queries
}

func newTaskList(q *queue.Queries) *taskList {
	return &taskList{q: q, active: map[string]*activeTask{}}
}

func (t *taskList) newActiveTask(wid string) *activeTask {
	uuid, _ := generateUUID()
	at := &activeTask{
		workerID: wid,
		id:       uuid,
		payload:  make(chan MsgTranscodingTask),
		progress: make(chan MsgWorkerProgress),
		errChan:  make(chan error),
		done:     make(chan MsgWorkerResult),
		tl:       t,
	}
	t.preInsert(at)
	return at
}

func (t *taskList) preInsert(at *activeTask) {
	t.Lock()
	t.active[at.id] = at
	t.Unlock()
}

func (t *taskList) preDelete(id string) {
	t.Lock()
	delete(t.active, id)
	t.Unlock()
}

func (t *taskList) persist(id string, tt MsgTranscodingTask) {
	t.RLock()
	at := t.active[id]
	t.RUnlock()
	t.q.CreateTask(context.Background(), queue.CreateTaskParams{
		Ref:    at.id,
		Worker: at.workerID,
		URL:    tt.URL,
		SDHash: tt.SDHash,
	})
}

func (t *taskList) getActive(id string) (*activeTask, bool) {
	t.RLock()
	defer t.RUnlock()
	at, ok := t.active[id]
	return at, ok
}

func (at *activeTask) RecordProgress(m MsgWorkerProgress) (queue.Task, error) {
	at.progress <- m
	return at.tl.q.SetStageProgress(context.Background(), queue.SetStageProgressParams{
		Ref:           at.id,
		Stage:         sql.NullString{String: string(m.Stage)},
		StageProgress: sql.NullInt32{Int32: int32(math.Ceil(float64(m.Percent)))},
	})
}

func (at *activeTask) SetError(e string) (queue.Task, error) {
	at.errChan <- errors.New(e)
	return at.tl.q.SetError(context.Background(), queue.SetErrorParams{
		Ref:   at.id,
		Error: sql.NullString{String: e},
	})
}

func (at *activeTask) MarkDone(m MsgWorkerResult) (queue.Task, error) {
	at.done <- m
	return at.tl.q.MarkDone(context.Background(), queue.MarkDoneParams{
		Ref:    at.id,
		Result: sql.NullString{String: m.RemoteStream.URL},
	})
}
