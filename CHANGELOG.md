# Changes

## Unreleased

- browsercall: the dialer's To field now uses `type="tel"` with
  `inputmode="tel"`, so phones bring up the numeric dialpad by default.
- browsercall: pressing Enter in the To field now places the call, equivalent
  to clicking the Call button.

## 1.8.1 - 2026-05-27

- Add `default_phone_region` config option (ISO 3166-1 alpha-2 country code,
  e.g. "US") so the browser dialer accepts local phone number formats like
  "(925) 943-5839" in addition to E.164. Numbers are normalized to E.164 via
  libphonenumber before dialing. When unset, only E.164 is accepted (existing
  behavior).

## 1.8.0 - 2026-05-22

- Update imports to reference github.com/kevinburke/twilio-go/v2 (and v2.11.0 of
  the library).
- Add browser calling: place outbound voice calls from the Calls page using the
  Twilio Voice JS SDK. Gated on the new `can_make_calls` user permission and
  the new `twilio_api_key`, `twilio_api_secret`, and `twilio_twiml_app_sid`
  settings; reuses `default_sending_phone_number` as caller ID. The
  `/dial/voice` TwiML webhook verifies the `X-Twilio-Signature` header. A new
  `logrole_create_twiml_app` binary creates or updates the required TwiML
  Application against the Twilio API.
- Improve browser-call diagnostics by logging the TwiML branch returned by the
  `/dial/voice` webhook, including the signed URL and Twilio `CallSid`, and
  initialize the Twilio Voice SDK from the Call button so browsers allow audio
  startup.
- Add an embeddable `github.com/kevinburke/logrole/browsercall` package so
  other apps can mount the dialer (and its token/voice endpoints) under any
  URL prefix. The logrole server itself now uses this package internally.
- Bundle the Twilio Voice JS SDK at build time via npm + esbuild rather than
  loading it from a CDN. The SDK is embedded into the Go binary along with
  the rest of the static assets; run `make assets` to rebuild it after
  bumping `@twilio/voice-sdk` in `package.json`.
- Align templates with Bootstrap 5 navbar, form, spacing, and grid classes, and
  remove old inline layout CSS and repeated spacer markup.

## 1.7.1 - 2026-05-20

- Replace `gopkg.in/yaml.v2` with `github.com/goccy/go-yaml` for config and
  policy parsing.
- Remove the built-in Sentry/raven error reporter while keeping the generic
  error-reporter registration hook. Unknown configured reporters now fail
  during settings construction instead of silently falling back to noop.

## 1.7.0 - 2026-05-19

- Support mounting Logrole under a URL base path. Generated links, redirects,
  forms, OpenSearch URLs, and error pages now respect the configured base path.

## 1.6.1 - 2026-05-19

- Modernize the project for Go modules and Go 1.26, replacing the old dep and
  vendor workflow and refreshing dependencies.
- Add Buildkite CI coverage for formatting, linting, tests, and builds, plus
  updated Makefile targets for tests, assets, releases, authors, and benchmarks.
- Add UI support for sending outbound SMS messages, including permission
  checks, CSRF protection, a new message form, reply/new-message actions, and a
  configurable default sending phone number.
- Update templates and styles for newer Bootstrap markup and vendored CSS.
- Update HTTP, logging, REST, and Twilio API usage to current packages and
  context-aware request patterns.

## 1.6 - 2017-07-20

- Switch to using dep
- Switch to using kevinburke/twilio-go
- Switch to using stdlib context

## 1.5 - 2017-02-13

Ensure repository works with latest version of kevinburke/rest.

## 1.4 - 2017-01-02

- Use a monotonic clock for reporting durations.
- Slight improvements to error handling and cancellation when actions are
  completed in parallel.
- 3 day alert view had problems.
- Slight improvement for list view filters on normal sized laptops.

## 1.1 - 2016-11-10

Tweak the README/homepage and describe how to contribute to the project.

## 1.0 - 2016-11-10

We made it! Link from every phone number on the site to the phone number
instance view.

## 0.77 - 2016-11-10

Flush out the phone number instance view - show calls and messages to and from
this number. Add links to the fuller list of those resources. Show dates in
a shorter format if a message is old.

## 0.76 - 2016-11-10

Flush out the phone number list view (paging, filters). Implement a phone
number instance view. Redirect /phone-numbers/PN123 to /phone-numbers/+1410...
and implement tab-to-search.

## 0.73 - 2016-11-08

Add a phone number list view.

## 0.72 - 2016-11-08

Reject invalid query parameters on list views

Implement per-user/group MaxResourceAge settings.

Refactor template generation a little bit.

## 0.71 - 2016-11-07

Implement caching for Messages

Show whether a result was returned from the cache, and if so how old that
result is. Alters services.Duration to show fewer bits after the decimal if the
Duration is larger than one second.

## 0.70 - 2016-11-07

Implement search filters for Alerts

## 0.69 - 2016-11-07

Add back caching for calls/messages/conferences

## 0.67 - 2016-11-04

Switch the date filtering system from day-and-UTC based to timezone aware, to
the hour filtering of calls, messages, and conferences.

## 0.66 - 2016-11-04

Cosmetic changes - changing "redacted" to "hidden" and reordering some filters.

## 0.61 - 2016-11-03

Implement an Alerts list view.

## 0.58 - 2016-11-02

Implement multi-user permissions

Specify `policy` or `policy_file` to define permission groups, and define users
to exist in those groups, as well as a default group for unknown users. Support
policy in the GoogleAuthenticator and the BasicAuthAuthenticator. Document how
policy behaves if it is/isn't specified, and how it interacts with
allowed_domains.

Moves all of the Authenticator code into the config directory. The interaction
between GoogleAuthenticator and the server directory is a little complicated -
we want to render a 401 error if Google auth fails, which needs to be done from
the server directory, but GoogleAuthenticator shouldn't necessarily live there.
We also need to get a URL from GoogleAuthenticator to show on the login page.
We hack around this, I'm not super happy with it, but it works for the moment.
Open to better ideas about how this should work.

Move the YAML config out of the logrole_server binary and into the config
directory. Add Policy to it, and a custom parser for UserSettings. These let
other Go code load a Logrole YAML file, if they want.

Add more documentation around possibly-confusing settings. Document how to get
a Google client ID and client secret.

Fix errors in Google authentication, and add a whole bunch of tests around
policies, Google auth, and Basic auth. Removes some unused code that set
a global map in the config directory.

Add tools in write_config_from_env to download a policy file from a URL (for
Heroku deployment, if you can't include the permissions as part of the Git
repo).

## 0.56 - 2016-10-31

Highlight Call list rows in red if the call ended unsuccessfully.

## 0.55 - 2016-10-28

Show error/warning information about a Call on the instance page.

Messages that resulted in an error are highlighted in red.

## 0.54 - 2016-10-28

You can configure timezones via config.yml, and the timezones in the menu bar
are now dynamic.

## 0.51 - 2016-10-28

Gzip static files so they get sent to the client more quickly. It would be nice
to also gzip the HTML, but this would be vulnerable to BEAST/CRIME attacks on
SSL.

## 0.50 - 2016-10-27

Add a Conference instance view.

## 0.48 - 2016-10-27

Renamed the binaries from `server` and `write_config_from_env` to
`logrole_server` and `logrole_write_config_from_env` to avoid conflicts with
other Go binaries.

Add `google_allowed_domains` config variable to restrict access to email
addresses that are part of a certain domain.
