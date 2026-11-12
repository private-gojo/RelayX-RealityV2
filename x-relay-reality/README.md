# X-Relay Reality V2

Cloud Run WebSocket carrier for a byte-transparent Xray VLESS + XHTTP + REALITY connection.

## Architecture

Android Xray client -> local TCP-to-WebSocket adapter -> Cloud Run WSS -> server WebSocket-to-TCP bridge -> local Xray XHTTP + REALITY inbound -> freedom outbound.

The outer WebSocket carries the inner TCP stream unchanged. Xray performs VLESS, XHTTP, and REALITY end to end. There is no second VLESS-WS hop or TLS-terminating reverse proxy in this path.

## Defaults

| Setting | Value |
| --- | --- |
| Xray | 26.11.9 |
| XHTTP path | /reality |
| REALITY SNI | firebaseremoteconfigrealtime.googleapis.com |
| REALITY target | firebaseremoteconfigrealtime.googleapis.com:443 |
| XHTTP mode | auto |
| Xray server listener | 127.0.0.1:10000 |
| Cloud Run listener | PORT, default 8080 |
| Client adapter | 127.0.0.1:11000 |
| Local SOCKS5 | 127.0.0.1:10808 |

Credentials are fixed test values in the source for repeatable bring-up. They are public test credentials and must not be used for production.

## Local client

Requires Go and Xray on Android/Termux.

```sh
bash scripts/start-client.sh
```

Enter only the Cloud Run hostname when prompted. Point the Android proxy client to SOCKS5 127.0.0.1:10808.

## Cloud Run deployment

Build/publish the container from the GitHub Actions workflow, then deploy from Cloud Shell:

```sh
bash scripts/deploy.sh ghcr.io/private-gojo/relayx-reality-v2:latest
```

The service uses WebSockets and a 3600-second request timeout. Reconnect support is required for long-running sessions. Do not enable end-to-end HTTP/2 for the outer WebSocket service.

## Validation

The workflow runs Go formatting, module verification, go vet, race-enabled tests, shell syntax checks, Docker build, Xray config validation, and Xray version verification.

A green build validates compilation and generated config acceptance. It does not prove carrier-specific connectivity or end-to-end performance.
