package tower

import (
	"testing"
	"time"

	"github.com/wagslane/go-rabbitmq"
)

func TestWorker_Start(t *testing.T) {
	type fields struct {
		outbox            rabbitmq.Publisher
		inbox             rabbitmq.Consumer
		accepted          uint64
		id                string
		poolSize          int
		pipeline          *pipeline
		stopChan          chan interface{}
		heartbeatInterval time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// c := &Worker{
			// 	id:                tt.fields.id,
			// 	poolSize:          tt.fields.poolSize,
			// 	pipeline:          tt.fields.pipeline,
			// 	stopChan:          tt.fields.stopChan,
			// 	heartbeatInterval: tt.fields.heartbeatInterval,
			// }
			// if err := c.Start(); (err != nil) != tt.wantErr {
			// 	t.Errorf("Worker.Start() error = %v, wantErr %v", err, tt.wantErr)
			// }
		})
	}
}
