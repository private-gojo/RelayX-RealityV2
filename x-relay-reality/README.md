# X-Relay Reality V2

Cloud Run WebSocket carrier with a Zero-Termux NetMod profile, plus the optional legacy Termux bridge.

## Zero-Termux architecture

NetMod Xray client (VLESS + XHTTP + REALITY)
-> client-side `dialerProxy`
-> VLESS + WebSocket + TLS to Cloud Run `/_chain`
-> Go WebSocket frame proxy
-> local Xray VLESS + WebSocket inbound on `127.0.0.1:18080`
-> Xray Freedom redirect to `127.0.0.1:18443`
-> local Xray VLESS + XHTTP + REALITY inbound
-> Freedom outbound to the requested destination.

Cloud Run terminates the outer HTTPS connection. The inner REALITY/XHTTP stream remains inside the WebSocket frames and reaches the local REALITY inbound unchanged.

## NetMod import

Open the generated profile endpoint:

```text
https://<cloud-run-service-host>/profile.json
```

Import the returned raw Xray JSON in NetMod. The profile uses the service host from the incoming HTTP Host header, so the URL does not need manual edits.

No Termux process or local Go adapter is required for this profile. The client core must support Xray outbound `streamSettings.sockopt.dialerProxy` and WebSocket custom headers.

## Optional legacy Termux bridge

The existing TCP-to-WebSocket adapter remains available for compatibility.

- Legacy WebSocket paths: `/reality` and `/_ws`
- Xray server-side REALITY/XHTTP listener: `127.0.0.1:18443`
- Local adapter listener: `127.0.0.1:11000`
- Local SOCKS5 listener: `127.0.0.1:10808`

Requires Go and Xray on Android/Termux:

```sh
cd x-relay-reality
bash scripts/start-client.sh
```

## Defaults

| Setting | Value |
| --- | --- |
| Xray | 26.11.9 |
| XHTTP path | /reality |
| REALITY SNI | firebaseremoteconfigrealtime.googleapis.com |
| REALITY target | firebaseremoteconfigrealtime.googleapis.com:443 |
| XHTTP mode | auto |
| Public chain path | /_chain |
| Xray WebSocket chain listener | 127.0.0.1:18080 |
| Xray REALITY/XHTTP listener | 127.0.0.1:18443 |
| Cloud Run listener | PORT, default 8080 |

Credentials are fixed test values in the source for repeatable bring-up. They are public test credentials and must not be used for production.

## Cloud Run deployment

Build/publish the container from the GitHub Actions workflow, then deploy from Cloud Shell:

```sh
cd x-relay-reality
bash scripts/deploy.sh ghcr.io/private-gojo/relayx-reality-v2:latest
```

The service uses WebSockets and a 3600-second request timeout. Cloud Run WebSocket requests are subject to this timeout. Reconnect support is required for long-running sessions. Do not enable end-to-end HTTP/2 for the outer WebSocket service.

## Validation

The workflow runs Go formatting, module verification, go vet, race-enabled tests, shell syntax checks, Docker build, Xray validation of the server, legacy client, and generated NetMod chained profile, plus Xray version verification.

A green build validates compilation, generated Xray config acceptance, and the local chain proxy tests. It does not prove carrier-specific connectivity or NetMod app import and end-to-end performance on a device.
