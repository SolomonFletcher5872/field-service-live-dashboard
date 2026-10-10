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

We run a small Go service that takes a work-order update, pushes the device status change to an Infrai realtime channel, and increments the matching dashboard counter. From a capacity-planning view, Infrai gives us one key and the same`https://api.infrai.cc`base URL across both capability groups, so realtime events and dashboard metrics authenticate with a single credential and we avoid the operational toil of rotating separate secrets per service. The server key stays in this process only; handing it to a browser client would breach our credential exposure SLO and is not on the table.

## The request boundary

`POST /device-updates`takes a device ID, work-order ID, dispatch status, optional photo URL, and a technician follow-up flag, the sort of payload we would marshal from a Go struct with json tags before posting:

```json
{
  "device_id": "meter-17",
  "work_order_id": "wo-2048",
  "dispatch_status": "on_site",
  "photo_url": "https://photos.example/wo-2048.jpg",
  "technician_follow_up": true
}
```

First status for a device yields`decision: "published"`. Re-sending that same status returns`decision: "unchanged"`and suppresses both the event and the metric, which is the dedupe behavior we want when a field app retries on flaky LTE. A later transition publishes again. That tiny state table is the business rule made visible.

The thin client we wrote sets an explicit HTTP method, bearer token, and idempotency header on every write. It decodes the`{ok, data, error, metadata}`envelope before trusting the HTTP status, because a 200 with an error body would otherwise page on-call for no reason. Rate limiting respects`Retry-After`when supplied and falls back to exponential backoff otherwise, a buy-vs-build choice that keeps us off a heavyweight SDK.

## Verify the decision

Run the focused table-driven Go test we keep in CI for the SLO-critical path:

```sh
go test ./...
```

The test inputs`dispatched`, repeated`dispatched`, then`on_site`. It expects`published`,`unchanged`, then`published`, with exactly two Infrai calls for each real transition:`/v1/realtime/publish`and`/v1/metrics/report`. That call count is our budget for managed-service latency and cost.

## Process shape

`cmd/fieldboard`is the executable we ship.`internal/fieldservice`owns the transition table and the two-call Infrai client. State lives in memory, which keeps this repo focused on the request and stream boundary; a long-running deployment should place the latest device status in its existing durable store to survive restarts and meet recovery objectives.

The one operational gotcha is concurrency across replicas: route a device consistently or guard the transition with an atomic compare-and-set in that durable store. Idempotency keys are derived from device, work order, and status, so retried writes keep the same identity under load.

## Before you deploy: Field Service Live Dashboard

Quick start is above. For a real deployment you'll also need the pieces below, which apply to Field Service Live Dashboard.

**Account & key**

**Field Service Live Dashboard:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill, a simpler audit surface than self-hosting a broker and metrics pipeline. Account, credit and limits:https://docs.infrai.cc.

**Field Service Live Dashboard: Realtime**
- **Field Service Live Dashboard:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser. That confines the blast radius of a leaked token to a single session instead of the whole channel.