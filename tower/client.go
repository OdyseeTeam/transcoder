package tower

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/streadway/amqp"
	"github.com/wagslane/go-rabbitmq"
)

type Client struct {
	// Different connections for publishing and consuming
	outbox   rabbitmq.Publisher
	inbox    rabbitmq.Consumer
	accepted uint64
	id       string
	poolSize int
}

// NewClient creates a new client connecting to AMQP server specified as `addr`.
func NewClient(addr string, poolSize int) (*Client, error) {
	client := Client{poolSize: poolSize}
	outbox, returns, err := rabbitmq.NewPublisher(addr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	client.outbox = outbox

	inbox, err := rabbitmq.NewConsumer(addr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	client.inbox = inbox

	go func() {
		for r := range returns {
			fmt.Printf("client got message from server: %+v\n", r)
		}
	}()

	return &client, nil
}

func (c *Client) SetID(id string) {
	if c.id != "" {
		return
	}
	c.id = id
}

func (c *Client) generateID() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	c.SetID(fmt.Sprintf("%v-%v-%v", hostname, time.Now().UnixNano(), randomdata.Number(100, 200)))
}

func (c *Client) Start() error {
	if c.id == "" {
		c.generateID()
	}
	return c.inbox.StartConsuming(
		func(d rabbitmq.Delivery) bool {
			var req request
			err := json.Unmarshal(d.Body, &req)
			if err != nil {
				fmt.Println("errrrrr", err)
				return true
			}
			c.accepted++
			c.Respond(req, response{Status: Accepted})
			return true
		},
		"inbox",
		[]string{requestsQueueName},
		rabbitmq.WithConsumeOptionsConcurrency(c.poolSize), // Set to transcoder worker pool size
		rabbitmq.WithConsumeOptionsBindingExchangeDurable,
		rabbitmq.WithConsumeOptionsBindingExchangeName("transcoder"),
		rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"),
		rabbitmq.WithConsumeOptionsConsumerName(c.id),
		rabbitmq.WithConsumeOptionsQueueDurable,
		rabbitmq.WithConsumeOptionsQuorum,
	)
}

func (c *Client) Respond(req request, resp response) error {
	resp.Ref = req.Payload.URL
	resp.ClientId = c.id
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.outbox.Publish(
		body,
		[]string{responsesQueueName},
		// leave blank for defaults
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange("transcoder"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsPersistentDelivery,
	)
}

func (c *Client) Stop() {
	c.inbox.StopConsuming(c.id, false)
	c.inbox.Disconnect()
}
