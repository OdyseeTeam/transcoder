package tower

type Payload struct {
	URL string `json:"url"`
}

type MsgRequest struct {
	Ref         string `json:"ref"`
	URL         string `json:"url"`
	SDHash      string `json:"sd_hash"`
	CallbackURL string `json:"callback_url"`
	Key         string `json:"key"`
}

type mPipelineError struct {
	Error string `json:"error,omitempty"`
}

type workerMessage struct {
	RequestStage int
}

type MsgStatus struct {
	ID        string `json:"id"`
	Capacity  int    `json:"capacity"`
	Available int    `json:"available"`
}
