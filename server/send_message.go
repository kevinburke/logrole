package server

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/views"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rest/resterror"
	twilio "github.com/kevinburke/twilio-go/v2"
)

// sendMessageRoute matches the GET form page. Sends are handled by POSTs to
// /messages, registered separately in serve.go.
var sendMessageRoute = regexp.MustCompile(`^/messages/new$`)

type sendMessageServer struct {
	*slog.Logger
	Client                    views.Client
	DefaultSendingPhoneNumber string
	SecretKey                 *[32]byte
	urls                      urlBuilder
	tpl                       *template.Template
}

type sendMessageData struct {
	From      string
	To        string
	Body      string
	CSRFToken string
	Err       string
	// AvailableNumbers is populated with the account's incoming phone numbers
	// so the user can pick a "From" if the default is empty or they want to
	// override it.
	AvailableNumbers []twilio.PhoneNumber
}

func (s *sendMessageData) Title() string { return "Send Message" }

func newSendMessageServer(l *slog.Logger, vc views.Client, defaultFrom string, secretKey *[32]byte, basePaths ...string) (*sendMessageServer, error) {
	basePath := optionalBasePath(basePaths)
	s := &sendMessageServer{
		Logger:                    l,
		Client:                    vc,
		DefaultSendingPhoneNumber: defaultFrom,
		SecretKey:                 secretKey,
		urls:                      urlBuilder{basePath: basePath},
	}
	tpl, err := newTpl(template.FuncMap{}, base+sendMessageTpl, basePath)
	if err != nil {
		return nil, err
	}
	s.tpl = tpl
	return s, nil
}

func (s *sendMessageServer) renderForm(w http.ResponseWriter, r *http.Request, code int, data *sendMessageData) {
	if data.CSRFToken == "" {
		data.CSRFToken = generateCSRFToken(s.SecretKey)
	}
	bd := &baseData{Data: data}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if code != 0 {
		w.WriteHeader(code)
	}
	if err := render(w, r, s.tpl, "base", bd); err != nil {
		rest.ServerError(w, r, err)
	}
}

// resolveDefaultFrom returns the configured default From number, or - if the
// configured value is empty and the account has exactly one number - that
// number. Returns "" if no unambiguous default can be determined.
func (s *sendMessageServer) resolveDefaultFrom(r *http.Request) string {
	if s.DefaultSendingPhoneNumber != "" {
		return s.DefaultSendingPhoneNumber
	}
	ctx, cancel := getContext(r.Context(), 3*time.Second)
	defer cancel()
	nums, err := s.Client.GetTwilioNumbers(ctx)
	if err != nil {
		s.Warn("Could not fetch incoming numbers for default From", "err", err)
		return ""
	}
	if len(nums) == 1 {
		return string(nums[0])
	}
	return ""
}

func (s *sendMessageServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := config.GetUser(r)
	if !ok {
		rest.ServerError(w, r, errors.New("no user available"))
		return
	}
	if !u.CanSendMessages() {
		rest.Forbidden(w, r, &resterror.Error{Title: "You do not have permission to send messages"})
		return
	}
	data := &sendMessageData{
		From: s.resolveDefaultFrom(r),
		To:   r.URL.Query().Get("to"),
	}
	// Best-effort: populate available numbers for the From dropdown.
	ctx, cancel := getContext(r.Context(), 3*time.Second)
	defer cancel()
	if nums, err := s.Client.GetTwilioNumbers(ctx); err == nil {
		data.AvailableNumbers = nums
	}
	s.renderForm(w, r, 0, data)
}

// messageCollectionServer dispatches POST /messages to a send handler. We do
// not handle GET on this route - that's owned by messageListServer in
// messages.go.
type messageCollectionServer struct {
	*slog.Logger
	Client                    views.Client
	DefaultSendingPhoneNumber string
	SecretKey                 *[32]byte
	urls                      urlBuilder
	sendTpl                   *template.Template
}

func newMessageCollectionServer(l *slog.Logger, vc views.Client, defaultFrom string, secretKey *[32]byte, basePaths ...string) (*messageCollectionServer, error) {
	basePath := optionalBasePath(basePaths)
	tpl, err := newTpl(template.FuncMap{}, base+sendMessageTpl, basePath)
	if err != nil {
		return nil, err
	}
	return &messageCollectionServer{
		Logger:                    l,
		Client:                    vc,
		DefaultSendingPhoneNumber: defaultFrom,
		SecretKey:                 secretKey,
		urls:                      urlBuilder{basePath: basePath},
		sendTpl:                   tpl,
	}, nil
}

func (s *messageCollectionServer) renderFormErr(w http.ResponseWriter, r *http.Request, code int, data *sendMessageData) {
	if data.CSRFToken == "" {
		data.CSRFToken = generateCSRFToken(s.SecretKey)
	}
	bd := &baseData{Data: data}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := render(w, r, s.sendTpl, "base", bd); err != nil {
		rest.ServerError(w, r, err)
	}
}

func (s *messageCollectionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := config.GetUser(r)
	if !ok {
		rest.ServerError(w, r, errors.New("no user available"))
		return
	}
	if !u.CanSendMessages() {
		rest.Forbidden(w, r, &resterror.Error{Title: "You do not have permission to send messages"})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderFormErr(w, r, http.StatusBadRequest, &sendMessageData{Err: "Could not parse form: " + err.Error()})
		return
	}
	csrf := r.PostForm.Get("csrf_token")
	if err := verifyCSRFToken(csrf, s.SecretKey); err != nil {
		s.Warn("CSRF verification failed on send-message POST", "err", err)
		s.renderFormErr(w, r, http.StatusForbidden, &sendMessageData{
			Err:  "Could not verify the form token; please reload and try again.",
			From: r.PostForm.Get("from"),
			To:   r.PostForm.Get("to"),
			Body: r.PostForm.Get("body"),
		})
		return
	}
	from := strings.TrimSpace(r.PostForm.Get("from"))
	to := strings.TrimSpace(r.PostForm.Get("to"))
	body := r.PostForm.Get("body")
	if from == "" {
		from = s.DefaultSendingPhoneNumber
	}
	if from == "" || to == "" || strings.TrimSpace(body) == "" {
		ctx, cancel := getContext(r.Context(), 3*time.Second)
		defer cancel()
		var nums []twilio.PhoneNumber
		if n, err := s.Client.GetTwilioNumbers(ctx); err == nil {
			nums = n
		}
		s.renderFormErr(w, r, http.StatusBadRequest, &sendMessageData{
			Err:              "From, To, and Body are all required.",
			From:             from,
			To:               to,
			Body:             body,
			AvailableNumbers: nums,
		})
		return
	}
	ctx, cancel := getContext(r.Context(), 3*time.Second)
	defer cancel()
	msg, err := s.Client.SendMessage(ctx, u, from, to, body)
	if err != nil {
		s.Warn("Could not send message", "err", err)
		var nums []twilio.PhoneNumber
		if n, gErr := s.Client.GetTwilioNumbers(ctx); gErr == nil {
			nums = n
		}
		code := http.StatusInternalServerError
		if rerr, ok := err.(*resterror.Error); ok && rerr.Status >= 400 && rerr.Status < 500 {
			code = rerr.Status
		}
		s.renderFormErr(w, r, code, &sendMessageData{
			Err:              cleanError(err),
			From:             from,
			To:               to,
			Body:             body,
			AvailableNumbers: nums,
		})
		return
	}
	sid, sErr := msg.Sid()
	if sErr != nil {
		// User can send but not view; redirect to the list instead.
		http.Redirect(w, r, s.urls.Path("/messages"), http.StatusFound)
		return
	}
	http.Redirect(w, r, s.urls.Path("/messages/"+sid), http.StatusFound)
}
