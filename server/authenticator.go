package server

import (
	"log/slog"
	"net/http"

	"github.com/kevinburke/logrole/config"
)

type logoutServer struct {
	*slog.Logger
	Authenticator config.Authenticator
}

func (l *logoutServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l.Authenticator.Logout(w, r)
}

var theUser = config.NewUser(&config.UserSettings{
	CanViewNumMedia:       true,
	CanViewMessages:       true,
	CanViewMessageFrom:    true,
	CanViewMessageTo:      true,
	CanViewMessageBody:    false,
	CanViewMessagePrice:   false,
	CanViewMedia:          true,
	CanViewCalls:          true,
	CanViewCallFrom:       true,
	CanViewCallTo:         true,
	CanViewCallPrice:      false,
	CanViewNumRecordings:  true,
	CanPlayRecordings:     true,
	CanViewRecordingPrice: false,
	CanViewConferences:    true,
	CanViewAlerts:         true,
})
