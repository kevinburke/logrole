// Package browsercall provides an embeddable HTTP handler that lets users
// place outbound voice calls from a web browser through Twilio's Voice JS
// SDK.
//
// The handler exposes three routes under whatever path it is mounted at:
//
//	GET  .         - the dialer page (the mount root itself)
//	GET  ./token   - mints a short-lived Twilio access token (JSON)
//	POST ./voice   - the TwiML webhook that Twilio hits when the browser
//	                 places a call. Verifies X-Twilio-Signature.
//
// Mount it from another app like this:
//
//	h, err := browsercall.New(browsercall.Config{
//	    AccountSid:   "AC...",
//	    APIKey:       "SK...",
//	    APISecret:    "...",
//	    TwiMLAppSid:  "AP...",
//	    AuthToken:    "...",
//	    CallerID:     "+14155550199",
//	    ScriptURL:    "/static/js/twilio-voice-sdk.js",
//	})
//	if err != nil { ... }
//	mux.Handle("/phone/", http.StripPrefix("/phone", h))
//
// The bundled dialer page is a complete self-contained HTML document. To
// wrap the dialer in custom application chrome, supply Config.DialerLayout;
// it receives the dialer body fragment and writes the full response.
//
// browsercall does not bundle the @twilio/voice-sdk JavaScript itself.
// Twilio no longer publishes a public CDN, so the embedder must bundle
// the SDK (e.g. via esbuild) and serve it at Config.ScriptURL.
package browsercall

import (
	_ "embed"

	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kevinburke/rest"
	"github.com/kevinburke/rest/resterror"
	twilio "github.com/kevinburke/twilio-go"
	"github.com/kevinburke/twilio-go/token"
	"github.com/ttacon/libphonenumber"
)

// pkgSourceURL is rendered as an HTML comment on the dialer page so
// somebody viewing the page source in dev tools can find the project.
const pkgSourceURL = "https://github.com/kevinburke/logrole/tree/master/browsercall"

// Lifetime of access tokens issued to browser clients. Twilio's max is 24h;
// short enough that a leaked token expires quickly.
const accessTokenTTL = time.Hour

// validPhoneNumber reports whether s parses as an E.164 phone number
// that libphonenumber recognizes as valid for some region. We use it
// for both the configured CallerID and the To value Twilio echoes
// back to the /voice webhook, so SIP URIs ("sip:..."), Twilio Client
// identities ("client:..."), and made-up numbers can't be smuggled
// through the dialer.
func validPhoneNumber(s string) (*libphonenumber.PhoneNumber, error) {
	// Empty defaultRegion: the caller must supply E.164 with a leading
	// "+" or libphonenumber will reject the parse.
	pn, err := libphonenumber.Parse(s, "")
	if err != nil {
		return nil, err
	}
	if !libphonenumber.IsValidNumber(pn) {
		return nil, fmt.Errorf("%q is not a valid phone number", s)
	}
	return pn, nil
}

//go:embed dialer.html
var dialerTplSrc string

//go:embed dialer_page.html
var dialerPageTplSrc string

// Config configures a Handler. AccountSid, APIKey, APISecret, TwiMLAppSid,
// AuthToken, CallerID, and ScriptURL are required; everything else is
// optional.
type Config struct {
	// AccountSid is the Twilio AccountSid (AC...). The access token is
	// scoped to this account.
	AccountSid string

	// APIKey is a Twilio API Key SID (SK...). Create one at
	// https://www.twilio.com/console/voice/runtime/api-keys.
	APIKey string

	// APISecret is the secret paired with APIKey. Twilio only displays
	// this once, at creation time.
	APISecret string

	// TwiMLAppSid (AP...) is the TwiML Application whose Voice URL points
	// at this handler's /voice route. Browser-initiated calls cause
	// Twilio to fetch TwiML from that URL.
	//
	// If you don't have a TwiML App yet, run the bundled helper:
	//
	//   go run github.com/kevinburke/logrole/commands/logrole_create_twiml_app \
	//       --account-sid AC... --auth-token ... \
	//       --public-host your.host.example
	//
	// (or `logrole_create_twiml_app --config=config.yml` if logrole is
	// already configured). It creates the application via the Twilio
	// REST API and prints the SID. Pass `--twiml-app-sid AP...` to
	// update an existing app's Voice URL in place instead.
	TwiMLAppSid string

	// AuthToken is the Twilio account's AuthToken. Used to verify the
	// X-Twilio-Signature header on the /voice webhook.
	AuthToken string

	// CallerID is the E.164 number used as the caller ID on outbound
	// calls. Must be a verified Twilio number on the account.
	CallerID string

	// ScriptURL is the URL where the bundled @twilio/voice-sdk JS is
	// served. The dialer page loads this script and expects it to expose
	// Twilio.Device on window. Twilio no longer publishes a public CDN,
	// so embedders must bundle the SDK (e.g. via esbuild) and host the
	// resulting file.
	ScriptURL string

	// PublicHost overrides r.Host when reconstructing the URL Twilio
	// signed for /voice. Set this if your deployment terminates TLS at a
	// proxy that rewrites the Host header.
	PublicHost string

	// AllowHTTP permits the signature verifier to accept http:// URLs.
	// Off by default; useful for local development against a tunnel that
	// presents http on the inside.
	AllowHTTP bool

	// Authorize, if non-nil, is called on /new and /token to gate access
	// to the dialer. Returning false sends 403. The /voice webhook is
	// always reachable - it's authenticated by X-Twilio-Signature.
	// If nil, both /new and /token are open; rely on your parent app's
	// auth middleware.
	Authorize func(*http.Request) bool

	// DialerLayout, if non-nil, lets the embedder wrap the dialer body
	// fragment in custom chrome. The function is called with the rendered
	// fragment (the status pane, form, and SDK <script>); it must write a
	// complete HTTP response.
	//
	// If nil, the handler serves a minimal self-contained HTML page.
	DialerLayout func(w http.ResponseWriter, r *http.Request, fragment template.HTML) error

	// TokenIdentity, if non-nil, returns the identity string embedded in
	// the access token JWT. Defaults to a random per-token identifier.
	// Override to bind tokens to a known authenticated user.
	TokenIdentity func(*http.Request) string

	// Logger sinks warnings and errors. Defaults to slog.Default().
	Logger *slog.Logger

	// Version, if non-empty, is rendered as an HTML comment in the
	// dialer page alongside a link to this package's source. logrole
	// passes server.Version here. Optional.
	Version string
}

// Validate reports an error if any required field is missing or malformed.
func (c *Config) Validate() error {
	switch {
	case c.AccountSid == "":
		return errors.New("browsercall: AccountSid is required")
	case c.APIKey == "":
		return errors.New("browsercall: APIKey is required")
	case c.APISecret == "":
		return errors.New("browsercall: APISecret is required")
	case c.TwiMLAppSid == "":
		return errors.New("browsercall: TwiMLAppSid is required")
	case c.AuthToken == "":
		return errors.New("browsercall: AuthToken is required (for X-Twilio-Signature verification)")
	case c.CallerID == "":
		return errors.New("browsercall: CallerID is required")
	case c.ScriptURL == "":
		return errors.New("browsercall: ScriptURL is required (bundle @twilio/voice-sdk and host it)")
	}
	if _, err := validPhoneNumber(c.CallerID); err != nil {
		return fmt.Errorf("browsercall: CallerID: %w", err)
	}
	return nil
}

// Handler implements http.Handler. It routes the three sub-paths /new,
// /token, and /voice. Use http.StripPrefix when mounting at a sub-path.
type Handler struct {
	cfg       Config
	logger    *slog.Logger
	dialerTpl *template.Template
	pageTpl   *template.Template
}

// New constructs a Handler. Returns an error if cfg is invalid.
func New(cfg Config) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	dialerTpl, err := template.New("dialer").Parse(dialerTplSrc)
	if err != nil {
		return nil, fmt.Errorf("browsercall: parse dialer template: %w", err)
	}
	pageTpl, err := template.New("page").Parse(dialerPageTplSrc)
	if err != nil {
		return nil, fmt.Errorf("browsercall: parse page template: %w", err)
	}
	return &Handler{cfg: cfg, logger: logger, dialerTpl: dialerTpl, pageTpl: pageTpl}, nil
}

// ServeHTTP dispatches to the three sub-routes. The request path is
// matched after any prefix has been stripped, so the handler can be
// mounted under any URL prefix. The dialer is served at the mount root
// (path "" or "/"); the token and voice endpoints sit beside it.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "", "/":
		if r.Method != http.MethodGet {
			rest.NotAllowed(w, r)
			return
		}
		h.serveDialer(w, r)
	case "/token":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			rest.NotAllowed(w, r)
			return
		}
		h.serveToken(w, r)
	case "/voice":
		if r.Method != http.MethodPost {
			rest.NotAllowed(w, r)
			return
		}
		h.serveVoice(w, r)
	default:
		rest.NotFound(w, r)
	}
}

// authorized reports whether the request should be allowed to reach a
// user-facing endpoint. /voice is independently authenticated by
// X-Twilio-Signature and bypasses Authorize.
func (h *Handler) authorized(r *http.Request) bool {
	if h.cfg.Authorize == nil {
		return true
	}
	return h.cfg.Authorize(r)
}

// dialerFragmentData is what dialer.html renders against.
type dialerFragmentData struct {
	From      string
	ScriptURL string
	// TokenPath is the URL the dialer fetches for its access-token JWT,
	// relative to the dialer page itself.
	TokenPath string
	// SourceURL and SourceVersion are rendered as an HTML comment so
	// somebody reading the page source can find the project that
	// produced it.
	SourceURL     string
	SourceVersion string
}

// pageData is what dialer_page.html renders against.
type pageData struct {
	Title    string
	Fragment template.HTML
}

func (h *Handler) serveDialer(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		rest.Forbidden(w, r, &resterror.Error{Title: "not authorized to place calls"})
		return
	}
	tokenPath := siblingTokenPath(r.URL.Path)
	frag, err := h.renderFragment(tokenPath)
	if err != nil {
		rest.ServerError(w, r, fmt.Errorf("render dialer fragment: %w", err))
		return
	}
	if h.cfg.DialerLayout != nil {
		if err := h.cfg.DialerLayout(w, r, frag); err != nil {
			h.logger.Warn("browsercall: DialerLayout error", "err", err)
		}
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.pageTpl.Execute(w, &pageData{Title: "Place a call", Fragment: frag}); err != nil {
		h.logger.Warn("browsercall: render page", "err", err)
	}
}

// Fragment renders the dialer body fragment as HTML. Use this when you
// want to render the dialer as part of a larger page in your own
// response handler, instead of (or alongside) mounting Handler at its
// own URL.
//
// tokenPath is the absolute or relative URL the browser-side JS will
// fetch its access-token JWT from. You're still responsible for
// serving that endpoint - either by also mounting Handler somewhere
// (which exposes /token at its mount root) and pointing tokenPath at
// it, or by routing /token directly to Handler.ServeHTTP yourself.
// The /voice TwiML webhook (which Twilio calls, not the browser) also
// needs to be reachable; the simplest setup is to mount the bundled
// Handler at some prefix and use Fragment only to embed the dialer UI
// elsewhere.
//
// See the package examples for a worked integration.
func (h *Handler) Fragment(tokenPath string) (template.HTML, error) {
	return h.renderFragment(tokenPath)
}

func (h *Handler) renderFragment(tokenPath string) (template.HTML, error) {
	var sb strings.Builder
	err := h.dialerTpl.Execute(&sb, &dialerFragmentData{
		From:          h.cfg.CallerID,
		ScriptURL:     h.cfg.ScriptURL,
		TokenPath:     tokenPath,
		SourceURL:     pkgSourceURL,
		SourceVersion: h.cfg.Version,
	})
	if err != nil {
		return "", err
	}
	return template.HTML(sb.String()), nil
}

// siblingTokenPath returns the URL path the dialer's JS should fetch
// for its access token. The dialer is served at the mount root, so the
// token endpoint is one segment below: "/phone" → "/phone/token", "/"
// → "/token", "" → "token" (relative).
func siblingTokenPath(dialerPath string) string {
	switch dialerPath {
	case "", "/":
		// Bare "token" lets the browser resolve it against the current
		// URL. Works whether we're at the URL root or mounted under a
		// prefix that didn't include a trailing slash.
		return "token"
	}
	return strings.TrimRight(dialerPath, "/") + "/token"
}

// identityCharRegex matches characters disallowed by Twilio in identities.
var identityCharRegex = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// identityForRequest returns a Twilio-safe identity string for the access
// token. Falls back to a random label when TokenIdentity isn't configured.
func (h *Handler) identityForRequest(r *http.Request) string {
	if h.cfg.TokenIdentity != nil {
		if id := strings.TrimSpace(h.cfg.TokenIdentity(r)); id != "" {
			return sanitizeIdentity(id)
		}
	}
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "browser-call"
	}
	return "browser-" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

func sanitizeIdentity(id string) string {
	id = identityCharRegex.ReplaceAllString(id, "_")
	if len(id) > 121 {
		id = id[:121]
	}
	return id
}

// tokenResponse is what serveToken writes to the browser. TTL is in
// seconds so the dialer can schedule a token refresh.
type tokenResponse struct {
	Token    string `json:"token"`
	Identity string `json:"identity"`
	TTL      int    `json:"ttl"`
}

func (h *Handler) serveToken(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		rest.Forbidden(w, r, &resterror.Error{Title: "not authorized to place calls"})
		return
	}
	identity := h.identityForRequest(r)
	at := token.New(h.cfg.AccountSid, h.cfg.APIKey, h.cfg.APISecret,
		identity, accessTokenTTL)
	at.AddGrant(token.NewVoiceGrant(h.cfg.TwiMLAppSid, nil, "", "", false))
	jwt, err := at.JWT()
	if err != nil {
		rest.ServerError(w, r, fmt.Errorf("sign access token: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(tokenResponse{
		Token:    jwt,
		Identity: identity,
		TTL:      int(accessTokenTTL.Seconds()),
	}); err != nil {
		h.logger.Warn("browsercall: encode token response", "err", err)
	}
}

// twimlResponse is the minimal subset of TwiML we emit.
type twimlResponse struct {
	XMLName xml.Name `xml:"Response"`
	Dial    *twimlDial
	Say     string `xml:"Say,omitempty"`
}

type twimlDial struct {
	CallerID       string `xml:"callerId,attr,omitempty"`
	AnswerOnBridge string `xml:"answerOnBridge,attr,omitempty"`
	Number         string `xml:",chardata"`
}

func (h *Handler) serveVoice(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Twilio-Signature")
	if signature == "" {
		h.logger.Warn("browsercall: /voice hit without X-Twilio-Signature", "ra", r.RemoteAddr)
		rest.Forbidden(w, r, &resterror.Error{Title: "missing X-Twilio-Signature"})
		return
	}
	if err := r.ParseForm(); err != nil {
		rest.BadRequest(w, r, &resterror.Error{Title: "could not parse form: " + err.Error()})
		return
	}

	scheme := "https"
	if h.cfg.AllowHTTP && r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	host := r.Host
	if h.cfg.PublicHost != "" {
		host = h.cfg.PublicHost
	}
	signedURL := scheme + "://" + host + r.URL.RequestURI()
	expected := twilio.GetExpectedTwilioSignature("", h.cfg.AuthToken, signedURL, r.PostForm)
	if expected != signature {
		h.logger.Warn("browsercall: X-Twilio-Signature mismatch",
			"signed_url", signedURL, "got", signature, "expected", expected,
			"ra", r.RemoteAddr)
		rest.Forbidden(w, r, &resterror.Error{Title: "bad X-Twilio-Signature"})
		return
	}

	to := strings.TrimSpace(r.PostForm.Get("To"))
	if _, err := validPhoneNumber(to); err != nil {
		h.logger.Warn("browsercall: invalid To", "to", to, "err", err)
		h.respond(w, &twimlResponse{Say: "Sorry, that number is not valid."})
		return
	}
	h.respond(w, &twimlResponse{
		Dial: &twimlDial{
			CallerID:       h.cfg.CallerID,
			AnswerOnBridge: "true",
			Number:         to,
		},
	})
}

func (h *Handler) respond(w http.ResponseWriter, resp *twimlResponse) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		h.logger.Warn("browsercall: write XML header", "err", err)
		return
	}
	enc := xml.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		h.logger.Warn("browsercall: encode TwiML", "err", err)
	}
}
