# Submission

## What I built

**Part one: a disable/enable control on the article page.** Editors can take an
article down and put it back up. Because the change is applied asynchronously by
a service we do not own, the page shows the editor where their request
stands: *requested*, *applied*, or *overdue*.

**Part two: making the Go service more production ready.** I did two things:
graceful shutdown and structured JSON logging. See
[Part two](#part-two-production-readiness) for why those two, and what I
would do next.

### How to run it

Nothing differs from the main README:

```bash
make up          # start everything, then open http://localhost:5183
make logs-api    # our service
make logs-consumer
```

The new migration runs automatically when the API starts. To watch the
asynchronous gap more clearly, slow the queue down:

```bash
make queue-delay DELAY=30
```

---

## Part one: what do we show the person who just clicked?

### The problem

The API can only ever say *"the queue accepted your request"*. The article's
`disabled` flag changes some seconds later, if at all, and nothing tells us when.
The queue also:

- delivers **at least once**, so a message can be applied twice.
- is **unordered**, so disable-then-enable can be applied as enable-then-disable.
- **dead-letters** messages it cannot apply after three attempts, without telling us.

So the UI has to handle three situations: "accepted but not yet applied",
"applied", and "accepted but never applied".

### Options I considered

| Option | What the editor sees | Impact of building it | Why I did / did not pick it |
| --- | --- | --- | --- |
| **Client-side pending + polling** | "Disabling…" in that tab until the flag matches | **Frontend only**, apart from the new RPC. No schema change. Every open article page re-reads the article every 1–2s while waiting, and a hard timeout has to be guessed in the browser. Smallest change. | My first idea, rejected. The pending state lives in one browser tab, so it is lost on refresh and invisible to other editors. It cannot tell *slow* from *failed*, so it ends in a vague timeout. And it cannot tell whether it was *your* request that landed or someone else's. |
| **Server-side request record** ✅ | "Disable requested 4s ago", then "Article disabled", or "Disable not applied" | **Every layer.** A new table and migration, an indexed join on every article read, a new `pending_change` field through proto, BFF and UI, and a row lock while a request is decided. Polling remains, but only while something is pending. A 2-minute threshold has to be kept in line with the queue settings. Moderate change. | **Chosen.** It fixes everything wrong with client-side polling: the state survives refreshes, every editor sees it, *slow* and *failed* are told apart, and out-of-order delivery becomes visible. And it fits the time budget, unlike push. |
| **Server-side record + push** (Postgres `LISTEN/NOTIFY` → gRPC stream → tRPC subscription) | Same, but the badge flips the instant the change lands | **Everything above, plus** a trigger on `articles`, a table the consumer writes, so it fires inside their updates. The API holds a dedicated `LISTEN` connection and one long-lived gRPC stream per watching page, and the BFF holds one SSE connection per browser. Reconnects, proxy timeouts and fan-out across API replicas all need handling. Largest change. | The best experience, but too much for the time budget. The chosen design is built so this only replaces the refresh mechanism. |

### The chosen design

**Why this one.** The core problem is that "accepted" and "applied" are two
separate facts, but only one of them was stored. Every weakness of client-side
polling comes from keeping the second fact in a browser tab. Storing it on the
server fixes all of them at once. It is also the smallest step that does so:
push needs the same record anyway, and only replaces how the page finds out.

**What it costs.** One more table, an extra join on every read, polling while
a change is pending, and a 2-minute threshold that has to be kept in line with
the queue settings. I judged those worth it for a takedown control, where
showing the wrong state matters more than a little extra load.

**How it works.** The
consumer owns the `articles.disabled` flag (what has been *applied*). We now
own a new table, `article_status_requests`, which records what editors *asked
for*:

```
article_status_requests
  id            UUID   -- also sent as the message's trace_id
  article_id    UUID   -- references articles(id)
  action        TEXT   -- 'disable' | 'enable'
  requested_at  TIMESTAMPTZ
```

When reading an article, the API compares the **latest** request with the
current flag:

| Latest request vs `disabled` | Request age | State returned | Editor sees |
| --- | --- | --- | --- |
| No request, or it matches | – | none | "Live" / "Disabled" |
| Does not match | under 2 minutes | `PENDING` | A "Disable requested" chip next to the badge, "requested 4s ago" on the article page, and the button is locked |
| Does not match | over 2 minutes | `OVERDUE` | A "Disable not applied" chip, "requested 3m ago but has not been applied, it may have failed", and a "Try disable again" button |

Details the comparison above does not cover:

- **Why 2 minutes.** The queue holds a message for 5s and then allows three
  attempts 30s apart, so a change that will ever apply has done so after about
  70s. The rest is margin. It is one constant, `overdueAfter`.
- **Out-of-order delivery.** If disable/enable land in the wrong order, the
  flag will not match the latest request, so the editor sees `OVERDUE` rather
  than a false "Live".
- **Tracing.** The request id is the message's `trace_id`, so one click can be
  followed through our logs and the consumer's.

### Preventing the race rather than only reporting it

Unordered delivery only hurts when opposite requests are in flight together,
so the API allows at most one per article. It locks the article row while
deciding (`SELECT … FOR UPDATE`), then:

| Situation | Result |
| --- | --- |
| Same action already pending | Returns the existing request. A double click or a second editor sends one message, not two |
| Opposite action pending | `FailedPrecondition`. The UI locks the button too, but the server is the real guard |
| Already in the requested state | `FailedPrecondition`, rather than publishing a no-op |
| Latest request `OVERDUE` | Allowed, so an editor is never stuck |

Messages carry the target state, never a toggle, so a duplicate delivery is
harmless.

### Write-then-publish ordering

Within one transaction the API locks the row, inserts the request, publishes the
message and then commits. If publishing fails, the transaction rolls back and
the editor is told "could not send, try again" (`Unavailable`).

**Known issue: a timed-out send may still be delivered.** I found this while
testing graceful shutdown. With the queue paused, the publish timed out after
5s, so the API rolled back and reported failure. Once the queue resumed, all
four of those "failed" messages were delivered and applied. A timeout means
*we don't know* whether the message was sent, not that it wasn't. So:

- the editor is told it failed, but the change applies anyway;
- the record was rolled back, so the page never shows it as pending, and the
  badge just flips later with no explanation.

**The fix I would make:** on a send error, keep the request instead of rolling
back, and tell the editor "we couldn't confirm the change was sent, it may
still apply". The existing design then handles it honestly: it shows as
pending, the badge flips if the change lands, and it becomes overdue with a
retry if it never does. It is a small change. I left it so I could finish and
document the Part two work.

### How the UI refreshes

The UI refreshes **only while something is pending** and only the queries that
matter: the article page and the list page re-fetch every 2 seconds while an
article shows `PENDING`, and stop as soon as nothing does. This is still
polling, but it is bounded and driven by server state, and it is the only part
that the push design above would replace.

The accepted request is written into the page cache straight away, so the
"requested" state appears without waiting for the next read. It is what the
server returned, and the applied badge only changes once the database says
so. When the editor's own request lands, the page says "Article disabled.
It is no longer shown to readers."

### Where the code is

| Layer | Change |
| --- | --- |
| Database | [`00003_create_article_status_requests_table.sql`](backend/internal/database/migrations/00003_create_article_status_requests_table.sql) |
| Proto | `RequestArticleStatusChange` RPC and `Article.pending_change` in [`backend/api/proto`](backend/api/proto). I named it *Request…* rather than *Set…* because it cannot promise to set anything. |
| Go rules | [`article.go`](backend/internal/articles/article.go): `changeState` and `planStatusChange` are pure functions, so all the rules above are table-tested in [`article_test.go`](backend/internal/articles/article_test.go) |
| Go persistence | [`repository.go`](backend/internal/articles/repository.go): the locked lock/check/insert/publish/commit transaction, and the latest request joined onto every read |
| Go API | [`service.go`](backend/internal/articles/service.go): validation, the queue message and gRPC error codes |
| BFF | `requestStatusChange` mutation in [`articles-router.ts`](frontend/src/server/trpc/routers/articles/articles-router.ts). I mapped `FailedPrecondition` to `PRECONDITION_FAILED`, which was missing from the error map. |
| React | [`status-control.tsx`](frontend/src/client/routes/-components/status-control.tsx), [`status-badge.tsx`](frontend/src/client/routes/-components/status-badge.tsx), [`status-change.ts`](frontend/src/client/lib/status-change.ts) |

### How I verified it

- **Unit tests:** table tests cover every state and decision (`go test ./...`).
- **API:** I called each case directly with `make api-call`, and each returned
  the right result or error code.
- **End to end:** I followed one `trace_id` from our API into the consumer's
  logs and saw the pending state clear once the change landed.
- **Overdue:** I back-dated a request in the database and confirmed it shows
  as overdue and can be retried.
- **Browser:** I clicked through disable, enable and the pending state in the
  app.

### Assumptions

- **Everyone using the web app is an editor.** There is no authentication
  anywhere in the stack, and the app already lists disabled articles with a
  badge, so it reads as an internal editorial tool rather than the public
  reader site. I did not build auth (see [What I would do next](#what-i-would-do-next)). Adding a *write* action
  to an unauthenticated app is the biggest risk I would flag before shipping.
- **We may add tables to our own database.** The consumer only writes
  `articles.disabled`. The new table is ours, is created by our migrations and
  is never touched by the consumer.
- **2 minutes is a reasonable "overdue" threshold** for this queue's
  configuration. In production I would tie it to the real queue settings and
  alert on overdue requests rather than only showing them.

---

## Part two: production readiness

The brief asked for three things done properly rather than twelve half done. I
prioritised by *"what would hurt most at 3am on call"*.

### 1. Graceful shutdown ✅

**Problem.** `server.Stop()` cut off in-flight calls on every deploy. A failing
`Serve` was ignored, so the process could stay up serving nothing. The
database was never closed.

**Why first.** It affects every deploy, not just failures. It also protects
the status-change flow, which holds a row lock and a queue publish in one
transaction.

**Change** ([`main.go`](backend/cmd/api/main.go),
[`shutdown.go`](backend/cmd/api/shutdown.go)):

```
SIGTERM → stop accepting → drain in-flight calls (≤ 8s) → force stop if needed → close DB → exit
```

- `GracefulStop()`, capped at 8s so a hung call cannot block a deploy.
- A failing `Serve` exits with code 1 so the orchestrator restarts it.
- Docker Compose gave the container only 1s before `SIGKILL`, and `air` 3s,
  so both killed the API mid-drain. I raised them to 10s and 9s. The
  platform's grace period must exceed the service's own timeout.

**Verified** with tests for an idle drain and a forced stop, and in Docker by
stopping the API mid-request. The call finished cleanly and the process exited
0, where before it was killed with code 137.

### 2. Structured logging ✅

**Problem.** Logs were free text from `fmt.Println`, with no levels or fields,
so you could not filter by error or follow a request. Read-path database
errors were not logged at all.

**Why second.** Request logging, alerting and the `trace_id` from Part 1 are
only useful if logs can be searched.

**Change** ([`logging.go`](backend/cmd/api/logging.go)):

- JSON via the standard library's `log/slog`, with every line tagged
  `service: "articles-api"`.
- Level from `LOG_LEVEL`, defaulting to `info`.
- Goose's logs are routed through it too, so there is one format.
- Read-path errors are now logged.
- Field names (`article_id`, `trace_id`, `action`) match the consumer's, so
  one search follows a change across both services.

```json
{"level":"INFO","msg":"article status change requested","service":"articles-api","trace_id":"ce9b549f-…","action":"disable"}
```

**Verified** with unit tests for levels and JSON shape, and in Docker, where
our `trace_id` matched the consumer's log line.

---

## What I would do next

In priority order:

| Next step | Why it matters |
| --- | --- |
| **Login and permissions:** only editors can disable articles, with a record of who did what | Today anyone who can reach the app can take articles down. This is the biggest risk in the submission |
| **Configuration:** read the database URL and queue settings from environment variables, and refuse to start if any are missing | The database password is hardcoded, so the service cannot be deployed anywhere real |
| **Database connection limits:** cap and recycle connections | Under load the service could use up all of Postgres's connections, which it shares with the consumer |
| **Change history in the UI:** "Disabled by Alice at 14:02" | Editors can see who changed what, using the table we already have |
