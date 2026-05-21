package browsercall_test

import (
	"html/template"
	"log"
	"net/http"

	"github.com/kevinburke/logrole/browsercall"
)

// ExampleNew shows the simplest integration: mount Handler at /phone in
// your existing app's mux. GET /phone is the dialer page; the SDK
// loads /static/twilio-voice-sdk.js (which you bundle separately) and
// fetches tokens from /phone/token. Twilio's TwiML App must be
// configured with Voice URL https://<your-host>/phone/voice.
func ExampleNew() {
	h, err := browsercall.New(browsercall.Config{
		AccountSid:  "AC11111111111111111111111111111111",
		APIKey:      "SK22222222222222222222222222222222",
		APISecret:   "your-api-key-secret",
		TwiMLAppSid: "AP33333333333333333333333333333333",
		AuthToken:   "your-account-auth-token",
		CallerID:    "+14155550199", // a verified Twilio number on this account
		ScriptURL:   "/static/twilio-voice-sdk.js",
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/phone/", http.StripPrefix("/phone", h))
	// mux.Handle("/phone", http.StripPrefix("/phone", h)) // optional: catch the bare prefix too
	_ = mux
}

// ExampleHandler_Fragment shows embedding the dialer in a larger page
// rendered by the host app. The Handler is still mounted somewhere so
// /token and /voice work; Fragment renders just the dialer body that
// gets spliced into the host page.
//
// In this layout:
//
//   - GET  /dashboard   - host app's page, with the dialer embedded
//   - GET  /phone/token - mints access tokens (served by Handler)
//   - POST /phone/voice - TwiML webhook for Twilio (served by Handler)
//
// Twilio still needs to be configured to POST /phone/voice; the
// dashboard page just borrows the dialer markup.
func ExampleHandler_Fragment() {
	h, err := browsercall.New(browsercall.Config{
		AccountSid:  "AC11111111111111111111111111111111",
		APIKey:      "SK22222222222222222222222222222222",
		APISecret:   "your-api-key-secret",
		TwiMLAppSid: "AP33333333333333333333333333333333",
		AuthToken:   "your-account-auth-token",
		CallerID:    "+14155550199",
		ScriptURL:   "/static/twilio-voice-sdk.js",
		Authorize:   onlyLoggedInUsers,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Render the dialer body once at startup. tokenPath points at the
	// mounted Handler's /token route. Fragment doesn't change between
	// requests, so we can render it eagerly.
	dialerHTML, err := h.Fragment("/phone/token")
	if err != nil {
		log.Fatal(err)
	}

	dashboardTpl := template.Must(template.New("dashboard").Parse(`
		<!doctype html>
		<title>Dashboard</title>
		<nav>...your navigation...</nav>
		<main>
		  <h1>Welcome back</h1>
		  <section class="dialer">{{ .Dialer }}</section>
		</main>
	`))

	mux := http.NewServeMux()
	// Host-app dashboard that splices the dialer into its own layout.
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dashboardTpl.Execute(w, map[string]any{"Dialer": dialerHTML})
	})
	// Handler still serves /token (auth-gated by the Authorize func
	// above) and /voice (auth-gated by X-Twilio-Signature).
	mux.Handle("/phone/", http.StripPrefix("/phone", h))
	_ = mux
}

// ExampleConfig_DialerLayout shows wrapping the bundled dialer page
// in custom chrome at its own URL, without using Fragment.
// DialerLayout is the option you want when the dialer should stay at a
// dedicated route but inherit your app's header, navigation, etc.
func ExampleConfig_DialerLayout() {
	hostTpl := template.Must(template.New("host").Parse(`
		<!doctype html>
		<title>Place a call</title>
		<header>...your header...</header>
		<main>{{ .Dialer }}</main>
	`))
	layout := func(w http.ResponseWriter, r *http.Request, fragment template.HTML) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return hostTpl.Execute(w, map[string]any{"Dialer": fragment})
	}

	h, err := browsercall.New(browsercall.Config{
		AccountSid:   "AC11111111111111111111111111111111",
		APIKey:       "SK22222222222222222222222222222222",
		APISecret:    "your-api-key-secret",
		TwiMLAppSid:  "AP33333333333333333333333333333333",
		AuthToken:    "your-account-auth-token",
		CallerID:     "+14155550199",
		ScriptURL:    "/static/twilio-voice-sdk.js",
		DialerLayout: layout,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = h
}

// onlyLoggedInUsers is a stand-in for whatever auth your host app
// uses. Authorize is consulted on /new and /token; /voice is gated by
// X-Twilio-Signature instead.
func onlyLoggedInUsers(r *http.Request) bool {
	_, ok := r.Context().Value(userCtxKey{}).(string)
	return ok
}

type userCtxKey struct{}
