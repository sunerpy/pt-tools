# `internal/notify/` — Notification Channels and Delivery

## Role

This package provides transport-neutral notifications, channel factories, fan-out, dedupe, quiet hours, digesting, and durable retry. Supported production adapters are Telegram, QQ/OneBot, generic webhook, WeCom webhook, and the outbound-only Bark, ServerChan, ntfy, DingTalk, and Feishu adapters.

## Core Interface

```go
type Channel interface {
    Type() string
    Init(context.Context, *models.NotificationConf) error
    SupportsInbound() bool
    Send(context.Context, Notification) error
    OnInbound(InboundHandler)
    Close(context.Context) error
    Healthy() bool
}
```

## Layout

```text
internal/notify/
├── channel.go / notification.go # Transport contracts and DTOs
├── registry.go                  # Channel type → factory
├── router.go                    # Enabled-config fan-out, dedupe, timeout
├── outbox.go                    # Durable retry worker
├── digest.go                    # RSS notification batching
├── quiet.go                     # Cross-midnight quiet-hour calculation
└── adapter/
    ├── telegram/                # Inbound/outbound + callback actions
    ├── qq/                      # OneBot/NapCat inbound/outbound
    ├── webhook/                 # Stateless generic webhook
    ├── wecom/                   # WeCom webhook (sends through outbound/)
    ├── bark/ serverchan/ ntfy/  # Outbound-only push services
    ├── dingtalk/ feishu/        # Outbound-only group bots (official hosts only, optional signing)
    └── outbound/                # Shared HTTP client for the outbound-only adapters (see below)
```

## Delivery Paths

- `Router.Route` loads enabled configurations, applies an optional allow-list and 60-second dedupe, initializes/reuses channel instances, and fans out with a bounded timeout.
- Immediate app delivery uses live channel instances. Failures are persisted to `NotificationOutbox` when an outbox is available.
- `OutboxWorker` retries due rows at bounded backoff and marks exhausted rows dead.
- RSS delivery adds a separate idempotency row in `RSSNotificationLog`, applies quota/quiet-hour policy, and may use `DigestBuffer` before sending.

## Stateful Adapter Warning

Do not create a second live Telegram or QQ listener for outbox delivery: it can collide on polling, ports, or connection ownership. Outbox retries go through the production live-channel manager (`OutboxWorker.SetLiveSender`); the factory path is only a fallback for stateless webhook adapters and decrypts `ConfigJSON` first. Changes here require explicit lifecycle tests.

Telegram `Healthy()` follows the latest `getUpdates` result (`pollHealthTransport`): telego retries a failing long poll internally without closing the updates channel, so transport-level results are the only signal. `runInbound` restarts the long poll with backoff if the updates channel closes while the poll context is alive.

## Outbound-only Adapters

Bark, ServerChan, ntfy, DingTalk, Feishu and WeCom send through `outbound.Client.PostJSON`:

- The request is bound to the caller's `ctx`: a timed-out send is canceled, so it cannot arrive late and then be delivered again by the outbox.
- No environment proxy, no redirects (a 3xx is a failure), responses capped at 64 KiB.
- Every connection is checked after DNS (`net.Dialer.Control`): link-local (including `169.254.169.254`), multicast and reserved addresses are never dialed; loopback and private ranges only when the request sets `AllowPrivate` (the user's `allow_private` switch on Bark/ntfy). DingTalk and Feishu webhooks must be the official HTTPS hosts.
- HTTP errors carry only the status code. Adapters pick whitelisted fields (`code`/`message`, `errcode`/`errmsg`) and pass them through `outbound.Clean` with the configured secrets. Success needs the explicit success field (Bark `code==200`, ServerChan `code==0`, DingTalk/WeCom `errcode==0`, Feishu `code==0` or legacy `StatusCode==0`, ntfy a message `id`); a missing field is a failure.
- They implement `notify.ConfigChecker`; `NotificationService` calls it with the merged plaintext config before create/update, so a bad config is rejected with `ErrInvalidConf` (HTTP 400) instead of failing at hot reload.
- Tests point official hosts at a local TLS server with `outbound.NewClient(outbound.Options{Divert: addr})`. The `qa` build reads the same divert from `PT_TOOLS_QA_NOTIFY_DIVERT`; release builds have no such switch.

## Adding an Adapter

1. Implement the full `Channel` interface under `adapter/<type>`.
2. Parse and validate only that adapter's decrypted `ConfigJSON` during `Init`; implement `ConfigChecker` when that validation needs no I/O.
3. Register a factory with `notify.RegisterChannel`.
4. Add the production side-effect import in `cmd/root.go` or `cmd/web.go`.
5. Test init failure, outbound success/error, health, inbound callback (if supported), and bounded `Close`.
6. Preserve the disclosure boundary: list DTOs redact config; only the authenticated detail editor may receive decrypted configuration. Never log it.

## Security and Reliability

- Persist adapter configuration encrypted; decrypt only in trusted wiring/service paths.
- Sanitize errors before storing/logging so tokens and signed URLs do not leak.
- Treat `Send` acceptance separately from end-user delivery when the transport is asynchronous.
- Router dedupe is an optimization, not durable idempotency; durable workflows need a DB key.
- Respect caller contexts. `Close` must not hang shutdown.
- Quiet-hour ranges support crossing midnight; use package helpers rather than custom comparisons.
