package manager

import (
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OdyseeTeam/transcoder/library"
	"github.com/OdyseeTeam/transcoder/library/db"
	"github.com/OdyseeTeam/transcoder/pkg/logging"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"
	"github.com/OdyseeTeam/transcoder/pkg/mfr"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/suite"
)

type managerSuite struct {
	suite.Suite
	library.LibraryTestHelper
}

func isLevel5(_ string) bool {
	return rand.Intn(2) == 0 // #nosec G404
}

func isChannelEnabled(_ string) bool {
	return rand.Intn(2) == 0 // #nosec G404
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(managerSuite))
}

func (s *managerSuite) SetupSuite() {
	logger = logging.Create("manager", logging.Dev)
	s.Require().NoError(s.SetupLibraryDB())
}

func (s *managerSuite) TearDownSuite() {
	s.Require().NoError(s.TearDownLibraryDB())
}

func (s *managerSuite) TestVideo() {
	var err error
	lib := library.New(library.Config{DB: s.DB, Log: zapadapter.NewKV(nil)})

	_, err = lib.AddChannel("@BretWeinstein#f", db.ChannelPriorityHigh)
	s.Require().NoError(err)
	_, err = lib.AddChannel("@veritasium:f", "")
	s.Require().NoError(err)
	_, err = lib.AddChannel("@specialoperationstest#3", "")
	s.Require().NoError(err)
	_, err = lib.AddChannel("@TheVoiceofReason#a", db.ChannelPriorityDisabled)
	s.Require().NoError(err)

	mgr := NewManager(lib, 0)

	urlsPriority := []string{
		"@BretWeinstein#f/EvoLens87#1",
	}
	urlsEnabled := []string{
		"@veritasium#f/on-these-questions,-smarter-people-do#e",
		"@specialoperationstest#3/fear-of-death-inspirational#a",
	}
	urlsLevel5 := []string{
		"@samtime#1/airpods-max-parody-ehh-pods-max#7",
	}
	urlsNotEnabled := []string{
		"@TRUTH#2/what-do-you-know-what-do-you-believe#2",
	}
	urlsNoChannel := []string{
		"what#1",
	}
	urlsDisabled := []string{
		"lbry://@TheVoiceofReason#a/PaypalSucks#5",
	}
	urlsNotFound := []string{
		randomdata.SillyName() + "#" + randomdata.SillyName(),
		randomdata.Alphanumeric(96),
	}

	for _, u := range urlsPriority {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrTranscodingQueued, err, u)
	}

	for _, u := range urlsEnabled {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrTranscodingQueued, err, u)
	}

	for _, u := range urlsLevel5 {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrTranscodingQueued, err, u)
	}

	for _, u := range urlsNotEnabled {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrTranscodingForbidden, err, u)
	}

	for _, u := range urlsDisabled {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrTranscodingForbidden, err, u)
	}

	for _, u := range urlsNoChannel {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrNoSigningChannel, err, u)
	}

	for _, u := range urlsNotFound {
		v, err := mgr.Video(u)
		s.Empty(v)
		s.Equal(resolve.ErrClaimNotFound, err)
	}

	expectedUrls := []string{urlsPriority[0], urlsEnabled[0], urlsLevel5[0], urlsNotEnabled[0], urlsEnabled[1]}
	receivedUrls := func() []string {
		requestsChan := mgr.Requests()
		timeout := time.After(15 * time.Second)
		defer mgr.pool.Stop()
		urls := []string{}
		for len(urls) <= 5 {
			select {
			case r := <-requestsChan:
				urls = append(urls, strings.TrimPrefix(r.URI, "lbry://"))
			case <-timeout:
				return urls
			}
		}
		return urls
	}()

	sort.Strings(expectedUrls)
	sort.Strings(receivedUrls)
	s.Equal(expectedUrls, receivedUrls)

}

func (s *managerSuite) TestRequests() {
	var r1, r2 *TranscodingRequest

	lib := library.New(library.Config{DB: s.DB, Log: zapadapter.NewKV(nil)})
	mgr := NewManager(lib, 0)

	mgr.Video("@specialoperationstest#3/fear-of-death-inspirational#a")
	out := mgr.Requests()
	r1 = <-out

	s.Equal(mfr.StatusActive, mgr.RequestStatus(r1.SDHash))
	select {
	case r2 = <-out:
		s.Failf("got output from Requests channel", "%v", r2)
	default:
	}

	s.NotNil(r1)
}
