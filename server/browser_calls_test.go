package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/test"
	"github.com/kevinburke/logrole/test/harness"
	"github.com/kevinburke/logrole/views"
	"github.com/kevinburke/nacl"
	twilio "github.com/kevinburke/twilio-go/v2"
)

// browserCallerID is the caller ID the test dialer places calls from, and
// the number the recent-calls list is filtered on.
const browserCallerID = "+19253920364"

// makeBrowserCallsTestHandler returns a handler wired up just like the
// production server would wire it for browser calling. It bypasses the
// full server.NewServer plumbing so the tests stay focused on the
// adapter logic.
func makeBrowserCallsTestHandler(t *testing.T, vc views.Client) http.Handler {
	t.Helper()
	settings := &config.Settings{
		Logger:                    dlog,
		SecretKey:                 nacl.NewKey(),
		Client:                    twilio.NewClient("AC11111111111111111111111111111111", "test-auth-token", nil),
		TwilioAPIKey:              "SK22222222222222222222222222222222",
		TwilioAPISecret:           "test-secret",
		TwilioTwiMLAppSid:         "AP33333333333333333333333333333333",
		DefaultSendingPhoneNumber: browserCallerID,
		PublicHost:                "example.com",
	}
	h, err := newBrowserCallHandler(dlog, vc, lf, settings)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("expected handler, got nil")
	}
	return h
}

// recentCallsTestServer serves the canned call list response and records the URL
// the views client asked for.
func recentCallsTestServer(t *testing.T, urls chan<- string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case urls <- r.URL.String():
		default:
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(200)
		if _, err := w.Write(test.CallListBody); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestBrowserCallsRouteGatedByPermission(t *testing.T) {
	t.Parallel()
	vc := harness.ViewsClient(harness.ViewHarness{SecretKey: key})
	h := makeBrowserCallsTestHandler(t, vc)
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
	urls := make(chan string, 1)
	s := recentCallsTestServer(t, urls)
	vc := harness.ViewsClient(harness.ViewHarness{
		SecretKey:      key,
		TestServer:     s,
		MaxResourceAge: config.DefaultMaxResourceAge,
	})
	h := makeBrowserCallsTestHandler(t, vc)
	req := httptest.NewRequest("GET", "/dial", nil)
	req = config.SetUser(req, config.NewUser(config.AllUserSettings()))
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

func TestBrowserCallsDialerListsRecentCalls(t *testing.T) {
	t.Parallel()
	urls := make(chan string, 1)
	s := recentCallsTestServer(t, urls)
	vc := harness.ViewsClient(harness.ViewHarness{
		SecretKey:      key,
		TestServer:     s,
		MaxResourceAge: config.DefaultMaxResourceAge,
	})
	h := makeBrowserCallsTestHandler(t, vc)
	req := httptest.NewRequest("GET", "/dial", nil)
	req = config.SetUser(req, config.NewUser(config.AllUserSettings()))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	select {
	case u := <-urls:
		if !strings.Contains(u, "From=%2B19253920364") {
			t.Errorf("expected call list filtered on the caller ID, got %s", u)
		}
		if !strings.Contains(u, "PageSize=10") {
			t.Errorf("expected call list to request %s results, got %s", recentCallCount, u)
		}
	default:
		t.Fatal("expected the dialer to request the recent call list")
	}
	body := w.Body.String()
	if !strings.Contains(body, "Recent calls") {
		t.Errorf("expected a recent calls heading, got %s", body)
	}
	// The dialed number, the duration, and the time the call was placed.
	if !strings.Contains(body, "610-331-7238") {
		t.Errorf("expected the dialed number in the recent call list, got %s", body)
	}
	if !strings.Contains(body, "17s") {
		t.Errorf("expected the call duration in the recent call list, got %s", body)
	}
	if !strings.Contains(body, "2016") {
		t.Errorf("expected the date the call was placed, got %s", body)
	}
}

// A user who can place calls but cannot view them gets the dialer without a
// call list, rather than an error.
func TestBrowserCallsDialerHidesCallsWithoutPermission(t *testing.T) {
	t.Parallel()
	urls := make(chan string, 1)
	s := recentCallsTestServer(t, urls)
	vc := harness.ViewsClient(harness.ViewHarness{
		SecretKey:      key,
		TestServer:     s,
		MaxResourceAge: config.DefaultMaxResourceAge,
	})
	h := makeBrowserCallsTestHandler(t, vc)
	req := httptest.NewRequest("GET", "/dial", nil)
	req = config.SetUser(req, config.NewUser(&config.UserSettings{CanMakeCalls: true}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	select {
	case u := <-urls:
		t.Errorf("expected no call list request for a user who cannot view calls, got %s", u)
	default:
	}
	body := w.Body.String()
	if strings.Contains(body, "610-331-7238") {
		t.Errorf("expected no call list for a user who cannot view calls, got %s", body)
	}
	if strings.Contains(body, "Could not load recent calls") {
		t.Errorf("expected no error message for a user who cannot view calls, got %s", body)
	}
}
