package server

import (
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kevinburke/logrole/browsercall"
	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/services"
	"github.com/kevinburke/logrole/views"
	"github.com/kevinburke/rest/v2"
	twilio "github.com/kevinburke/twilio-go/v2"
)

// recentCallCount is how many recent outbound calls we list underneath the
// dialer. Small enough that the table doesn't push the keypad off the
// screen; use the "More calls" link for the full list.
const recentCallCount = "10"

// recentCallTimeout bounds the API request for the recent call list. The
// dialer itself does not depend on it, so a slow or failing Twilio request
// degrades to an error message rather than a failed page render.
const recentCallTimeout = 3 * time.Second

// dialerChromeTpl wraps the browsercall dialer fragment in logrole's site
// chrome. Parsed against base.html so the dialer page keeps the navbar,
// footer, and timezone selector that the rest of the UI has.
//
// Underneath the dialer we list the most recent calls placed from the
// dialer's caller ID - when they were placed, who they were placed to, and
// how long they lasted.
const dialerChromeTpl = `{{ define "content" }}{{ .Fragment }}
<h3 class="mt-4">Recent calls</h3>
{{- if .RecentCallsErr }}
<div class="alert alert-warning">
  <p>Could not load recent calls: {{ .RecentCallsErr }}</p>
</div>
{{- else if .RecentCalls }}
{{- if .RecentCalls.Calls }}
<table class="table table-striped">
  <thead>
    <tr>
      <th>Placed</th>
      {{- if .RecentCalls.ShowHeader "To" }}
      <th class="pn">To</th>
      {{- end }}
      {{- if .RecentCalls.ShowHeader "Duration" }}
      <th>Duration</th>
      {{- end }}
    </tr>
  </thead>
  <tbody>
    {{- range .RecentCalls.Calls }}
      {{- if .CanViewProperty "Sid" }}
      <tr class="call {{ if .CanViewProperty "Status" }}{{ if .Failed }}list-error{{ end }}{{ end }}">
        <td class="friendly-date">
          <a href="{{ url (printf "/calls/%s" .Sid) }}" title="View more details">
            {{- if .CanViewProperty "DateCreated" }}
              {{ friendly_date (.DateCreated.Time.In $.Loc) }}
            {{- else }}
            View more details
            {{- end }}
          </a>
        </td>
        {{- if .CanViewProperty "To" }}
          {{- template "phonenumber" .To }}
        {{- end }}
        {{- if .CanViewProperty "Duration" }}
        <td>{{ .Duration.String }}</td>
        {{- end }}
      </tr>
      {{- end }}
    {{- end }}
  </tbody>
</table>
{{- template "copy-phonenumber" }}
<a class="btn btn-info btn-next" href="{{ url "/calls" }}?from={{ .CallerID }}">More calls</a>
{{- else }}
<p>No calls have been placed from {{ .CallerID }}.</p>
{{- end }}
{{- end }}
{{/* end content */}}{{- end }}`

// chromeData satisfies the data interface expected by base.html (which
// reads .Data.Title). Fragment is the HTML body rendered by browsercall;
// RecentCalls is the list of calls rendered underneath it.
type chromeData struct {
	Fragment template.HTML
	// CallerID is the number browser calls are placed from, and the number
	// RecentCalls was filtered on.
	CallerID string
	// RecentCalls is nil when the user may not view calls at all; in that
	// case we render the dialer with no call list and no error.
	RecentCalls    *views.CallPage
	RecentCallsErr string
	Loc            *time.Location
}

func (c *chromeData) Title() string { return "Place a Call" }

// canMakeCallsAuthorizer adapts logrole's per-user permission system to
// browsercall.Config.Authorize.
func canMakeCallsAuthorizer(r *http.Request) bool {
	u, ok := config.GetUser(r)
	return ok && u.CanMakeCalls()
}

// recentCalls fetches the most recent calls placed from callerID.
//
// Twilio's call list cannot be filtered by direction, and a browser call
// shows up as two records: the parent leg from the browser client and the
// child leg the <Dial> verb creates. Filtering on From=callerID selects the
// child legs, which are the ones carrying the dialed number and the talk
// time - along with any other outbound call placed from the same number,
// which is what somebody looking at "recent outbound calls" wants to see.
//
// Returns (nil, "") when the user may not view calls, so the dialer renders
// without a call list instead of with an error.
func recentCalls(r *http.Request, vc views.Client, u *config.User, callerID string) (*views.CallPage, string) {
	if !u.CanViewCalls() {
		// Don't spend a Twilio request on a list we're going to throw away.
		return nil, ""
	}
	ctx, cancel := getContext(r.Context(), recentCallTimeout)
	defer cancel()
	data := url.Values{}
	data.Set("From", callerID)
	data.Set("PageSize", recentCallCount)
	page, _, err := vc.GetCallPageInRange(ctx, u, twilio.Epoch, twilio.HeatDeath, data)
	switch err {
	case nil:
		return page, ""
	case twilio.NoMoreResults:
		return new(views.CallPage), ""
	case config.PermissionDenied, config.ErrTooOld:
		return nil, ""
	default:
		return nil, cleanError(err)
	}
}

// newBrowserCallHandler builds the bundled browser-calling handler.
// Returns (nil, nil) when settings.BrowserCallingEnabled() is false so
// the caller can skip route registration entirely.
//
// The returned handler is mounted at <basePath>/dial, with the token
// and voice endpoints at <basePath>/dial/token and <basePath>/dial/voice.
// We strip the prefix internally so the underlying browsercall.Handler
// sees the "", /token, /voice paths it dispatches on.
func newBrowserCallHandler(l *slog.Logger, vc views.Client, lf services.LocationFinder, settings *config.Settings) (http.Handler, error) {
	if !settings.BrowserCallingEnabled() {
		return nil, nil
	}
	basePath, err := config.NormalizeBasePath(settings.BasePath)
	if err != nil {
		return nil, err
	}
	chromeTpl, err := newTpl(template.FuncMap{
		"is_our_pn": vc.IsTwilioNumber,
	}, base+dialerChromeTpl+phoneTpl+copyScript, basePath)
	if err != nil {
		return nil, err
	}
	callerID := settings.DefaultSendingPhoneNumber
	layout := func(w http.ResponseWriter, r *http.Request, fragment template.HTML) error {
		cd := &chromeData{
			Fragment: fragment,
			CallerID: callerID,
			Loc:      lf.GetLocationReq(r),
		}
		// The dialer route runs behind the authenticator, so a missing
		// user means somebody rewired the routes; render the dialer
		// without a call list rather than leaking one.
		if u, ok := config.GetUser(r); ok {
			cd.RecentCalls, cd.RecentCallsErr = recentCalls(r, vc, u, callerID)
		} else {
			l.Warn("browser calls: no user on dialer request, skipping recent calls")
		}
		bd := &baseData{
			LF:   lf,
			Data: cd,
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
		CallerID:      callerID,
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
