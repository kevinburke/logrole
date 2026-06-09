package server

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kevinburke/logrole/browsercall"
	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/rest/v2"
)

// dialerChromeTpl wraps the browsercall dialer fragment in logrole's site
// chrome. Parsed against base.html so the dialer page keeps the navbar,
// footer, and timezone selector that the rest of the UI has.
const dialerChromeTpl = `{{ define "content" }}{{ .Fragment }}{{ end }}`

// chromeData satisfies the data interface expected by base.html (which
// reads .Data.Title). Fragment is the HTML body rendered by browsercall.
type chromeData struct {
	Fragment template.HTML
}

func (c *chromeData) Title() string { return "Place a Call" }

// canMakeCallsAuthorizer adapts logrole's per-user permission system to
// browsercall.Config.Authorize.
func canMakeCallsAuthorizer(r *http.Request) bool {
	u, ok := config.GetUser(r)
	return ok && u.CanMakeCalls()
}

// newBrowserCallHandler builds the bundled browser-calling handler.
// Returns (nil, nil) when settings.BrowserCallingEnabled() is false so
// the caller can skip route registration entirely.
//
// The returned handler is mounted at <basePath>/dial, with the token
// and voice endpoints at <basePath>/dial/token and <basePath>/dial/voice.
// We strip the prefix internally so the underlying browsercall.Handler
// sees the "", /token, /voice paths it dispatches on.
func newBrowserCallHandler(l *slog.Logger, settings *config.Settings) (http.Handler, error) {
	if !settings.BrowserCallingEnabled() {
		return nil, nil
	}
	basePath, err := config.NormalizeBasePath(settings.BasePath)
	if err != nil {
		return nil, err
	}
	chromeTpl, err := newTpl(template.FuncMap{}, base+dialerChromeTpl, basePath)
	if err != nil {
		return nil, err
	}
	layout := func(w http.ResponseWriter, r *http.Request, fragment template.HTML) error {
		bd := &baseData{
			LF:   settings.LocationFinder,
			Data: &chromeData{Fragment: fragment},
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := render(w, r, chromeTpl, "base", bd); err != nil {
			rest.ServerError(w, r, err)
			return err
		}
		return nil
	}
	h, err := browsercall.New(browsercall.Config{
		AccountSid:    settings.Client.AccountSid,
		APIKey:        settings.TwilioAPIKey,
		APISecret:     settings.TwilioAPISecret,
		TwiMLAppSid:   settings.TwilioTwiMLAppSid,
		AuthToken:     settings.Client.AuthToken,
		CallerID:      settings.DefaultSendingPhoneNumber,
		PublicHost:    settings.PublicHost,
		AllowHTTP:     settings.AllowUnencryptedTraffic,
		DefaultRegion: settings.DefaultPhoneRegion,
		Logger:        l,
		Authorize:     canMakeCallsAuthorizer,
		DialerLayout:  layout,
		ScriptURL:     basePath + "/static/js/twilio-voice-sdk.js",
		Version:       Version,
	})
	if err != nil {
		return nil, err
	}
	prefix := basePath + "/dial"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok {
			// Defensive: should be impossible given the regexps in
			// serve.go. Fail loudly rather than silently pretending
			// the request is for /new.
			rest.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		u := *r.URL
		u.Path = trimmed
		r2.URL = &u
		h.ServeHTTP(w, r2)
	}), nil
}
