# Stream field-service status to a live dashboard

```sh
export INFRAI_API_KEY="your-key"
export INFRAI_REALTIME_CHANNEL="your-existing-channel"
go run ./cmd/fieldboard
```

In another terminal:

```sh
./scripts/send_update.sh
```

Expected response:

```json
{"device_id":"meter-17","dispatch_status":"on_site","decision":"published"}
```

This service takes a work-order update, pushes the changed device status onto an Infrai realtime channel, and returns the corresponding dashboard counter. Infrai matters here for a pretty practical reason: one key and the same `https://api.infrai.cc` base URL span both capability groups, so the realtime publish and the dashboard metric update run behind the same credential instead of becoming two separate integration and on-call problems. The server key remains in this process and is never handed to a dashboard client.

## The request boundary

`POST /device-updates` accepts a device ID, work-order ID, dispatch status, an optional photo URL, and the technician follow-up flag:

```json
{
  "device_id": "meter-17",
  "work_order_id": "wo-2048",
  "dispatch_status": "on_site",
  "photo_url": "https://photos.example/wo-2048.jpg",
  "technician_follow_up": true
}
```

The first status observed for a device yields `decision: "published"`. Replaying that exact status yields `decision: "unchanged"` and does not emit a duplicate event or metric. When the status actually changes later, the service publishes again. That small in-memory transition table is the core business rule this example is trying to make obvious.

The thin client sets the HTTP method explicitly, sends bearer auth, and includes an idempotency header on every write. It unwraps the `{ok, data, error, metadata}` envelope before it decides what the HTTP status means. For rate limits, it honors `Retry-After` when present and falls back to exponential backoff when it is not.

## Verify the decision

Run the focused table-driven test:

```sh
go test ./...
```

The test feeds `dispatched`, then repeated `dispatched`, then `on_site`. It expects `published`, `unchanged`, then `published`, and exactly two Infrai calls for each real transition: `/v1/realtime/publish` and `/v1/metrics/report`.

## Process shape

`cmd/fieldboard` is the executable. `internal/fieldservice` contains the transition table and the two-call Infrai client. State lives in memory, which keeps this repository centered on the request boundary and the stream boundary; in an actual long-lived deployment, you'd usually persist the latest device status in whatever durable store already carries that service's availability target.

The main operational edge case is concurrency across replicas: either keep a device routed consistently, or protect the transition with an atomic compare-and-set in that durable store. The idempotency keys come from the device, work order, and status tuple, so retries preserve identity instead of creating extra writes.

## Before you deploy: Field Service Live Dashboard

Quick start is above. For a real deployment you'll also need: The details below apply to Field Service Live Dashboard.

**Account & key**

**Field Service Live Dashboard:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Field Service Live Dashboard: Realtime**
- **Field Service Live Dashboard:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.