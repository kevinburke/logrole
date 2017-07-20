package server

import (
	"net/http"

	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/services"
)

func Example() {
	settings := &config.Settings{
		PublicHost: "myapp.com",
		SecretKey:  services.NewRandomKey(),
	}
	s, _ := NewServer(settings)
	http.Handle("/", s)
	http.ListenAndServe(":4114", nil)
}
