// Entry point for the browser bundle. Pulls in @twilio/voice-sdk and
// exposes Twilio.Device + Twilio.Call as globals on window, matching the
// shape the dialer page expects (`new Twilio.Device(token, opts)` etc).
//
// Twilio's docs document only the ESM import (`import { Device } from
// "@twilio/voice-sdk"`) and no longer publish a CDN, so we bundle the
// SDK ourselves into static/js/twilio-voice-sdk.js and embed it via
// go-bindata at build time. Downstream consumers of the browsercall
// package supply their own bundle URL via browsercall.Config.ScriptURL.

import { Call, Device } from '@twilio/voice-sdk';

const ns = (typeof window.Twilio === 'object' && window.Twilio !== null) ? window.Twilio : {};
ns.Device = Device;
ns.Call = Call;
window.Twilio = ns;
