# `internal/notify/` — Notification Channels and Delivery

## Role

This package provides transport-neutral notifications, channel factories, fan-out, dedupe, quiet hours, digesting, and durable retry. Supported production adapters are Telegram, QQ/OneBot, generic webhook, and WeCom webhook.

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
    └── wecom/                   # WeCom webhook
```

## Delivery Paths

- `Router.Route` loads enabled configurations, applies an optional allow-list and 60-second dedupe, initializes/reuses channel instances, and fans out with a bounded timeout.
- Immediate app delivery uses live channel instances. Failures are persisted to `NotificationOutbox` when an outbox is available.
- `OutboxWorker` retries due rows at bounded backoff and marks exhausted rows dead.
- RSS delivery adds a separate idempotency row in `RSSNotificationLog`, applies quota/quiet-hour policy, and may use `DigestBuffer` before sending.

## Stateful Adapter Warning

Do not create a second live Telegram or QQ listener for outbox delivery: it can collide on polling, ports, or connection ownership. Reuse the production live-channel manager for stateful transports. The current outbox factory path is reliable for stateless webhook adapters; changes here require explicit lifecycle tests.

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
