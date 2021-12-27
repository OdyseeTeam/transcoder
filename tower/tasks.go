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
	done      chan MsgWorkerResult
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

func (t *taskList) restore(taskChan chan<- *activeTask) error {
	dbt, err := t.q.GetActiveTasks(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	go func() {
		for _, dt := range dbt {
			at := t.newActiveTask(dt.Worker, dt.Uuid, &MsgTranscodingTask{SDHash: dt.SDHash, URL: dt.URL})
			at.restored = true
			taskChan <- at
		}
	}()
	return nil
}

func (t *taskList) loadRetriable(taskChan chan<- *activeTask) error {
	dbTasks, err := t.q.GetRetriableTasks(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, dt := range dbTasks {
		at := t.newActiveTask(dt.Worker, dt.Uuid, &MsgTranscodingTask{SDHash: dt.SDHash, URL: dt.URL})
		at.retries = dt.Retries.Int32 + 1
		taskChan <- at
	}
	for _, dt := range dbTasks {
		t.q.MarkRetrying(context.Background(), dt.Uuid)
	}
	return nil
}

func (t *taskList) newActiveTask(wid, uuid string, exPayload *MsgTranscodingTask) *activeTask {
	if uuid == "" {
		uuid, _ = generateUUID()
	}
	at := &activeTask{
		workerID:  wid,
		id:        uuid,
		exPayload: exPayload,
		payload:   make(chan MsgTranscodingTask),
		progress:  make(chan MsgWorkerProgress),
		errors:    make(chan MsgWorkerError),
		done:      make(chan MsgWorkerResult),
		tl:        t,
	}
	if exPayload != nil {
		exPayload.TaskID = at.id
	}
	t.insert(at)
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

func (t *taskList) commit(id string, tt MsgTranscodingTask) {
	t.RLock()
	at := t.active[id]
	t.RUnlock()
	t.q.CreateTask(context.Background(), queue.CreateTaskParams{
		Uuid:   at.id,
		Worker: at.workerID,
		URL:    tt.URL,
		SDHash: tt.SDHash,
	})
}

func (t *taskList) getActive(ref string) (*activeTask, bool) {
	t.RLock()
	defer t.RUnlock()
	at, ok := t.active[ref]
	// if !ok {
	// 	dbt, err := t.q.GetTask(context.Background(), ref)
	// 	if err != nil {
	// 		return nil, false
	// 	}
	// 	return t.newActiveTask(dbt.Worker, dbt.Uuid), true
	// }
	return at, ok
}

func (at *activeTask) SendPayload(mtt *MsgTranscodingTask) {
	mtt.TaskID = at.id
	at.payload <- *mtt
}

func (at *activeTask) RecordProgress(m MsgWorkerProgress) (queue.Task, error) {
	at.progress <- m
	t, err := at.tl.q.SetStageProgress(context.Background(), queue.SetStageProgressParams{
		Uuid:          at.id,
		Stage:         sql.NullString{String: string(m.Stage), Valid: true},
		StageProgress: sql.NullInt32{Int32: int32(math.Ceil(float64(m.Percent))), Valid: true},
	})
	return t, err
}

func (at *activeTask) SetError(m MsgWorkerError) (queue.Task, error) {
	at.errors <- m
	t, err := at.tl.q.SetError(context.Background(), queue.SetErrorParams{
		Uuid:  at.id,
		Error: sql.NullString{String: m.Error, Valid: true},
		Fatal: sql.NullBool{Bool: m.Fatal, Valid: true},
	})
	return t, err
}

func (at *activeTask) MarkDone(m MsgWorkerResult) (queue.Task, error) {
	at.done <- m
	return at.tl.q.MarkDone(context.Background(), queue.MarkDoneParams{
		Uuid:   at.id,
		Result: sql.NullString{String: m.RemoteStream.URL, Valid: true},
	})
}
