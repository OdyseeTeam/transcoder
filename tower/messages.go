package tower

import (
	"time"

	"github.com/lbryio/transcoder/storage"
)

type Payload struct {
	URL string `json:"url"`
}

type MsgTranscodingTask struct {
	TaskID string `json:"task_id"`
	Ref    string `json:"ref"`
	URL    string `json:"url"`
	SDHash string `json:"sd_hash"`
}

type taskProgress struct {
	Stage   RequestStage `json:"stage"`
	Percent float32      `json:"progress"`
}

type taskResult struct {
	remoteStream *storage.RemoteStream
}

type mPipelineError struct {
	Error string `json:"error,omitempty"`
}

type workerMessage struct {
	RequestStage int
}

type MsgWorkerStatus struct {
	WorkerID  string    `json:"worker_id"`
	Capacity  int       `json:"capacity"`
	Available int       `json:"available"`
	Timestamp time.Time `json:"timestamp"`
}

type MsgWorkerProgress struct {
	WorkerID  string       `json:"worker_id"`
	TaskID    string       `json:"task_id"`
	Stage     RequestStage `json:"stage"`
	Percent   float32      `json:"progress"`
	Timestamp time.Time    `json:"timestamp"`
}

type MsgWorkerError struct {
	WorkerID  string    `json:"worker_id"`
	TaskID    string    `json:"task_id"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

type MsgWorkerResult struct {
	WorkerID     string                `json:"worker_id"`
	TaskID       string                `json:"task_id"`
	Timestamp    time.Time             `json:"timestamp"`
	RemoteStream *storage.RemoteStream `json:"remote_stream"`
}

type MsgWorkRequest struct {
	WorkerID string `json:"worker_id"`
}
