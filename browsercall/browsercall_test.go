package browsercall

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	twilio "github.com/kevinburke/twilio-go/v2"
)

func validConfig() Config {
	return Config{
		AccountSid:  "AC11111111111111111111111111111111",
		APIKey:      "SK22222222222222222222222222222222",
		APISecret:   "test-secret-do-not-use",
		TwiMLAppSid: "AP33333333333333333333333333333333",
		AuthToken:   "test-auth-token",
		// 555-01XX is the NANP-reserved fictional range. libphonenumber
		// accepts these as valid; the unreserved 555-XXXX numbers it
		// rejects. See https://en.wikipedia.org/wiki/555_(telephone_number).
		CallerID:   "+14155550199",
		PublicHost: "example.com",
		ScriptURL:  "/static/js/twilio-voice-sdk.js",
	}
}

func TestValidateRequiresFields(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Config){
		"missing AccountSid":  func(c *Config) { c.AccountSid = "" },
		"missing APIKey":      func(c *Config) { c.APIKey = "" },
		"missing APISecret":   func(c *Config) { c.APISecret = "" },
		"missing TwiMLAppSid": func(c *Config) { c.TwiMLAppSid = "" },
		"missing AuthToken":   func(c *Config) { c.AuthToken = "" },
		"missing CallerID":    func(c *Config) { c.CallerID = "" },
		"bad CallerID":        func(c *Config) { c.CallerID = "not-e164" },
		"missing ScriptURL":   func(c *Config) { c.ScriptURL = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(&c)
			if _, err := New(c); err == nil {
				t.Errorf("expected error from New, got nil")
			}
		})
	}
}

func TestTokenEndpointEmitsJWT(t *testing.T) {
	t.Parallel()
	h, err := New(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/token", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token, Identity string
		TTL             int `json:"ttl"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("not JSON: %v body=%s", err, w.Body.String())
	}
	if resp.Token == "" || resp.Identity == "" || resp.TTL <= 0 {
		t.Fatalf("incomplete: %+v", resp)
	}
	parts := strings.Split(resp.Token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if !strings.Contains(string(payload), "AP33333333333333333333333333333333") {
		t.Errorf("expected TwiML App SID in payload, got %s", payload)
	}
}

func TestTokenEndpointHonorsAuthorize(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Authorize = func(*http.Request) bool { return false }
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/token", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTokenIdentityOverride(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.TokenIdentity = func(*http.Request) string { return "alice@example.com" }
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/token", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct{ Identity string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// '@' is not in the allowed identity character set so it's substituted.
	if resp.Identity != "alice_example.com" {
		t.Errorf("expected sanitized override identity, got %q", resp.Identity)
	}
}

func signedTwilioPost(cfg Config, target string, form url.Values) *http.Request {
	body := form.Encode()
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = cfg.PublicHost
	signedURL := "https://" + cfg.PublicHost + req.URL.RequestURI()
	sig := twilio.GetExpectedTwilioSignature("", cfg.AuthToken, signedURL, form)
	req.Header.Set("X-Twilio-Signature", sig)
	return req
}

func TestVoiceWebhookSignedHappyPath(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"To": []string{"+14155550123"}, "From": []string{"client:browser-abc"}}
	req := signedTwilioPost(cfg, "/voice", form)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<Dial callerId="+14155550199"`) {
		t.Errorf("missing callerId in TwiML: %s", body)
	}
	if !strings.Contains(body, ">+14155550123</Dial>") {
		t.Errorf("missing dialed number in TwiML: %s", body)
	}
}

func TestVoiceWebhookRejectsBadSignature(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body := url.Values{"To": []string{"+14155550123"}}
	req := httptest.NewRequest("POST", "/voice", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "AAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	req.Host = cfg.PublicHost
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on bad signature, got %d", w.Code)
	}
}

func TestVoiceWebhookRejectsMissingSignature(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body := url.Values{"To": []string{"+14155550123"}}
	req := httptest.NewRequest("POST", "/voice", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = cfg.PublicHost
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on missing signature, got %d", w.Code)
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		region  string
		want    string
		wantErr bool
	}{
		{"+14155550123", "US", "+14155550123", false},
		{"(925) 943-5839", "US", "+19259435839", false},
		{"925-943-5839", "US", "+19259435839", false},
		{"9259435839", "US", "+19259435839", false},
		{"+44 20 7946 0958", "US", "+442079460958", false},
		{"+44 20 7946 0958", "", "+442079460958", false},
		{"020 7946 0958", "GB", "+442079460958", false},
		// No default region: local numbers fail.
		{"(925) 943-5839", "", "", true},
		{"sip:eve@evil.example", "US", "", true},
		{"client:browser-abc", "US", "", true},
		{"not-a-number", "US", "", true},
		{"", "US", "", true},
	}
	for _, tc := range cases {
		got, err := normalizePhoneNumber(tc.in, tc.region)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizePhoneNumber(%q, %q): want error, got %q", tc.in, tc.region, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizePhoneNumber(%q, %q): unexpected error: %v", tc.in, tc.region, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizePhoneNumber(%q, %q) = %q, want %q", tc.in, tc.region, got, tc.want)
		}
	}
}

func TestVoiceWebhookNormalizesLocalNumber(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.DefaultRegion = "US"
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"To":   []string{"(925) 943-5839"},
		"From": []string{"client:browser-abc"},
	}
	req := signedTwilioPost(cfg, "/voice", form)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	body := w.Body.String()
	if !strings.Contains(body, ">+19259435839</Dial>") {
		t.Errorf("expected normalized E.164 number in TwiML: %s", body)
	}
}

func TestVoiceWebhookRejectsNonE164(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"To": []string{"sip:eve@evil.example"}}
	req := signedTwilioPost(cfg, "/voice", form)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 (we render an apology Say), got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<Dial") {
		t.Errorf("expected no <Dial> tag for invalid To: %s", w.Body.String())
	}
}

func TestDialerDefaultPage(t *testing.T) {
	t.Parallel()
	h, err := New(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("expected full HTML page, missing doctype")
	}
	if !strings.Contains(body, `src="/static/js/twilio-voice-sdk.js"`) {
		t.Errorf("expected ScriptURL in <script src>, got %s", body)
	}
	// Mount root: tokenURL is bare "token" so the browser resolves it
	// against the current URL (works under any mount prefix).
	if !strings.Contains(body, `var tokenURL = "token"`) {
		t.Errorf("expected sibling token URL 'token', got %s", body)
	}
	if !strings.Contains(body, "function ensureDevice()") {
		t.Errorf("expected dialer to defer Twilio.Device construction to call flow, got %s", body)
	}
	if strings.Contains(body, "device = new Twilio.Device(data.token") {
		t.Errorf("dialer constructs Twilio.Device during token fetch, before a user gesture: %s", body)
	}
	// html/template strips HTML comments, so the source attribution
	// lives on the container's data-* attributes instead.
	if !strings.Contains(body, `data-source="https://github.com/kevinburke/logrole`) {
		t.Errorf("expected source attribution data-* attrs, got %s", body)
	}
}

func TestDialerCustomLayout(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.DialerLayout = func(w http.ResponseWriter, r *http.Request, fragment template.HTML) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := w.Write([]byte("<custom-chrome>" + string(fragment) + "</custom-chrome>"))
		return err
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "<custom-chrome>") {
		t.Errorf("expected custom chrome wrapper, got %s", body)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Errorf("expected DialerLayout to suppress default page, got %s", body)
	}
}

func TestSiblingTokenPath(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"/phone", "/phone/token"},
		{"/phone/", "/phone/token"},
		{"/", "token"},
		{"", "token"},
	}
	for _, c := range cases {
		got := siblingTokenPath(c.in)
		if got != c.want {
			t.Errorf("siblingTokenPath(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
