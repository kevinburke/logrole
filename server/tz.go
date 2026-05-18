package server

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/kevinburke/logrole/services"
)

type tzServer struct {
	*slog.Logger
	LocationFinder          services.LocationFinder
	AllowUnencryptedTraffic bool
}

func (t *tzServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO csrf
	if err := r.ParseForm(); err != nil {
		t.Warn("Error parsing form on TZ page", "err", err)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	tz := r.PostForm.Get("tz")
	ok := t.LocationFinder.SetLocation(w, tz, !t.AllowUnencryptedTraffic)
	if !ok {
		t.Warn("Could not set location on request", "loc", tz)
	}
	g := r.PostForm.Get("g")
	u, err := url.Parse(g)
	if err == nil {
		http.Redirect(w, r, u.Path, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}
