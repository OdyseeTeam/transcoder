package tower

import "time"

type RequestStage string
type WorkerMessageType string

const (
	responsesConsumerName = "response-consumer"
	responsesQueueName    = "responses"
	requestsQueueName     = "requests"
	workerHandshakeQueue  = "worker-handshake"
	workRequestsQueue     = "work-requests"
	taskStatusQueue       = "task-status"
	backupSuccessQueue    = "backup-success"

	workersExchange = "workers"

	headerTaskID      = "task-id"
	headerWorkerID    = "worker-id"
	headerMessageType = "message-type"

	mTypeProgress = "progress"
	mTypeSuccess  = "success"
	mTypeError    = "error"

	defaultHeartbeatInterval = 30 * time.Second
	maxFailedAttempts        = 5
)

const (
	StagePending RequestStage = "pending"

	StageAccepted    RequestStage = "accepted"
	StageDownloading RequestStage = "downloading"
	StageEncoding    RequestStage = "encoding"
	StageUploading   RequestStage = "uploading"

	StageFailedRequeued   RequestStage = "failed_requeued"
	StageTimedOutRequeued RequestStage = "timed_out_requeued"

	StageFailed RequestStage = "failed"

	StageDone          RequestStage = "done"
	StageFailedFatally RequestStage = "failed_fatally" // This is a fatal error stage and stream cannot be re-added after this
	StageCompleted     RequestStage = "completed"      // All processing has been successfully completed and stream is in the database
)

const (
	tHeartbeat      WorkerMessageType = "heartbeat"
	tPipelineUpdate WorkerMessageType = "p_update"
	tPipelineError  WorkerMessageType = "p_error"
)
