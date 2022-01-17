package tower

import (
	"context"
	"database/sql"
	"math"
	"sync"

	"github.com/lbryio/transcoder/tower/queue"
)

type activeTask struct {
	id        string
	workerID  string
	restored  bool
	retries   int32
	exPayload *MsgTranscodingTask
	payload   chan MsgTranscodingTask
	progress  chan MsgWorkerProgress
	errors    chan MsgWorkerError
	success   chan MsgWorkerSuccess
	tl        *taskList
}

type workerTask struct {
	payload  MsgTranscodingTask
	progress chan taskProgress
	errors   chan taskError
	result   chan taskResult
}

type taskList struct {
	sync.RWMutex
	active    map[string]*activeTask
	q         *queue.Queries
	retryChan chan *activeTask
}

func newTaskList(q *queue.Queries) (*taskList, error) {
	tl := &taskList{
		q:         q,
		active:    map[string]*activeTask{},
		retryChan: make(chan *activeTask),
	}
	return tl, nil
}

func (t *taskList) restore() (<-chan *activeTask, error) {
	restoreChan := make(chan *activeTask)
	dbt, err := t.q.GetActiveTasks(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	restored := []*activeTask{}
	for _, dt := range dbt {
		at := t.newActiveTask(dt.Worker, dt.ULID, &MsgTranscodingTask{SDHash: dt.SDHash, URL: dt.URL})
		at.restored = true
		at.tl.insert(at)
		restored = append(restored, at)
	}
	go func() {
		for _, at := range restored {
			restoreChan <- at
		}
		close(restoreChan)
	}()
	return restoreChan, nil
}

func (t *taskList) loadRetriable(taskChan chan<- *activeTask) error {
	dbTasks, err := t.q.GetRetriableTasks(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, dt := range dbTasks {
		at := t.newActiveTask(dt.Worker, dt.ULID, &MsgTranscodingTask{SDHash: dt.SDHash, URL: dt.URL})
		at.retries = dt.Retries.Int32 + 1
		at.tl.insert(at)
		taskChan <- at
	}
	for _, dt := range dbTasks {
		t.q.MarkRetrying(context.Background(), dt.ULID)
	}
	return nil
}

func (t *taskList) newEmptyTask(wid, ulid string) *activeTask {
	at := &activeTask{
		workerID: wid,
		id:       ulid,
		payload:  make(chan MsgTranscodingTask),
		progress: make(chan MsgWorkerProgress),
		errors:   make(chan MsgWorkerError),
		success:  make(chan MsgWorkerSuccess),
		tl:       t,
	}
	return at
}

func (t *taskList) newActiveTask(wid, ulid string, exPayload *MsgTranscodingTask) *activeTask {
	at := &activeTask{
		workerID:  wid,
		id:        ulid,
		exPayload: exPayload,
		payload:   make(chan MsgTranscodingTask),
		progress:  make(chan MsgWorkerProgress),
		errors:    make(chan MsgWorkerError),
		success:   make(chan MsgWorkerSuccess),
		tl:        t,
	}
	if exPayload != nil {
		exPayload.TaskID = at.id
	}
	return at
}

func (t *taskList) insert(at *activeTask) {
	t.Lock()
	t.active[at.id] = at
	t.Unlock()
}

func (t *taskList) delete(id string) {
	t.Lock()
	delete(t.active, id)
	t.Unlock()
}

func (t *taskList) get(ref string) (*activeTask, bool) {
	t.RLock()
	defer t.RUnlock()
	at, ok := t.active[ref]
	return at, ok
}

func (at *activeTask) SendPayload(mtt *MsgTranscodingTask) {
	mtt.TaskID = at.id
	at.payload <- *mtt
}

func (at *activeTask) RecordProgress(m MsgWorkerProgress) (queue.Task, error) {
	t, err := at.tl.q.SetStageProgress(context.Background(), queue.SetStageProgressParams{
		ULID:          at.id,
		Stage:         sql.NullString{String: string(m.Stage), Valid: true},
		StageProgress: sql.NullInt32{Int32: int32(math.Ceil(float64(m.Percent))), Valid: true},
	})
	select {
	case at.progress <- m:
	default:
	}
	return t, err
}

func (at *activeTask) SetError(m MsgWorkerError) (queue.Task, error) {
	var t queue.Task
	var err error
	if m.Fatal {
		t, err = at.tl.q.MarkFailed(context.Background(), queue.MarkFailedParams{
			ULID:  at.id,
			Error: sql.NullString{String: m.Error, Valid: true},
		})
	} else {
		t, err = at.tl.q.SetError(context.Background(), queue.SetErrorParams{
			ULID:  at.id,
			Error: sql.NullString{String: m.Error, Valid: true},
		})
	}
	if err != nil {
		return t, err
	}
	at.errors <- m
	// select {
	// case at.errors <- m:
	// default:
	// }
	return t, err
}

func (at *activeTask) MarkDone(m MsgWorkerSuccess) (queue.Task, error) {
	t, err := at.tl.q.MarkDone(context.Background(), queue.MarkDoneParams{
		ULID:   at.id,
		Result: sql.NullString{String: m.RemoteStream.URL, Valid: true},
	})
	select {
	case at.success <- m:
	default:
	}
	return t, err
}
