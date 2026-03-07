# sensing-player

A Go application that polls a WiFi sensor API and displays targeted video ads in a Chrome window based on the number of people detected.

## How it works

1. Polls a sensor endpoint (e.g. `wifi-detect`) at a configurable interval
2. Maps the `people_count` from the sensor reading to an ad using rules defined in `config.yaml`
3. Serves a video player page locally and pushes ad-change events to the browser via Server-Sent Events (SSE)
4. Launches Chrome in kiosk mode pointing at the local player page

## Requirements

- [Go](https://golang.org/) >= 1.22
- Chrome or Chromium installed
- A running sensor API compatible with the expected response shape (see [Sensor API](#sensor-api))

## Setup

```bash
go mod download
```

## Running

```bash
# Production
go run .

# With a custom config file
go run . -config /path/to/config.yaml

# Development (skip launching Chrome)
go run . -no-browser

# Debug logging
go run . -log-level debug
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config.yaml` | Path to config file |
| `-log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-no-browser` | `false` | Skip launching Chrome (useful for testing) |

## Configuration

All configuration lives in `config.yaml`:

```yaml
server:
  host: 127.0.0.1
  port: 8080

sensor:
  endpoint: http://localhost:8000/detected
  poll_interval: 2s
  timeout_seconds: 5

chrome:
  executable_path: ""   # auto-detected if blank
  kiosk_mode: true
  flags:
    - "--disable-extensions"

ads:
  default:
    label: "Default Ad"
    url: "https://example.com/default.mp4"
  small_crowd:
    label: "Small Crowd Ad"
    url: "https://example.com/small.mp4"

rules:
  - min_people: 0
    max_people: 0
    ad_key: default
  - min_people: 1
    max_people: 3
    ad_key: small_crowd

default_ad_key: default
```

### Ad URLs

`url` can be an absolute URL or a local file path served relative to the working directory:

```yaml
ads:
  promo:
    label: "Promo"
    url: "ads/promo.mp4"   # served from ./ads/promo.mp4
```

### Rules

Rules are evaluated in order; the first match wins. Use `-1` for no bound on `min_people` or `max_people`. Falls back to `default_ad_key` if no rule matches.

## Sensor API

The sensor endpoint must return JSON in this shape:

```json
{ "people_count": 3, "activity": "walking", "confidence": 0.9 }
```

| Field | Type | Description |
|-------|------|-------------|
| `people_count` | integer | Number of people detected |
| `activity` | string | Detected activity (e.g. `"walking"`, `"standing"`) |
| `confidence` | float | Detection confidence (0.0 – 1.0) |

The [`wifi-detect`](../wifi-detect) service implements this interface.

## Player endpoints

Once running, the local server exposes:

| Endpoint | Description |
|----------|-------------|
| `GET /` | Video player page (opened by Chrome) |
| `GET /events` | SSE stream of ad-change events |
| `GET /current` | Current ad state as JSON |
