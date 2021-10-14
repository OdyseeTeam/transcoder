package tower

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
	"github.com/wagslane/go-rabbitmq"
)

const responsesConsumerName = "response-consumer"
const responsesQueueName = "responses"
const requestsQueueName = "requests"

type Server struct {
	// Different connections for publishing and consuming
	outbox   rabbitmq.Publisher
	inbox    rabbitmq.Consumer
	clients  map[string]*WorkerClient
	requests *requests
}

type WorkerClient struct {
	Name       string
	Capacity   int
	Busy       int
	LastActive time.Time
}

type RegistrationRequest struct {
	Name     string
	Capacity int
}

type CapacityNotification struct {
	Name     string
	Capacity int
	Busy     int
}

type flyingRequest struct {
	URI, Name, ClaimID, SDHash, ChannelURI, NormalizedName string

	Status           RequestStatus
	Error            string
	Progress         uint
	assignedClientId string
}

type requests struct {
	sync.RWMutex
	data map[string]*flyingRequest
}

func NewServer(addr string) (*Server, error) {
	server := Server{
		clients:  map[string]*WorkerClient{},
		requests: &requests{data: map[string]*flyingRequest{}},
	}
	outbox, returns, err := rabbitmq.NewPublisher(addr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	server.outbox = outbox

	inbox, err := rabbitmq.NewConsumer(addr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	server.inbox = inbox

	go func() {
		for r := range returns {
			fmt.Printf("message returned from server: %s\n", string(r.Body))
		}
	}()

	return &server, nil
}

func (s *Server) Start() error {
	return s.consumeInbox(
		func(d rabbitmq.Delivery) bool {
			var resp response
			err := json.Unmarshal(d.Body, &resp)
			if err != nil {
				fmt.Println(err)
				return true
			}
			s.requests.Lock()
			defer s.requests.Unlock()
			if reqData, ok := s.requests.data[resp.Ref]; !ok {
				fmt.Println("request not found, ref", resp.Ref)
			} else {
				reqData.Status = resp.Status
				reqData.Progress = resp.Progress
				reqData.Error = resp.Error
				reqData.assignedClientId = resp.ClientId
			}
			return true
		},
	)
}

func (s *Server) consumeInbox(handler func(d rabbitmq.Delivery) bool) error {
	err := s.inbox.StartConsuming(
		handler,
		responsesQueueName,
		[]string{responsesQueueName},
		rabbitmq.WithConsumeOptionsConcurrency(10),
		rabbitmq.WithConsumeOptionsBindingExchangeDurable,
		rabbitmq.WithConsumeOptionsBindingExchangeName("transcoder"),
		rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"),
		rabbitmq.WithConsumeOptionsConsumerName(responsesConsumerName),
		rabbitmq.WithConsumeOptionsQueueDurable,
		rabbitmq.WithConsumeOptionsQuorum,
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) SendRequest(req request) error {
	s.requests.Lock()
	// s.requests.data[req.SDHash] = &flyingRequest{
	// 	URI:            req.URI,
	// 	SDHash:         req.SDHash,
	// 	Name:           req.Name,
	// 	ClaimID:        req.ClaimID,
	// 	ChannelURI:     req.ChannelURI,
	// 	NormalizedName: req.NormalizedName,
	// }
	s.requests.Unlock()

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	fmt.Println("sending request:", string(body))

	return s.outbox.Publish(
		body,
		[]string{requestsQueueName},
		// leave blank for defaults
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange("transcoder"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsPersistentDelivery,
	)
}

func (s *Server) Stop() {
	s.inbox.StopConsuming(responsesConsumerName, false)
	s.inbox.Disconnect()
}

func (s *Server) Available() int {
	var available int
	for _, c := range s.clients {
		available += c.Capacity - c.Busy
	}
	return available
}
