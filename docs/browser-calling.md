# Browser Calling

Logrole can place outbound voice calls from the browser using Twilio's
[Voice JS SDK]. Calls bridge browser audio to the PSTN through a Twilio
phone number on your account, which doubles as the caller ID.

[Voice JS SDK]: https://www.twilio.com/docs/voice/sdks/javascript

A user with the `can_make_calls` permission visits **/dial**, types
a phone number, and presses Call. Their microphone connects to Twilio,
Twilio dials the recipient, and the two audio legs are bridged.

## How it works

```
Browser (Voice JS SDK)
  │  device.connect({params: {To: "+15551234"}})
  ▼
Twilio fetches POST /dial/voice  (signed with X-Twilio-Signature)
  │  Logrole returns <Dial callerId="...">+15551234</Dial>
  ▼
Twilio bridges browser audio ↔ outbound PSTN leg
```

Three endpoints are involved:

| Route          | Who calls it | Auth                                                  |
| -------------- | ------------ | ----------------------------------------------------- |
| `/dial`   | Browser      | Logrole session + `can_make_calls` user permission    |
| `/dial/token` | Browser      | Logrole session + `can_make_calls` user permission    |
| `/dial/voice` | Twilio       | `X-Twilio-Signature` HMAC against your Twilio AuthToken |

`/dial/token` mints a short-lived (1h) [Twilio Access Token][access-token]
that lets the browser register with Twilio's media servers. `/dial/voice`
is the [TwiML webhook][twiml] Twilio hits when the browser initiates a
call — it returns the `<Dial>` TwiML that tells Twilio who to bridge the
call to. Logrole verifies every `/dial/voice` request using the same
`X-Twilio-Signature` mechanism Twilio uses for SMS/call status callbacks.

[access-token]: https://www.twilio.com/docs/iam/access-tokens
[twiml]: https://www.twilio.com/docs/voice/twiml

## One-time setup

You'll need three things from Twilio, in addition to the AccountSid +
AuthToken Logrole already uses for the log viewer.

### 1. A Twilio phone number, used as caller ID

Browsers don't have phone numbers, but the `<Dial>` TwiML Logrole emits
requires a `callerId` — a verified Twilio number on your account.
Logrole reuses the existing `default_sending_phone_number` config key
(the same number the "Send Message" form uses) for this.

If you don't have `default_sending_phone_number` set and your account
has exactly one Twilio number, Logrole picks it automatically. Otherwise
set it explicitly:

```yaml
default_sending_phone_number: "+14155550199"
```

### 2. A Twilio API Key + Secret

Access tokens are signed with an [API Key][api-keys], not your account's
AuthToken directly. The API Key SID starts with `SK...`; the secret is
shown once at creation time.

Create one at <https://www.twilio.com/console/voice/runtime/api-keys>
(or wherever Twilio is hosting the console these days) and add both to
`config.yml`:

```yaml
twilio_api_key:    SKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
twilio_api_secret: <the-secret>
```

If you're configuring via environment variables (i.e. using
`logrole_write_config_from_env`), set `TWILIO_API_KEY` and
`TWILIO_API_SECRET`.

[api-keys]: https://www.twilio.com/docs/iam/keys/api-key

### 3. A TwiML Application pointing at /dial/voice

Browser-initiated calls don't hit our `/dial/voice` URL directly. The
Voice SDK refers to a [TwiML Application][twiml-app] by SID, and that
application's "Voice URL" is what Twilio fetches. So we need a TwiML App
whose Voice URL is `https://<your-logrole-host>/dial/voice` (or
`https://<host><base_path>/dial/voice` if logrole is mounted under a
URL prefix).

Logrole ships a helper that creates the app for you. Once steps 1 and 2
above are done, run:

```bash
logrole_create_twiml_app --config=config.yml
```

It reads AccountSid/AuthToken/public_host/base_path from your config,
calls the Twilio REST API, and prints the new SID to stdout in YAML form:

```
twilio_twiml_app_sid: APxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Paste that line into `config.yml`. If you'd rather create the TwiML App
by hand, the Twilio console works fine — just set its Voice URL to
`https://<public_host><base_path>/dial/voice` with HTTP method POST.

To rotate later, run again with `--twiml-app-sid=AP...` to update the
existing app's Voice URL in place.

[twiml-app]: https://www.twilio.com/docs/usage/api/applications

### 4. Grant `can_make_calls` to your users

`can_make_calls` defaults to `true` for all users. If you've defined
[custom permission groups][permissions], add it to each group that should
be allowed to dial:

```yaml
policy:
    - name: support
      permissions:
          can_make_calls: true
          # ...
```

[permissions]: settings.md#custom-permissions-for-different-groups

## Verifying it works

Once all four values (`default_sending_phone_number`, `twilio_api_key`,
`twilio_api_secret`, `twilio_twiml_app_sid`) are set, the "Place a call
from your browser" link appears at the top of the Calls page for users
with `can_make_calls`. The `/dial` page should say "Ready (identity
browser-...)" once it has fetched a token and initialized the SDK.

A handful of things commonly go wrong on first install:

- **403 on /dial/voice from Twilio**: the `X-Twilio-Signature` doesn't
  match. The signature is computed over the exact URL Twilio sent the
  request to. If a proxy in front of logrole rewrites the path or host,
  or if you set the TwiML App's Voice URL to `http://` but Twilio is
  reaching logrole over `https://`, the verifier will reject. Use the
  `signed_url` field in logrole's warning log to see what URL it tried
  to verify against, and adjust either the proxy or `public_host` so
  they agree.

- **"Could not initialize: token request failed: 403"**: the user
  doesn't have `can_make_calls`.

- **No microphone permission**: browsers gate getUserMedia behind a
  permission prompt and require a secure context (HTTPS or localhost).
  Calls placed over plain HTTP from a non-localhost origin won't work.

- **Static assets missing on /static/js/twilio-voice-sdk.js**: the JS
  bundle is built at `make assets` time. If you `go install ./...`
  without running `make assets` first, the embedded bindata won't
  contain it. Run `make assets` (which runs `npm ci` and esbuild) and
  rebuild.

## Permissions reference

Add to `policy` entries or `policy_file`:

```yaml
can_make_calls: true   # or false; defaults to true
```

The `can_make_calls` permission gates `/dial` and `/dial/token`.
There's no permission gate on `/dial/voice` — it's authenticated by
the `X-Twilio-Signature` header instead.

## Settings reference

| YAML key                       | Environment var          | Description                                                |
| ------------------------------ | ------------------------ | ---------------------------------------------------------- |
| `twilio_api_key`               | `TWILIO_API_KEY`         | API Key SID (`SK...`). Required to enable browser calling. |
| `twilio_api_secret`            | `TWILIO_API_SECRET`      | API Key secret. Required.                                  |
| `twilio_twiml_app_sid`         | `TWILIO_TWIML_APP_SID`   | TwiML App SID (`AP...`). Required.                         |
| `default_sending_phone_number` | n/a (no env mapping)     | E.164 caller ID. Required.                                 |

If any of those four are missing, the routes are not registered and the
"Place a call" link is hidden. Logrole logs `Browser calling disabled:
set ...` at startup so you can confirm.

## Embedding the dialer in another Go app

If you're running another app — say, a personal home server — and want
the same dialer there without running logrole, import the
`browsercall` package:

```go
import "github.com/kevinburke/logrole/browsercall"

h, err := browsercall.New(browsercall.Config{
    AccountSid:  "AC...",
    APIKey:      "SK...",
    APISecret:   "...",
    TwiMLAppSid: "AP...",
    AuthToken:   "...",
    CallerID:    "+14155550199",
    ScriptURL:   "/static/twilio-voice-sdk.js",
    Authorize:   yourAuthFn, // returns false to send 403
})
if err != nil { /* ... */ }
mux.Handle("/phone/", http.StripPrefix("/phone", h))
```

The package routes `/new`, `/token`, and `/voice` under whatever prefix
it's mounted at. By default the dialer page is a complete self-contained
HTML document; pass `Config.DialerLayout` to wrap the dialer fragment
in your own app's chrome instead.

Two things you have to supply yourself:

- **The Twilio SDK bundle**. The package does not ship a copy of
  `@twilio/voice-sdk`; the embedder is responsible for bundling it
  (e.g. via esbuild) and serving the result at `Config.ScriptURL`. Look
  at this repo's `package.json` + `js/twilio-voice-sdk.js` for a minimal
  example.

- **Authentication for /new and /token**. The bundled handler does not
  enforce a session; supply `Config.Authorize` (a `func(*http.Request)
  bool`) or front the routes with your own auth middleware. The `/voice`
  webhook is independently authenticated by `X-Twilio-Signature` against
  `Config.AuthToken` — `Authorize` is not consulted there.

A few optional config knobs worth noting:

- `Config.TokenIdentity func(*http.Request) string` — by default each
  token gets a fresh random identity. Override to bind tokens to a
  known authenticated user.
- `Config.PublicHost` — set when reverse proxies rewrite the `Host`
  header, so `X-Twilio-Signature` verification has the right URL.
- `Config.AllowHTTP` — accept `http://` URLs in the signature check.
  For local development only.

### Embedding the dialer in an existing page

Two integration patterns:

- **Dedicated route, your chrome.** Set `Config.DialerLayout` to a
  function that wraps the rendered fragment in your app's header,
  navigation, etc. The bundled `Handler` still owns the URL but the
  page looks like the rest of your site. logrole uses this internally
  to keep its navbar.

- **Inline in an existing page.** Use `Handler.Fragment(tokenPath)` to
  render just the dialer's body HTML (the form, status pane, and SDK
  `<script>` tag). Splice it into a larger template you render
  yourself. You still need to mount the `Handler` somewhere so its
  `/token` and `/voice` routes are reachable — `tokenPath` should
  point at that mount's `/token`.

See the godoc examples for runnable snippets:
[`ExampleNew`][ex-new], [`ExampleHandler_Fragment`][ex-frag], and
[`ExampleConfig_DialerLayout`][ex-layout].

[ex-new]: https://godoc.org/github.com/kevinburke/logrole/browsercall#example-New
[ex-frag]: https://godoc.org/github.com/kevinburke/logrole/browsercall#example-Handler.Fragment
[ex-layout]: https://godoc.org/github.com/kevinburke/logrole/browsercall#example-Config.DialerLayout

See [godoc][godoc] for the complete API.

[godoc]: https://godoc.org/github.com/kevinburke/logrole/browsercall

## Cost and rate limits

Each browser-placed call is a regular Twilio outbound call — the same
per-minute rate as any other outbound call from the same number.
Twilio's [terms of service][tos] and acceptable use policy apply.

[tos]: https://www.twilio.com/legal/tos
