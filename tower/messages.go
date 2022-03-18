package tower

import (
	"github.com/lbryio/transcoder/library"
)

type Payload struct {
	URL string `json:"url"`
}

type MsgTranscodingTask struct {
	TaskID string `json:"tid"`
	URL    string `json:"url"`
	SDHash string `json:"sd_hash"`
}

type taskProgress struct {
	Stage   RequestStage `json:"stage"`
	Percent float32      `json:"progress"`
}

type taskResult struct {
	remoteStream *library.Stream
}

type taskError struct {
	err   error
	fatal bool
}

type mPipelineError struct {
	Error string `json:"error,omitempty"`
}

type workerMessage struct {
	RequestStage int
}

type workerMsgMeta struct {
	tid, wid, mType string
}

type MsgWorkerHandshake struct {
	WorkerID  string `json:"worker_id"`
	Capacity  int    `json:"capacity"`
	Available int    `json:"available"`
	SessionID string `json:"session_id"`
}

type MsgWorkerRequest struct {
	WorkerID  string `json:"worker_id"`
	SessionID string `json:"session"`
}

type MsgWorkerProgress struct {
	Stage   RequestStage `json:"stage"`
	Percent float32      `json:"progress"`
}

type MsgWorkerError struct {
	Error string `json:"error"`
	Fatal bool   `json:"fatal"`
}

type MsgWorkerSuccess struct {
	RemoteStream *library.Stream `json:"remote_stream"`
}
