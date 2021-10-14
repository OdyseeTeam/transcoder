package tower

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/suite"
	"github.com/wagslane/go-rabbitmq"
)

type towerSuite struct {
	suite.Suite
}

func TestTowerSuite(t *testing.T) {
	suite.Run(t, new(towerSuite))
}

func (s *towerSuite) SetupTest() {
	srv, err := NewServer("amqp://guest:guest@localhost/")
	s.Require().NoError(err)

	err = srv.consumeInbox(func(_ rabbitmq.Delivery) bool { fmt.Println("discarding"); return true })
	s.Require().NoError(err)
	srv.Stop()
}

func (s *towerSuite) TestRPC() {
	messagesToSend := 1000
	clientsNum := 300

	srv, err := NewServer("amqp://guest:guest@localhost/")
	s.Require().NoError(err)

	err = srv.Start()
	s.Require().NoError(err)

	clientsSeen := map[string]bool{}
	tasksSent := map[string]bool{}
	clients := []*Client{}
	for i := 0; i < clientsNum; i++ {
		c, err := NewClient("amqp://guest:guest@localhost/", 10)
		s.Require().NoError(err)

		err = c.Start()
		s.Require().NoError(err)
		s.Require().Greater(len(c.id), 5)
		clients = append(clients, c)
	}

	for i := 0; i < messagesToSend; i++ {
		r := request{Method: "", Payload: Payload{URL: "lbry://" + randomdata.SillyName(), CallbackURL: randomdata.Alphanumeric(64)}}
		tasksSent[r.Payload.URL] = true
		err := srv.SendRequest(r)
		s.Require().NoError(err)
	}

	accepted := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for len(accepted) < messagesToSend {
		select {
		case <-ctx.Done():
			s.FailNowf(
				"wait time exceeded",
				"only got %v acknowledgments from clients, expected",
				len(accepted), messagesToSend)
		default:
			accepted = map[string]bool{}
			for ref, r := range srv.requests.data {
				if r.Status == Accepted {
					accepted[ref] = true
					clientsSeen[r.assignedClientId] = true
				}
			}
		}
	}

	// Assure the work is distributed evenly
	s.GreaterOrEqual(len(clientsSeen), clientsNum)

	for _, c := range clients {
		c.Stop()
	}
}
