# Block & error pages

Corvus shows its own page when it refuses a request or when your service can't be reached. Both can
be replaced with your own HTML under **Block & error pages**.

![Block & error pages](../img/pages.png){ .shot }

| Page | Shown when |
|---|---|
| **Block page** | A security rule, a ban or exploit blocking refuses a request |
| **Error page** | The host's service doesn't answer (connection refused, timeout) or sends an invalid response: `502`, `503`, `504`. Error pages your service sends itself are passed through unchanged |

The built-in pages are translated into the visitor's language automatically.

## Placeholders

These are replaced (HTML-escaped) when the page is served:

| Placeholder | Value |
|---|---|
| `{{STATUS}}` | Status code |
| `{{RID}}` | Request ID |
| `{{HOST}}` | Requested site |
| `{{IP}}` | Visitor IP: search for it in the [request logs](traffic.md#request-logs) when a visitor reports a problem |
| `{{TIME}}` | Time |

Pages can be up to 64 KB. Saving deploys immediately; **Reset to default** brings back the built-in
page.
