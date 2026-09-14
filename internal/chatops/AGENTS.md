# `internal/chatops/` — Inbound Command Pipeline

## Role

ChatOps turns inbound channel messages into audited, permission-checked application commands. Channel transport belongs to `internal/notify`; DB/service behavior belongs to `internal/app` and injected command services.

## Layout

```text
internal/chatops/
├── command_registry.go # CommandSpec registry and aliases
├── message_chain.go    # Binding/permission/session/rate/admin dispatch
├── bindcode.go         # One-time binding code handling
├── session.go          # In-memory multi-step state with TTL cleanup
├── ratelimit.go        # Per-command/user throttling
└── commands/
    ├── services.go     # Injected application/downloader service set
    ├── bind.go / unbind.go
    ├── tasks.go / torrents.go / status.go / sites.go / version.go
    ├── pause.go / resume.go / delete.go
    └── addrss.go / delrss.go / help.go
```

## Mandatory Message Order

`MessageChain.Process` owns this sequence:

```text
binding lookup
  → unbound-user bind exception / allowed check
  → command parsing
  → pending multi-step session
  → registry lookup
  → rate limit
  → admin-only check
  → handler
  → audit record + channel reply
```

Business denials are normally replies plus audit records, not Go errors. Returned errors are for unrecoverable lookup/dependency failures. Do not bypass this chain for real inbound messages.

## Adding a Command

```go
func init() {
    chatops.RegisterCommand(chatops.CommandSpec{
        Name:        "example",
        Description: "...",
        Aliases:     []string{"ex"},
        AdminOnly:   true,
        RateLimit:   &chatops.RateLimitSpec{/* ... */},
        Handler:     exampleHandler,
    })
}
```

Guidelines:

- Normalize user-visible errors through existing translation/reply helpers.
- Obtain dependencies from `commands.getServices()`; production injects them once from `cmd/web.go`.
- High-risk mutations require explicit confirmation and must remain admin-gated where already specified.
- Use `SessionStore` for multi-step commands. Keys include channel type, config ID, and channel user ID; default TTL is five minutes.
- Callback payloads must be compact, deterministic, and validated again before mutation.
- Never embed or echo bot tokens, cookies, downloader passwords, or decrypted config.

## Registry and Globals

The default registry panics on duplicate commands/aliases by design. Tests adding global entries must clean them up or use an isolated registry. The injected `commands.Services` set is also process-global; restore it in `t.Cleanup`.

## Tests

Cover:

- Bound/unbound, disabled, admin/non-admin, rate-limited, and unknown-command paths.
- Audit result and reply behavior for business denials.
- Session expiry and isolation between channel/config/user tuples.
- Confirmation gates for pause/delete/RSS mutations.
- Both Chinese and English reply paths when a command localizes output.
