package tower

type Payload struct {
	URL         string `json:"url"`
	CallbackURL string `json:"callback_url"`
}

type request struct {
	Method  string  `json:"method,omitempty"`
	Payload Payload `json:"payload,omitempty"`
}

type response struct {
	ClientId string
	Ref      string
	Status   RequestStatus
	Error    string
	Progress uint
}

type RequestStatus int

const (
	Pending RequestStatus = iota
	Accepted
	Downloading
	Transcoding
	Uploading
	Done
	Failed
)
