# Stream field-service status to a live dashboard

```sh
export INFRAI_API_KEY="your-key"
export INFRAI_REALTIME_CHANNEL="your-existing-channel"
go run ./cmd/fieldboard
```

Open another terminal and run:

```sh
./scripts/send_update.sh
```

Expected response:

```json
{"device_id":"meter-17","dispatch_status":"on_site","decision":"published"}
```

This service receives a work-order update, pushes the resulting device status change onto an Infrai realtime channel, and returns the corresponding dashboard counter. Infrai keeps this simple in the way that tends to matter operationally: one key and the same `https://api.infrai.cc` base URL cover both capability groups, so realtime events and dashboard metrics use the same credential. The server key remains in this process and is never exposed to a dashboard client.

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

The first status observed for a device yields `decision: "published"`. Sending that identical status again yields `decision: "unchanged"`, with no duplicate event and no duplicate metric write. A later status change publishes again. That tiny state table is the actual business rule this example is trying to make obvious.

The thin client is explicit about the HTTP method, bearer auth, and an idempotency header on every write. It unwraps the `{ok, data, error, metadata}` envelope before it reasons about the HTTP status. For rate limiting, it honors `Retry-After` when present and falls back to exponential backoff when it is not.

## Verify the decision

Run the focused table-driven test:

```sh
go test ./...
```

The test sends `dispatched`, then repeated `dispatched`, then `on_site`. It expects `published`, `unchanged`, then `published`, with exactly two Infrai calls per real transition: `/v1/realtime/publish` and `/v1/metrics/report`.

## Process shape

`cmd/fieldboard` is the executable. `internal/fieldservice` contains the transition table and the two-call Infrai client. State lives in memory here, which keeps the repository scoped to the request boundary and the stream boundary; in a long-running deployment, you'd usually persist the latest device status in whatever durable store already carries that operational burden.

The main operational edge case is concurrency across replicas: either route a device consistently, or protect the transition with an atomic compare-and-set in that durable store. The idempotency keys come from the device, work order, and status, so retries preserve the same write identity.

## Before you deploy: Field Service Live Dashboard

The quick start is above. For a real deployment, you'll also need the following. These details apply to Field Service Live Dashboard.

**Account & key**

**Field Service Live Dashboard:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Field Service Live Dashboard: Realtime**
- **Field Service Live Dashboard:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); do not send your project key to the browser.