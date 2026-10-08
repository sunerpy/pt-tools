# Contributing

This page covers building pt-tools from source, submitting a change and editing this site. The full development guide, in Chinese, is the [Development guide](../../development.md).

## Building from source

The repository pins Go `1.26.7`, Node.js `25.2.0` and pnpm `10.25.0`. After cloning, run the full local checks:

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools
make check
```

`make check` checks the toolchain versions, formatting, documentation links, lint, tests and the build, in that order. Common targets:

| Command                | What it does                                            |
| ---------------------- | ------------------------------------------------------- |
| `make fmt`             | Formats the Go and frontend code                        |
| `make lint`            | Go and frontend lint and type checks                    |
| `make test`            | Frontend build and tests, Go race tests                 |
| `make build`           | Builds the frontend and a binary for this platform      |
| `make build-extension` | Checks the site list and packages the browser extension |

## Submitting a change

- Commit messages and pull request titles follow Conventional Commits, for example `fix(webui): …` or `feat(site): …`.
- Before a merge, CI runs the same checks as `make check`.
- Read the `AGENTS.md` files at the repository root and in each directory before contributing; they record the constraints on configuration, push safety checks, notifications and other modules.

## Adding a site

A new site is a site definition in `site/v2/definitions/` plus its fixture tests; the browser extension's site list is updated to match, and `make check-sites` checks that the two agree. The steps are in the [Development guide](../../development.md).

Without programming experience, you can collect redacted pages with the browser extension and send them to the maintainers; see [Request a new site](../guide/request-new-site.md).

## Editing this site

The content of this site is the `docs/` folder of the repository: Chinese pages in `docs/`, English pages in `docs/en/`, at the same paths. When you change a page:

1. Update both languages. The development guide and the design documents, which are in Chinese only, are the exception.
2. Link between pages with relative paths such as `../faq.md`, so the links work both on GitHub and on this site.
3. On a pull request, the `Docs site` check syncs the pages into the site and builds it once; a dead link or a page missing in one language fails the check.

The writing conventions, the format of the home page's data and the screenshot procedure are in `docs/README.md`. The site updates itself after the merge.

## Design documents

These design documents are in Chinese only. They record interface contracts and future directions; they do not mean a feature is released.

- [ChatOps, MCP and agent architecture](../../design/chatops-mcp-agent.md)
- [MCP server design](../../design/phase4-mcp.md)
- [AI agent design](../../design/phase5-agent.md)
