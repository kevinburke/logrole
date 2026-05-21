package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/nacl"
	twilio "github.com/kevinburke/twilio-go"
)

// makeBrowserCallsTestHandler returns a handler wired up just like the
// production server would wire it for browser calling. It bypasses the
// full server.NewServer plumbing so the tests stay focused on the
// adapter logic.
func makeBrowserCallsTestHandler(t *testing.T) http.Handler {
	t.Helper()
	settings := &config.Settings{
		Logger:                    dlog,
		SecretKey:                 nacl.NewKey(),
		Client:                    twilio.NewClient("AC11111111111111111111111111111111", "test-auth-token", nil),
		TwilioAPIKey:              "SK22222222222222222222222222222222",
		TwilioAPISecret:           "test-secret",
		TwilioTwiMLAppSid:         "AP33333333333333333333333333333333",
		DefaultSendingPhoneNumber: "+14155550199",
		PublicHost:                "example.com",
	}
	h, err := newBrowserCallHandler(dlog, settings)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("expected handler, got nil")
	}
	return h
}

func TestBrowserCallsRouteGatedByPermission(t *testing.T) {
	t.Parallel()
	h := makeBrowserCallsTestHandler(t)
	req := httptest.NewRequest("GET", "/dial", nil)
	req = config.SetUser(req, theUser) // theUser lacks CanMakeCalls
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for user lacking CanMakeCalls, got %d", w.Code)
	}
}

func TestBrowserCallsDialerRenderedThroughChrome(t *testing.T) {
	t.Parallel()
	h := makeBrowserCallsTestHandler(t)
	req := httptest.NewRequest("GET", "/dial", nil)
	req = config.SetUser(req, config.NewUser(&config.UserSettings{CanMakeCalls: true}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// chromeData wraps the browsercall fragment in base.html - confirm
	// both pieces ended up in the response.
	if !strings.Contains(body, "Place a Call") {
		t.Errorf("expected chrome title in HTML, got %s", body)
	}
	if !strings.Contains(body, `src="/static/js/twilio-voice-sdk.js"`) {
		t.Errorf("expected bundled SDK script tag, got %s", body)
	}
	if !strings.Contains(body, "navbar") {
		t.Errorf("expected logrole navbar (from base.html), got %s", body)
	}
}
