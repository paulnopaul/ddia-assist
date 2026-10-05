# How the spec works

1. **The spec leads.** Any behaviour change starts as a PR against `docs/spec/`. Code follows
   in a later PR, or in the same PR when the change is small.
2. **Every requirement has an ID**, like `ING-3` or `MCP-7`. IDs are never reused or
   renumbered. A dropped requirement is struck through and kept in place.
3. **Traceability.** Implementation PRs list the IDs they cover. Tests include the ID in their
   name, for example `TestUnitPlan_ING7_NeverCrossesChapter`, so `grep ING-7` finds the
   requirement, its code and its tests.
4. **Keywords.** MUST, SHOULD and MAY are used as in RFC 2119.
5. **Status** for each requirement is tracked in the table below and updated in the PR that
   implements it.

| Prefix | Spec | Implemented |
|---|---|---|
| ING | [01 Ingest](01-ingest.md) | – |
| DAT | [02 Data model](02-data-model.md) | – |
| MCP | [03 MCP interface](03-mcp.md) | – |
| SES | [04 Study session](04-study-session.md) | – |
| REV | [05 Review queue](05-review-queue.md) | – |
| UI  | [06 Web UI](06-web-ui.md) | – |
| DEP | [07 Deployment](07-deployment.md) | – |
