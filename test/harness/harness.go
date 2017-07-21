package harness

import (
	"net/http/httptest"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/views"
	"github.com/kevinburke/nacl"
	twilio "github.com/kevinburke/twilio-go"
)

var NullLogger = log.New()

func init() {
	NullLogger.SetHandler(log.DiscardHandler())
}

type ViewHarness struct {
	TestServer     *httptest.Server
	TwilioClient   *twilio.Client
	SecretKey      *[32]byte
	MaxResourceAge time.Duration
}

func ViewsClient(harness ViewHarness) views.Client {
	var c *twilio.Client
	if harness.TwilioClient == nil {
		c = twilio.NewClient("AC123", "123", nil)
	} else {
		c = harness.TwilioClient
	}
	if harness.TestServer != nil {
		c.Base = harness.TestServer.URL
	}
	if harness.SecretKey == nil {
		harness.SecretKey = nacl.NewKey()
	}
	if harness.MaxResourceAge == 0 {
		harness.MaxResourceAge = 720 * time.Hour
	}
	return views.NewClient(NullLogger, c, harness.SecretKey, config.NewPermission(harness.MaxResourceAge))
}
