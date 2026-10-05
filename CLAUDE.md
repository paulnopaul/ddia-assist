# ddia-assist

A local study helper for *Designing Data-Intensive Applications*. Go, SQLite and MCP, run in Docker.

## Spec-driven

- `docs/spec/` is the source of truth. Read the relevant spec file before changing behaviour.
- A behaviour change updates the spec first, either in its own PR or in the same PR when the change is small.
- Requirement IDs (`ING-3`, `MCP-7`…) are never renumbered or reused. Cite them in PR descriptions and
  put them in test names, e.g. `TestDriver_DEP1_HasFTS5`.
- When a requirement is implemented, update the status table in `docs/spec/README.md` in the same PR.
- Diagrams are Mermaid, so GitHub renders them.

## Never commit book content

The repo is public. No EPUB files, extracted text, database files or figures. `.gitignore` covers the usual paths.

## Layout

- `cmd/ddia`: the binary. `serve` runs the web UI plus HTTP MCP at `/mcp`; `mcp` runs stdio MCP for Claude Desktop.
- `internal/store`: SQLite (WAL) and embedded migrations in `internal/store/migrations/NNNN_name.sql`.
- `internal/mcpserver`: the MCP server shared by both transports.
- `internal/web`: the tracker UI.

## Checks

```sh
gofmt -l .        # must print nothing
go vet ./...
go test -race ./...
docker build .
```
