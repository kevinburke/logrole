// Command logrole_create_twiml_app creates (or updates) a Twilio TwiML
// Application whose Voice URL points at this server's /calls/voice endpoint.
// Browser-initiated outbound calls go through that TwiML App.
//
// The new Application SID is printed to stdout in YAML form so it can be
// appended to config.yml:
//
//	twilio_twiml_app_sid: APxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
//
// Account SID and Auth Token are read from the --config file by default;
// override with --account-sid / --auth-token.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/kevinburke/handlers"
	"github.com/kevinburke/logrole/config"
	"github.com/kevinburke/logrole/server"
	twilio "github.com/kevinburke/twilio-go"
)

var logger *slog.Logger

func init() {
	flag.Usage = func() {
		os.Stderr.WriteString(`logrole_create_twiml_app

Create a Twilio TwiML Application whose Voice URL points at this server's
public /calls/voice endpoint. The application's SID is printed in YAML form
suitable for appending to config.yml:

  twilio_twiml_app_sid: APxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

The Twilio AccountSid and AuthToken are read from --config; pass --account-sid
and --auth-token to override.

If --twiml-app-sid is provided, that application's voice URL is updated
instead of creating a new one.

Usage of logrole_create_twiml_app:
`)
		flag.PrintDefaults()
	}
	logger = handlers.Logger
}

func main() {
	var (
		cfgPath      = flag.String("config", "config.yml", "Path to logrole config.yml (for AccountSid/AuthToken/public_host/base_path)")
		accountSid   = flag.String("account-sid", "", "Twilio AccountSid (overrides config)")
		authToken    = flag.String("auth-token", "", "Twilio AuthToken (overrides config)")
		publicHost   = flag.String("public-host", "", "Public host serving this logrole instance (overrides config)")
		basePath     = flag.String("base-path", "", "URL base path where logrole is mounted (overrides config)")
		friendlyName = flag.String("friendly-name", "logrole browser calling", "Friendly name for the TwiML App")
		appSid       = flag.String("twiml-app-sid", "", "If set, update this existing Application instead of creating one")
		allowHTTP    = flag.Bool("allow-http", false, "Build an http:// (not https://) voice URL. For local testing only.")
		showVersion  = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()
	if *showVersion {
		fmt.Fprintf(os.Stdout, "logrole_create_twiml_app version %s\n", server.Version)
		return
	}

	c := new(config.FileConfig)
	if data, err := os.ReadFile(*cfgPath); err == nil {
		if err := yaml.Unmarshal(data, c); err != nil {
			logger.Error("Couldn't parse config file", "err", err, "path", *cfgPath)
			os.Exit(2)
		}
	} else if *cfgPath != "config.yml" {
		logger.Error("Couldn't open config file", "err", err, "path", *cfgPath)
		os.Exit(2)
	}

	asid := firstNonEmpty(*accountSid, c.AccountSid)
	tok := firstNonEmpty(*authToken, c.AuthToken)
	host := firstNonEmpty(*publicHost, c.PublicHost)
	bp := firstNonEmpty(*basePath, c.BasePath)
	if asid == "" || tok == "" {
		logger.Error("Missing Twilio account_sid / auth_token; supply via --config, --account-sid/--auth-token")
		os.Exit(2)
	}
	if host == "" {
		logger.Error("Missing public_host; supply via --config or --public-host")
		os.Exit(2)
	}
	normBase, err := config.NormalizeBasePath(bp)
	if err != nil {
		logger.Error("Invalid base_path", "err", err)
		os.Exit(2)
	}
	scheme := "https"
	if *allowHTTP {
		scheme = "http"
	}
	voiceURL := fmt.Sprintf("%s://%s%s/dial/voice", scheme, strings.TrimRight(host, "/"), normBase)

	client := twilio.NewClient(asid, tok, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data := url.Values{}
	data.Set("FriendlyName", *friendlyName)
	data.Set("VoiceUrl", voiceURL)
	data.Set("VoiceMethod", "POST")

	var app *twilio.Application
	if *appSid != "" {
		app, err = client.Applications.Update(ctx, *appSid, data)
		if err != nil {
			logger.Error("Couldn't update TwiML app", "err", err, "sid", *appSid)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "Updated TwiML Application %s\n", app.Sid)
	} else {
		app, err = client.Applications.Create(ctx, data)
		if err != nil {
			logger.Error("Couldn't create TwiML app", "err", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "Created TwiML Application %s (voice_url=%s)\n", app.Sid, voiceURL)
	}
	fmt.Fprintf(os.Stdout, "twilio_twiml_app_sid: %s\n", app.Sid)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
