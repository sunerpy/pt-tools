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
    ├── wecom/                   # WeCom webhook
    ├── bark/ serverchan/ ntfy/  # Outbound-only push services
    ├── dingtalk/ feishu/        # Outbound-only group bots (optional signing)
    └── outbound/                # Shared JSON POST helper; errors never include the request URL
```

## Delivery Paths

- `Router.Route` loads enabled configurations, applies an optional allow-list and 60-second dedupe, initializes/reuses channel instances, and fans out with a bounded timeout.
- Immediate app delivery uses live channel instances. Failures are persisted to `NotificationOutbox` when an outbox is available.
- `OutboxWorker` retries due rows at bounded backoff and marks exhausted rows dead.
- RSS delivery adds a separate idempotency row in `RSSNotificationLog`, applies quota/quiet-hour policy, and may use `DigestBuffer` before sending.

## Stateful Adapter Warning

Do not create a second live Telegram or QQ listener for outbox delivery: it can collide on polling, ports, or connection ownership. Outbox retries go through the production live-channel manager (`OutboxWorker.SetLiveSender`); the factory path is only a fallback for stateless webhook adapters and decrypts `ConfigJSON` first. Changes here require explicit lifecycle tests.

Telegram `Healthy()` follows the latest `getUpdates` result (`pollHealthTransport`): telego retries a failing long poll internally without closing the updates channel, so transport-level results are the only signal. `runInbound` restarts the long poll with backoff if the updates channel closes while the poll context is alive.

## Adding an Adapter

1. Implement the full `Channel` interface under `adapter/<type>`.
2. Parse and validate only that adapter's decrypted `ConfigJSON` during `Init`.
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
