package tower

import "time"

type RequestStage string
type WorkerMessageType string

const (
	responsesConsumerName = "response-consumer"
	responsesQueueName    = "responses"
	requestsQueueName     = "requests"
	workerStatusQueueName = "worker-status"
	workRequestsQueueName = "work-requests"
	taskProgressQueueName = "task-progress"
	taskErrorsQueueName   = "task-errors"
	taskDoneQueueName     = "task-done"

	replyToQueueName = "amq.rabbitmq.reply-to"

	workersExchange = "workers"

	headerTaskID   = "task-id"
	headerWorkerID = "worker-id"

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
