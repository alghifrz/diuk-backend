# diuk-backend

HTTP API for **DIUK Solution**, a multi-tenant AI automation platform for service-based businesses.
This repository currently contains the backend foundation plus the core multi-tenant schema:
configuration, logging, middleware, API versioning, graceful shutdown, the PostgreSQL layer and
the `businesses`, `users`, `roles` and `business_users` tables, plus JWT authentication,
tenant context, business profile/F&B settings, area/table seating layout, and
the menu catalog with private image storage, the customer/CRM foundation,
the provider-agnostic conversation/message domain, the reservation
and availability engine, the AI prompt/tool-calling engine, the
WhatsApp Cloud API channel layer, and a tenant-scoped knowledge base
with PostgreSQL/pgvector RAG. Instagram and other providers are
wired later through the same channel abstraction.

## Tech stack

- Go 1.26 with [Gin](https://github.com/gin-gonic/gin)
- Structured logging via the standard library `log/slog`
- Supabase PostgreSQL accessed with [pgx/v5](https://github.com/jackc/pgx) and `pgxpool` (no ORM)
- SQL migrations with [golang-migrate](https://github.com/golang-migrate/migrate)
- Redis (configuration and local container only at this stage)
- Docker / Docker Compose, deployed to IDCloudHost

## Requirements

- Go 1.26+
- Docker and Docker Compose (optional, for the containerized workflow)

## Run with Go

```bash
cp .env.example .env   # then adjust the values
go mod download
go run ./cmd/migrate up
go run ./cmd/api
```

The service listens on `http://localhost:8080` by default and refuses to start when PostgreSQL
is unreachable.

## Database migrations

Schema changes live in `migrations/` as sequential SQL files and are never applied automatically
by the API server.

```bash
go run ./cmd/migrate up          # apply all pending migrations
go run ./cmd/migrate up 1        # apply the next migration only
go run ./cmd/migrate down        # roll back the last migration
go run ./cmd/migrate down 2      # roll back the last two migrations
go run ./cmd/migrate version     # print the current schema version
go run ./cmd/migrate force 1     # clear a dirty state after a failed run
```

The container image ships the same command: `docker run --entrypoint /app/migrate <image> up`.

## Development seed

`cmd/seed` creates the Kasiko Coffee development tenant so JWT + tenant context can be tested
against a real Supabase Auth user. It never creates Auth users, passwords, or tokens.

```bash
# SEED_AUTH_USER_ID = existing auth.users.id
# SEED_USER_EMAIL   = that Auth user's email
go run ./cmd/seed
```

The command is idempotent: a second run reuses the same `kasiko-coffee` business, local user,
and OWNER membership. It fails instead of overwriting a conflicting business or membership.
All writes run in one transaction.

`migrations/000001_create_core_tables.sql` creates the multi-tenant core: `businesses` (tenants),
`users` (profile data for Supabase Auth users, never credentials), `roles` (seeded with `OWNER`,
`MANAGER`, `STAFF`) and `business_users`, which gives a user one role per business. Foreign keys
use `ON DELETE RESTRICT` so a tenant hierarchy cannot be deleted by accident.

## Run with Docker Compose

Compose starts the backend plus a Redis container. `DATABASE_URL` must be set in a local `.env`
file (Supabase connection string); Redis defaults to the Compose service at `redis://redis:6379`.

```bash
cp .env.example .env
docker compose up --build
```

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `APP_ENV` | no | `development` | `development` or `production`; controls log format and Gin mode |
| `APP_PORT` | no | `8080` | Port the HTTP server binds to |
| `APP_NAME` | no | `diuk-backend` | Service name reported by the API |
| `DATABASE_URL` | yes | — | Supabase PostgreSQL connection string |
| `REDIS_URL` | yes | — | Redis connection string |
| `SUPABASE_URL` | yes | — | Supabase project URL; used to derive the JWKS endpoint and JWT issuer |
| `SUPABASE_SERVICE_ROLE_KEY` | images | — | Backend-only service role key for private Storage uploads and signed URLs |
| `SUPABASE_STORAGE_MENU_BUCKET` | no | `diuk-menu-images` | Private Storage bucket for menu item images |
| `CORS_ALLOWED_ORIGINS` | production | `http://localhost:3000` | Comma-separated origins. Required in production; `*` is rejected when `APP_ENV=production` |
| `HTTP_READ_HEADER_TIMEOUT_SECONDS` | no | `10` | HTTP header read timeout |
| `HTTP_READ_TIMEOUT_SECONDS` | no | `30` | HTTP request read timeout |
| `HTTP_WRITE_TIMEOUT_SECONDS` | no | `180` | HTTP response write timeout |
| `HTTP_IDLE_TIMEOUT_SECONDS` | no | `60` | Keep-alive idle timeout |
| `HTTP_MAX_HEADER_BYTES` | no | `1048576` | Maximum request header size |
| `HTTP_MAX_JSON_BYTES` | no | `1048576` | Maximum JSON request body (multipart uploads use their own caps) |
| `HTTP_SHUTDOWN_TIMEOUT_SECONDS` | no | `15` | Graceful HTTP shutdown timeout |
| `HTTP_TRUSTED_PROXIES` | no | — | Comma-separated CIDRs trusted for `X-Forwarded-For`. Empty = do not trust proxies |
| `WEBHOOK_MAX_BODY_BYTES` | no | `1048576` | WhatsApp webhook payload cap |
| `DB_MAX_CONNS` | no | `10` | PostgreSQL pool max connections (keep small behind a pooler) |
| `DB_MIN_CONNS` | no | `2` | PostgreSQL pool min connections |
| `DB_MAX_CONN_LIFETIME_SECONDS` | no | `3600` | Recycle connections after this lifetime |
| `DB_MAX_CONN_IDLE_TIME_SECONDS` | no | `1800` | Close idle connections after this time |
| `DB_HEALTH_CHECK_PERIOD_SECONDS` | no | `60` | Pool health-check interval |
| `RATE_LIMIT_ENABLED` | no | `true` | In-process rate limits. Fail-open; Redis is not required |
| `RATE_LIMIT_WEBHOOK_PER_MIN` | no | `180` | Public WhatsApp webhook requests per IP per minute |
| `RATE_LIMIT_AI_PER_MIN` | no | `20` | AI generate requests per business per minute |
| `RATE_LIMIT_UPLOAD_PER_MIN` | no | `10` | Knowledge/menu upload and reprocess per business per minute |
| `RATE_LIMIT_CAMPAIGN_PER_MIN` | no | `10` | Campaign start/schedule per business per minute |
| `TEST_DATABASE_URL` | no | — | Migrated database used by the repository tests; they skip when it is unset |
| `TEST_STORAGE_ENABLED` | no | — | Set `true` to run live Supabase Storage tests |
| `AI_PROVIDER` | no | `openai` | Stored default provider (`openai`, `groq`, `anthropic`, `ollama`) |
| `AI_API_KEY` | generate | — | Provider API key; never stored in PostgreSQL or returned by the API |
| `AI_MODEL` | no | `gpt-4o-mini` | Default model written into new `ai_settings` rows |
| `AI_BASE_URL` | no | `https://api.openai.com/v1` | OpenAI-compatible Chat Completions base URL (Groq/Ollama use this) |
| `AI_TIMEOUT_SECONDS` | no | `30` | Per-request LLM timeout |
| `TEST_AI_LIVE` | no | `false` | Set `true` to run a live provider test; rejected when `APP_ENV=production` |
| `META_WHATSAPP_ACCESS_TOKEN` | send | — | WhatsApp Cloud API access token; never stored in PostgreSQL |
| `META_WHATSAPP_APP_SECRET` | webhook | — | App secret used to verify `X-Hub-Signature-256` |
| `META_WHATSAPP_VERIFY_TOKEN` | webhook | — | Token Meta sends on `GET /api/webhooks/whatsapp` |
| `META_WHATSAPP_API_VERSION` | no | `v21.0` | Graph API version |
| `META_WHATSAPP_BASE_URL` | no | `https://graph.facebook.com` | Graph API host |
| `META_WHATSAPP_TIMEOUT_SECONDS` | no | `10` | WhatsApp HTTP timeout |
| `TEST_WHATSAPP_LIVE` | no | `false` | Set `true` plus token/phone/to env to run a live send test; rejected when `APP_ENV=production` |
| `APP_MODE` | no | `all` | `api`, `worker`, or `all` |
| `REDIS_PASSWORD` | no | — | Optional Redis AUTH password; never logged |
| `REDIS_DB` | no | `0` | Redis logical database |
| `REDIS_TIMEOUT_SECONDS` | no | `5` | Redis dial/read/write timeout |
| `REDIS_QUEUE_PREFIX` | no | `diuk` | Stream prefix (`diuk:stream:ai`) |
| `WORKER_AI_CONCURRENCY` | no | `2` | Bounded AI worker goroutines |
| `WORKER_DELIVERY_CONCURRENCY` | no | `2` | Bounded delivery worker goroutines |
| `WORKER_KNOWLEDGE_CONCURRENCY` | no | `1` | Bounded knowledge ingest workers |
| `AUTOMATION_ENABLED` | no | `true` | Evaluate events and run the scheduler; CRUD still works when false |
| `AUTOMATION_SCHEDULER_INTERVAL` | no | `30` | Scheduler tick seconds |
| `AUTOMATION_MAX_RUNS_PER_TICK` | no | `50` | Max new scheduled runs per tick |
| `AUTOMATION_DEFAULT_INACTIVITY_MINUTES` | no | `120` | Fallback inactivity window |
| `WORKER_AUTOMATION_CONCURRENCY` | no | `2` | Bounded automation workers |
| `CAMPAIGN_BATCH_SIZE` | no | `50` | Recipients claimed per dispatch tick |
| `CAMPAIGN_DISPATCH_INTERVAL_MS` | no | `200` | Delay before the next campaign dispatch job |
| `CAMPAIGN_SCHEDULER_INTERVAL` | no | `30` | Campaign scheduler tick seconds |
| `CAMPAIGN_MAX_STARTS_PER_TICK` | no | `20` | Max due campaigns claimed per tick |
| `CAMPAIGN_MAX_CONCURRENCY` | no | `2` | Bounded campaign workers |
| `CAMPAIGN_WORKER_TIMEOUT_SECONDS` | no | `60` | Per campaign job timeout |
| `AI_EMBEDDING_ENABLED` | no | `false` | Enable embeddings/RAG; conversations still work when false |
| `AI_EMBEDDING_PROVIDER` | no | `openai` | Embedding provider name (OpenAI-compatible HTTP) |
| `AI_EMBEDDING_MODEL` | no | `text-embedding-3-small` | Embedding model |
| `AI_EMBEDDING_DIMENSIONS` | no | `1536` | Must match `knowledge_chunks.embedding` (`vector(1536)`) |
| `AI_EMBEDDING_BATCH_SIZE` | no | `64` | Chunks per embedding request |
| `AI_EMBEDDING_API_KEY` | ingest | — | Embedding key; falls back to `AI_API_KEY`; never stored in PostgreSQL |
| `KNOWLEDGE_CHUNK_SIZE` | no | `800` | Deterministic chunk size in runes |
| `KNOWLEDGE_CHUNK_OVERLAP` | no | `120` | Chunk overlap in runes |
| `KNOWLEDGE_TOP_K` | no | `5` | Retrieval limit |
| `KNOWLEDGE_MIN_SIMILARITY` | no | `0.35` | Cosine similarity threshold |
| `KNOWLEDGE_MAX_FILE_SIZE_MB` | no | `10` | Upload cap |
| `AI_WORKER_TIMEOUT_SECONDS` | no | `90` | Per AI job timeout |
| `DELIVERY_WORKER_TIMEOUT_SECONDS` | no | `20` | Per delivery job timeout |
| `JOB_MAX_ATTEMPTS` | no | `3` | Bounded retries before DLQ |
| `JOB_DLQ_RETENTION_DAYS` | no | `7` | Dead-letter TTL |
| `TEST_REDIS_URL` | no | — | When set, Redis stream integration tests run |
| `SEED_AUTH_USER_ID` | seed only | — | Existing Supabase Auth user UUID (`auth.users.id`) |
| `SEED_USER_EMAIL` | seed only | — | Email of that Auth user; used only for the local `users` profile |

Startup fails with an explicit error when a required variable is missing. Never commit `.env`.

## Endpoints

```
GET /                  -> {"service":"diuk-backend","message":"DIUK backend is running"}
GET /api/v1/health     -> {"status":"ok","service":"diuk-backend"}
GET /api/v1/health/ready -> liveness-style readiness: requires PostgreSQL
GET /api/v1/health/db  -> {"status":"ok","service":"diuk-backend","database":"ok"}
GET /api/v1/health/redis -> {"status":"ok","service":"diuk-backend","redis":"ok"}
GET /metrics           -> Prometheus aggregate telemetry (network-protect in production)
GET /api/v1/me              -> authenticated tenant probe (see Authentication)
GET /api/v1/me/permissions  -> permission codes for the current tenant role
GET /api/v1/business            -> current business profile (business.read)
PATCH /api/v1/business          -> update profile fields (business.update)
GET /api/v1/business/fnb-settings   -> F&B operational settings (business.read)
PATCH /api/v1/business/fnb-settings -> update F&B settings (business.update)
GET    /api/v1/areas                      -> list tenant areas (area.read)
POST   /api/v1/areas                      -> create area (area.create)
GET    /api/v1/areas/:id                  -> get area (area.read)
PATCH  /api/v1/areas/:id                  -> update area (area.update)
DELETE /api/v1/areas/:id                  -> deactivate area (area.delete)
GET    /api/v1/areas/:id/tables           -> list tables in an area (area.read)
POST   /api/v1/areas/:id/tables           -> create table (area.create)
GET    /api/v1/areas/:id/tables/:table_id -> get table (area.read)
PATCH  /api/v1/areas/:id/tables/:table_id -> update table (area.update)
DELETE /api/v1/areas/:id/tables/:table_id -> deactivate table (area.delete)
GET    /api/v1/menu/categories                -> list menu categories (menu.read)
POST   /api/v1/menu/categories                -> create category (menu.create)
GET    /api/v1/menu/categories/:id            -> get category (menu.read)
PATCH  /api/v1/menu/categories/:id            -> update category (menu.update)
DELETE /api/v1/menu/categories/:id            -> deactivate category (menu.delete)
GET    /api/v1/menu/categories/:id/items      -> list items in a category (menu.read)
GET    /api/v1/menu/items                     -> list menu items (menu.read)
POST   /api/v1/menu/items                     -> create item, optional image (menu.create)
GET    /api/v1/menu/items/:id                 -> get item (menu.read)
PATCH  /api/v1/menu/items/:id                 -> update item or image (menu.update)
DELETE /api/v1/menu/items/:id                 -> deactivate item (menu.delete)
GET    /api/v1/customers                      -> list customers (customer.read)
POST   /api/v1/customers                      -> create customer (customer.create)
GET    /api/v1/customers/:id                  -> get customer + identities + tags (customer.read)
PATCH  /api/v1/customers/:id                  -> update customer (customer.update)
DELETE /api/v1/customers/:id                  -> deactivate customer (customer.delete)
GET    /api/v1/customers/:id/identities       -> list identities (customer.read)
POST   /api/v1/customers/:id/identities       -> create identity (customer.create)
PATCH  /api/v1/customers/:id/identities/:identity_id -> update identity (customer.update)
DELETE /api/v1/customers/:id/identities/:identity_id -> deactivate identity (customer.delete)
GET    /api/v1/customers/:id/tags             -> list assigned tags (customer.read)
POST   /api/v1/customers/:id/tags/:tag_id     -> assign tag (customer.create)
DELETE /api/v1/customers/:id/tags/:tag_id     -> unassign tag (customer.delete)
GET    /api/v1/tags                           -> list CRM tags (customer.read)
POST   /api/v1/tags                           -> create tag (customer.create)
GET    /api/v1/tags/:id                       -> get tag (customer.read)
PATCH  /api/v1/tags/:id                       -> update tag (customer.update)
DELETE /api/v1/tags/:id                       -> deactivate tag (customer.delete)
GET    /api/v1/conversations                  -> list inbox threads (conversation.read)
POST   /api/v1/conversations                  -> create or reuse an open thread (conversation.update)
GET    /api/v1/conversations/:id              -> get conversation + summaries (conversation.read)
PATCH  /api/v1/conversations/:id              -> assign/unassign or open/close (conversation.update)
PATCH  /api/v1/conversations/:id/read         -> set unread_count to 0 (conversation.update)
GET    /api/v1/conversations/:id/messages     -> list messages oldest-first (conversation.read)
POST   /api/v1/conversations/:id/messages     -> append a message (conversation.update)
GET    /api/v1/business/hours                 -> weekly operating hours (business.read)
PUT    /api/v1/business/hours                 -> replace weekly hours (business.update)
PATCH  /api/v1/business/hours                 -> replace weekly hours (business.update)
GET    /api/v1/business/schedule-exceptions   -> list date exceptions (business.read)
POST   /api/v1/business/schedule-exceptions   -> create exception (business.update)
PATCH  /api/v1/business/schedule-exceptions/:id -> update exception (business.update)
DELETE /api/v1/business/schedule-exceptions/:id -> delete exception (business.update)
GET    /api/v1/reservations                   -> list reservations (reservation.read)
GET    /api/v1/reservations/availability      -> available tables for a slot (reservation.read)
POST   /api/v1/reservations                   -> create PENDING reservation (reservation.create)
GET    /api/v1/reservations/:id               -> reservation detail (reservation.read)
PATCH  /api/v1/reservations/:id               -> update booking fields (reservation.update)
POST   /api/v1/reservations/:id/confirm       -> PENDING → CONFIRMED (reservation.confirm)
POST   /api/v1/reservations/:id/cancel        -> cancel (reservation.cancel)
POST   /api/v1/reservations/:id/complete      -> CONFIRMED → COMPLETED (reservation.update)
PATCH  /api/v1/reservations/:id/reschedule    -> move PENDING/CONFIRMED booking (reservation.update)
GET    /api/v1/calendar/events                -> reservation + block events (calendar.read)
GET    /api/v1/calendar/day                   -> one local day (calendar.read)
POST   /api/v1/calendar/blocks                -> create ACTIVE block (calendar.update)
PATCH  /api/v1/calendar/blocks/:id            -> update ACTIVE block (calendar.update)
DELETE /api/v1/calendar/blocks/:id            -> cancel block (calendar.update)
GET    /api/v1/reservations/:id/payment       -> reservation payment (payment.read)
POST   /api/v1/reservations/:id/payment       -> create missing payment (payment.update)
GET    /api/v1/reservation-payments           -> list payments (payment.read)
PATCH  /api/v1/reservation-payments/:id       -> update payment notes (payment.update)
POST   /api/v1/reservation-payments/:id/confirm -> UNPAID → PAID (payment.update)
POST   /api/v1/reservation-payments/:id/refund  -> PAID → REFUNDED (payment.update)
GET    /api/v1/ai/settings                    -> tenant AI settings (prompt.read)
PATCH  /api/v1/ai/settings                    -> update provider/model/limits (prompt.update)
GET    /api/v1/ai/prompts                     -> list prompt versions (prompt.read)
POST   /api/v1/ai/prompts                     -> create DRAFT prompt (prompt.create)
GET    /api/v1/ai/prompts/:id                 -> prompt detail (prompt.read)
PATCH  /api/v1/ai/prompts/:id                 -> edit DRAFT prompt (prompt.update)
POST   /api/v1/ai/prompts/:id/activate        -> make this version ACTIVE (prompt.update)
DELETE /api/v1/ai/prompts/:id                 -> archive prompt (prompt.delete)
POST   /api/v1/ai/generate                    -> generate an AI reply (conversation.update)
GET    /api/webhooks/whatsapp                 -> Meta webhook verification (public)
POST   /api/webhooks/whatsapp                 -> Meta webhook events (public, HMAC)
GET    /api/v1/channels                       -> list channel configs (channel.read)
POST   /api/v1/channels                       -> create channel config (channel.create)
GET    /api/v1/channels/:id                   -> channel detail (channel.read)
PATCH  /api/v1/channels/:id                   -> update channel config (channel.update)
DELETE /api/v1/channels/:id                   -> deactivate channel (channel.delete)
GET    /api/v1/knowledge/documents            -> list knowledge documents (knowledge.read)
POST   /api/v1/knowledge/documents            -> create document + ingest job (knowledge.create)
GET    /api/v1/knowledge/documents/:id        -> document metadata (knowledge.read)
PATCH  /api/v1/knowledge/documents/:id        -> title/description only (knowledge.update)
DELETE /api/v1/knowledge/documents/:id        -> delete document and chunks (knowledge.delete)
POST   /api/v1/knowledge/documents/:id/reprocess -> rebuild chunks (knowledge.update)
GET    /api/v1/knowledge/status               -> retrieval enabled flag + upload limits (knowledge.read)
POST   /api/v1/knowledge/ask                  -> retrieve + LLM answer grounded in the excerpts, with sources (knowledge.read)
POST   /api/v1/knowledge/search               -> run the AI's retrieval for a query, rate-limited like AI (knowledge.read)
GET    /api/v1/automations                    -> list automations (automation.read)
POST   /api/v1/automations                    -> create automation (automation.create)
GET    /api/v1/automations/:id                -> automation detail (automation.read)
PATCH  /api/v1/automations/:id                -> update definition (automation.update)
DELETE /api/v1/automations/:id                -> archive automation (automation.delete)
POST   /api/v1/automations/:id/activate       -> set ACTIVE (automation.update)
POST   /api/v1/automations/:id/deactivate     -> set INACTIVE (automation.update)
GET    /api/v1/automations/:id/runs           -> safe run history (automation.read)
GET    /api/v1/analytics/overview             -> high-level business metrics (analytics.read)
GET    /api/v1/analytics/conversations        -> conversation metrics (analytics.read)
GET    /api/v1/analytics/customers            -> customer metrics (analytics.read)
GET    /api/v1/analytics/reservations         -> reservation metrics (analytics.read)
GET    /api/v1/analytics/conversion           -> conversation/reservation/payment funnel (analytics.read)
GET    /api/v1/analytics/revenue              -> operational payment/deposit totals (analytics.read)
GET    /api/v1/analytics/ai                   -> AI generation metrics (analytics.read)
GET    /api/v1/analytics/handover             -> human handover metrics (analytics.read)
GET    /api/v1/analytics/automation           -> automation run metrics (analytics.read)
GET    /api/v1/analytics/messaging            -> messaging/delivery metrics (analytics.read)
GET    /api/v1/analytics/campaigns            -> campaign run/recipient metrics (analytics.read)
GET    /api/v1/campaigns                      -> list campaigns (campaign.read)
POST   /api/v1/campaigns                      -> create draft campaign (campaign.create)
GET    /api/v1/campaigns/:id                  -> campaign detail (campaign.read)
PATCH  /api/v1/campaigns/:id                  -> update draft campaign (campaign.update)
DELETE /api/v1/campaigns/:id                  -> cancel/archive campaign (campaign.delete)
POST   /api/v1/campaigns/audience/preview     -> estimate audience without a run (campaign.read)
POST   /api/v1/campaigns/:id/schedule         -> DRAFT/SCHEDULED → SCHEDULED (campaign.update)
POST   /api/v1/campaigns/:id/start            -> start immediately (campaign.update)
POST   /api/v1/campaigns/:id/pause            -> pause pending dispatch (campaign.update)
POST   /api/v1/campaigns/:id/resume           -> resume a paused campaign (campaign.update)
POST   /api/v1/campaigns/:id/cancel           -> cancel and skip pending recipients (campaign.delete or campaign.update)
GET    /api/v1/campaigns/:id/runs             -> campaign runs + persisted counts (campaign.read)
GET    /api/v1/campaigns/:id/runs/:run_id     -> one run (campaign.read)
GET    /api/v1/campaigns/:id/runs/:run_id/recipients -> paginated recipients (campaign.read)
```

`/api/v1/health` confirms only that the HTTP application is alive; it requires no authentication
and does not touch PostgreSQL, Redis, or AI providers. `/api/v1/health/ready` requires
PostgreSQL and reports Redis as a dependency without failing solely because Redis is down.
`/api/v1/health/db` pings PostgreSQL and answers `503` with `"database":"unavailable"` when
the ping fails, without revealing the underlying error.

## Authentication

Login lives in Supabase Auth. The backend never accepts passwords and never stores tokens.

```
Authorization: Bearer <supabase access token>
X-Business-ID: <business UUID>
```

Request flow:

1. The frontend signs the user in with Supabase Auth and receives a JWT.
2. The backend fetches and caches `{SUPABASE_URL}/auth/v1/.well-known/jwks.json`.
3. The JWT signature, expiration, issuer (`{SUPABASE_URL}/auth/v1`) and algorithm are verified.
4. The `sub` claim is treated as `users.auth_user_id` and looked up in PostgreSQL.
5. `X-Business-ID` is parsed and checked against an **active** `business_users` row for that user
   in an **active** business. The role comes from the `roles` table, not from the client.
6. The resulting user, business and role are stored as request context.

`GET /api/v1/me` is a development probe that requires both steps. It returns the current user,
business and role. Health endpoints stay public.

Missing or invalid JWTs return `401`. A missing or malformed `X-Business-ID` returns `400`.
A valid user who is not an active member of the requested business receives `403`.

## Authorization

Business capabilities come from the database, not from the JWT or the client:

```
role (from business_users)
  → role_permissions
  → permissions.code
```

Future handlers attach `authz.RequirePermission(authz.Permission...)` after
`Authenticate` and `RequireTenant`. There is no OWNER bypass in Go.

`GET /api/v1/me/permissions` returns the sorted permission codes for the current
`X-Business-ID` membership. Missing permissions produce a generic `403 {"error":{"code":"FORBIDDEN","message":"forbidden"}}`.

## Business profile and F&B settings

`GET`/`PATCH /api/v1/business` read and update the current tenant's identity (name, contact,
timezone, URLs). Slug, business type and status are not editable here; renaming does not
change the slug.

`GET`/`PATCH /api/v1/business/fnb-settings` manage reservation configuration (currency,
slot length, party size, advance windows, and optional reservation deposit). Both routes use the Job 03 tenant context and
Job 04 permissions. The business id is never taken from the request path or body.

## Areas and tables

Area and table routes describe the tenant's seating layout. `DELETE` is a soft
deactivate (`status = INACTIVE`); rows are never physically removed. An area can
be deactivated only when it has no active tables. Table names are unique inside
an area; area names are unique inside a business. Tenant identity always comes
from `X-Business-ID` after Job 03 resolution. Reservation occupancy is not
calculated here.

## Menu categories and items

Menu routes describe the tenant catalog. `DELETE` is a soft deactivate
(`status = INACTIVE`). A category can be deactivated only when it has no
active items. Category names are unique inside a business; item names may
repeat across categories.

Create and update item endpoints use `multipart/form-data`. Images are
optional. Allowed types are JPEG, PNG and WebP, max 5 MB. The backend stores
only a tenant-scoped object path:

```text
{business_id}/menu/{item_id}/original.{jpg|png|webp}
```

`image_url` is a 15-minute signed URL generated at response time. It is never
written to PostgreSQL.

### Menu image bucket

Create this bucket in the Supabase dashboard. The API does not create it.

```text
Bucket:      diuk-menu-images
Visibility:  Private
```

The backend uses `SUPABASE_SERVICE_ROLE_KEY` for upload, replace, delete, and
signed URL generation. Do not put that key in Next.js or return it from the API.

## Customers and CRM

Customer routes store a tenant-scoped profile plus channel identities
(`WHATSAPP`, `INSTAGRAM`, `WEBSITE`) and CRM tags. `DELETE` on customers,
identities, and tags is a soft deactivate (`status = INACTIVE`). Removing a
customer-tag assignment is a physical relationship delete.

Tenant scope always comes from `X-Business-ID`. Composite foreign keys keep an
identity or tag assignment from pointing at another business's customer. Phone
numbers are not unique. Duplicate channel identities inside one business return
`409`. List customers supports `?search=`, `?status=`, `?limit=` (max 100) and
`?offset=`.

OWNER and MANAGER have full `customer.*` permissions. STAFF can read, create,
and update, but cannot delete.

## Conversations and messages

Conversation routes store a provider-agnostic inbox thread plus append-only
messages. One generic `conversations` / `messages` pair covers WhatsApp,
Instagram, and website chat. Provider IDs stay in `external_id` / `metadata`;
there are no Meta-specific columns or adapter tables.

A customer may have many conversations over time, but only one `OPEN` thread
per business + customer + channel + identity (or without identity for
website/manual threads). Creating the same open thread again returns `200`
and the existing row. A new thread returns `201`.

`POST /conversations/:id/messages` is transactional: insert the message,
set `last_message_at`, increment `unread_count` only for inbound customer
messages, and reopen a `CLOSED` conversation. Duplicate `external_id` values
inside the same tenant conversation are idempotent (`200` + existing
message). `GET` never marks a thread read; use `PATCH .../read`.

Assignment requires an active user with an active membership in the current
business. Tenant scope always comes from `X-Business-ID`. Composite foreign
keys keep a conversation from pointing at another tenant's customer or
identity, and a message from pointing at another tenant's conversation.

List conversations supports `?status=` (default `OPEN`), `?channel=`,
`?assigned_user_id=`, `?customer_id=`, `?search=` (customer name/phone/email
only), `?unread=`, `?limit=` (max 100) and `?offset=`. Inbox order is
`last_message_at DESC NULLS LAST, id DESC`.

OWNER, MANAGER, and STAFF all have `conversation.read` and
`conversation.update`. There is no `conversation.create` or
`conversation.delete` permission. Sending a message is an update.

## Reservations and availability

Reservation routes store tenant-scoped bookings against `customers` and
optional `tables`. Status is `PENDING` → `CONFIRMED` → `COMPLETED`, or
`PENDING`/`CONFIRMED` → `CANCELLED`. There is no delete endpoint or
`reservation.delete` permission. `PATCH` cannot change status; use the
explicit confirm/cancel/complete actions.

A confirmed reservation always has a table. If none was supplied, confirm
picks the smallest active table that fits the party and the interval
(`[start_at, end_at)`). Adjacent bookings (18:00–19:00 and 19:00–20:00)
do not conflict.

Double-booking is blocked by a PostgreSQL GiST exclusion constraint
(`btree_gist` + `tstzrange`) on the same table for `PENDING` and
`CONFIRMED` rows, plus `FOR UPDATE` locks during assignment. NULL
`table_id` values do not participate in that constraint.

Weekly hours live in `business_hours` (Sunday = 0). Multiple intervals
per day are allowed; overnight ranges such as 18:00–02:00 are rejected.
`business_schedule_exceptions` override the weekly template for one date.
Reservations must sit entirely inside one local interval on one local
date, using `businesses.timezone`. Party size and advance windows come
from existing `fnb_settings`. If `reservation_enabled` is false, create,
confirm, and availability return `409 RESERVATION_DISABLED`. If
`deposit_enabled` is true, confirm also requires a `PAID` reservation
payment or it returns `409 RESERVATION_PAYMENT_REQUIRED`.

`GET /reservations/availability` takes `date`, `start_time`,
`duration_minutes`, and `party_size` and combines them in the business
timezone. It never returns other customers' bookings.

OWNER and MANAGER have full `reservation.*` plus `business.update` for
hours. STAFF can read/create/update/confirm/cancel reservations but
cannot change hours. SQL integration and concurrency tests run when
`TEST_DATABASE_URL` is set.

## AI prompts, settings, and tool calling

JOB 11 adds an orchestration layer on top of existing domain services.
The model never talks to PostgreSQL, repositories, or arbitrary HTTP.
Registered tools call business, menu, customer, conversation, and
reservation services. API keys stay in the environment.

`ai_settings` is one row per business (`enabled`, `provider`, `model`,
optional `temperature` 0–2, optional `max_output_tokens` > 0). GET
creates defaults from `AI_PROVIDER` / `AI_MODEL` when no row exists.
PATCH cannot send an API key.

`ai_prompts` is versioned configuration. Create always inserts a
`DRAFT` with `version = max(name)+1`. Only one `ACTIVE` version exists
per `(business_id, name)` (partial unique index). Activate archives the
previous ACTIVE row. DELETE archives; history is not destroyed. If AI
is enabled but no ACTIVE prompt exists, the engine uses
`ai.DefaultSystemPrompt` rather than inventing a handler-local prompt.

The concrete provider is an OpenAI-compatible Chat Completions client
(`internal/ai/provider`). Groq and Ollama work through `AI_BASE_URL`.
`anthropic` can be stored but is not implemented in JOB 11.

Registered tools:

```
get_business_info
get_menu
search_customer
get_customer_reservations
check_availability
create_reservation
confirm_reservation
cancel_reservation
search_knowledge
```

`search_knowledge` retrieves unstructured tenant documents. It never
accepts `business_id` from the model. Structured tools above remain
authoritative for live menu, availability, reservations, and customer
lookup.

`check_availability` and reservation mutations delegate to the JOB 10
reservation service. Tenant and customer come from authenticated
context and the conversation, never from LLM `business_id`. Mutation
tools reject a `customer_id` that does not match the conversation
customer. Tool arguments are schema-validated before execution.

`POST /api/v1/ai/generate` requires `conversation.update`. Body:

```json
{
  "conversation_id": "...",
  "message": "Besok jam 7 malam ada meja untuk 4 orang?",
  "persist_incoming": true
}
```

`persist_incoming` defaults to true so the test endpoint can append the
customer message. Channel adapters should call `GenerateReply` with
`persist_incoming=false` after they already stored the inbound message.
The final AI reply is persisted as `OUTBOUND` / `AI` / `TEXT` /
`QUEUED`. JOB 11 does not send WhatsApp, Instagram, or any other
channel.

Orchestration limits: 20 recent messages, 5 tool rounds, 10 tool calls
per generation. Each LLM request uses `AI_TIMEOUT_SECONDS`. There is no
automatic retry, Redis, queue, worker, RAG, or embedding store.

OWNER has all `prompt.*` permissions. MANAGER can `prompt.read` and
`prompt.update`. STAFF can generate (`conversation.update`) but cannot
manage prompts or settings.

The default test suite mocks the LLM. Set `TEST_AI_LIVE=true` plus
`AI_API_KEY` to run one live provider call. Do not rely on that in CI.

## Channel integration (JOB 12)

The channel package is a thin adapter between messaging providers and the
existing conversation, customer, and AI domains.

```
WhatsApp Cloud API
        │
     Webhook (public)
        │
 WhatsApp adapter (verify + normalize)
        │
 Channel service
        │
 Conversation + Customer services
        │
 AI GenerateReply (JOB 11)
        │
 OUTBOUND / AI / TEXT / QUEUED
        │
 Channel delivery
        │
 WhatsApp Cloud API
```

WhatsApp is the first production provider. Instagram is reserved in the
`business_channels.channel` check (`WHATSAPP` | `INSTAGRAM`) but is not
implemented. Conversation and AI code never import Meta DTOs.

### Tenant resolution

Public webhooks do **not** use JWT or `X-Business-ID`. After the HMAC
signature is verified, the adapter extracts `metadata.phone_number_id`
and looks up `business_channels`:

```
phone_number_id  →  business_channels.external_phone_number_id
                 →  business_id
```

Unknown or inactive numbers are acknowledged without creating data so
Meta does not retry forever. Never trust a client-supplied business ID
on webhook routes.

### Production webhook

```
https://www.diuksolution.com/api/webhooks/whatsapp
```

Locally:

```
GET  http://localhost:8080/api/webhooks/whatsapp
POST http://localhost:8080/api/webhooks/whatsapp
```

Meta verification (`GET`):

1. `hub.mode` must be `subscribe`
2. `hub.verify_token` must match `META_WHATSAPP_VERIFY_TOKEN`
3. the handler returns `hub.challenge` as plain text

Signature verification (`POST`):

1. read the raw body
2. HMAC-SHA256(body, `META_WHATSAPP_APP_SECRET`)
3. constant-time compare to `X-Hub-Signature-256`
4. only then parse JSON

Invalid signatures return `401`. Malformed JSON returns `400`. Valid
events return `200` even when the payload is an unsupported type or a
duplicate, so Meta stops retrying.

### WhatsApp setup

1. Create a Meta app with WhatsApp Cloud API.
2. Set the webhook URL to the production path above (or a tunnel for local).
3. Subscribe to `messages`.
4. Put the verify token, app secret, and access token in the backend env.
5. Create a `business_channels` row via `POST /api/v1/channels` with the
   Meta `phone_number_id` (and optional WABA id / display name).

The backend does **not** provision Meta applications. It only stores
non-secret channel metadata. Tokens stay in environment variables for
JOB 12. The provider interface can later accept per-business secrets
without changing conversation or AI.

### Channel management API

Protected by JWT + `X-Business-ID` + RBAC:

| Permission | OWNER | MANAGER | STAFF |
|---|---|---|---|
| `channel.read` | yes | yes | no |
| `channel.create` | yes | yes | no |
| `channel.update` | yes | yes | no |
| `channel.delete` | yes | yes | no |

`GET`/`POST`/`PATCH` never return access tokens, app secrets, or verify
tokens. Soft-delete sets `status=INACTIVE`.

### Inbound / outbound

Inbound text becomes `INBOUND` / `CUSTOMER` / `RECEIVED` through the
existing conversation service. The WhatsApp message id is stored as
`messages.external_id`. Duplicates are ignored and do not call the AI
again.

After persist, the channel service triggers JOB 11 `GenerateReply` with
`persist_incoming=false`. The webhook does not wait for the LLM: the
trigger runs on a detached context with a timeout. JOB 13 can replace
that trigger with a queue worker.

Outbound `QUEUED` text is sent as:

```
POST {META_WHATSAPP_BASE_URL}/{version}/{phone_number_id}/messages
```

Success: `QUEUED` → `SENT` and the provider `wamid` is stored.
Failure: `QUEUED` → `FAILED`. There is no automatic retry.

Status webhooks (`sent` / `delivered` / `read` / `failed`) update the
message by `wamid`. Progression never goes backwards
(`READ` cannot become `SENT`). `FAILED` is only accepted from
`QUEUED` or `SENT`.

Unsupported inbound types (image, audio, video, document, sticker,
location, interactive, button) are acknowledged without creating a
malformed message.

### Local testing

Default tests use in-memory stores and a fake Graph API. They never
need Meta credentials.

Set `TEST_WHATSAPP_LIVE=true` plus:

```
META_WHATSAPP_ACCESS_TOKEN
META_WHATSAPP_PHONE_NUMBER_ID
META_WHATSAPP_TO
```

to run one optional live send. Do not enable that in CI. Never commit
tokens or app secrets.

### Security notes

- Webhook routes are public and rely on verify-token + HMAC, not JWT.
- Protected `/api/v1/channels` still require JWT + tenant + RBAC.
- Secrets are never logged, never stored in `business_channels`, and
  never returned in API responses.
- Logs may include `channel`, `business_id`, `conversation_id`,
  `message_id`, `external_message_id`, `event_type`, and `status`.
  They do not include tokens, secrets, full phone numbers, or message
  bodies.

### Current limitations (JOB 15+)

Instagram, Qiscus, web chat, media processing, OCR, broadcasts, and
scheduled follow-ups are not implemented. RAG uses PostgreSQL/pgvector
only; scanned PDFs without extractable text fail instead of using OCR.

## Jobs and workers (JOB 13)

```
Webhook
  → persist inbound (PostgreSQL)
  → job_outbox + Redis stream (AI_GENERATE)
  → AI worker → GenerateReply()
  → persist QUEUED outbound
  → job_outbox + Redis stream (MESSAGE_DELIVERY)
  → delivery worker → DeliverMessage()
  → WhatsApp
```

PostgreSQL stays the source of truth. Redis is only transport.

### Redis

Local Compose runs `redis:7.4.2-alpine` with AOF. The API process
connects with `REDIS_URL`. Passwords are never logged. Health:

```
GET /api/v1/health/redis
```

answers `ok` or `unavailable` without a connection string.

Streams:

```
{prefix}:stream:ai
{prefix}:stream:delivery
{prefix}:stream:knowledge
```

Consumer groups: `ai-workers`, `delivery-workers`, `knowledge-workers`.
Abandoned pending entries are reclaimed with `XAUTOCLAIM`.

### Workers

`APP_MODE=api|worker|all` (default `all`). Dedicated process:

```
go run ./cmd/worker
```

AI concurrency and delivery concurrency are bounded. Each job has its
own timeout. SIGINT/SIGTERM stops new claims, finishes in-flight work,
then closes Redis and PostgreSQL.

### Retry and DLQ

Retryable: timeouts, provider 5xx, WhatsApp 429 (`Retry-After` when
present), Redis blips. Permanent: 4xx, unknown tenant, invalid payload,
unsupported type. Max attempts default 3, exponential backoff + jitter
(~30s then ~2m). Exhausted jobs go to:

```
{prefix}:dead:ai
{prefix}:dead:delivery
{prefix}:dead:knowledge
```

with a 7-day TTL.

### Durability

`job_outbox` is written before Redis. If Redis is down, the inbound
message and outbox row remain. The publisher retries PENDING rows.
Workers ACK only after the domain call succeeds.

### Failure recovery

| Event | Result |
|---|---|
| Redis down after DB write | Outbox stays PENDING; publisher retries |
| Worker crash after claim | `XAUTOCLAIM` / reclaim gives the job to another worker |
| Provider 5xx / timeout | Revert `SENDING` → `QUEUED`, retry |
| Provider 429 | Honor `Retry-After`, then retry |
| AI timeout | Retry if attempts remain, else DLQ |
| Duplicate job | AI claim table / delivery `QUEUED→SENDING` claim skip |

Set `TEST_REDIS_URL` to run stream integration tests. Default tests use
an in-memory queue and do not need Redis.

## Knowledge Base + RAG (JOB 14)

Each tenant can upload unstructured documents. The AI retrieves relevant
chunks during conversations through the existing tool registry. Structured
tools remain authoritative for live menu, availability, reservations, and
customer lookup.

```
POST /knowledge/documents
  → store file privately
  → job_outbox + Redis (KNOWLEDGE_INGEST)
  → worker: PENDING → PROCESSING
  → extract TXT/PDF/DOCX → chunk → embed
  → replace chunks + READY
```

```
GenerateReply
  → LLM
  → search_knowledge (business_id from auth context only)
  → PostgreSQL + pgvector cosine search
  → LLM
```

Supported sources: `.txt`, `.pdf`, `.docx`. OCR is not implemented.
Scanned PDFs without extractable text fail with a sanitized processing
error. Documents are stored at:

```
{business_id}/knowledge/{document_id}/source.{ext}
```

using the existing private Supabase Storage client. Responses never
include embeddings or storage credentials.

### pgvector

Migration `000012` enables `vector` and stores embeddings as
`vector(1536)` for `text-embedding-3-small`. Keep
`AI_EMBEDDING_DIMENSIONS` aligned with that column. Retrieval is
tenant-scoped, READY-only, cosine similarity, configurable top K and
minimum similarity.

Local development:

```
# enable the vector extension on the test/dev database
go run ./cmd/migrate up
# optional: set AI_EMBEDDING_ENABLED=true and an embedding API key
```

When `AI_EMBEDDING_ENABLED=false`, knowledge CRUD still works,
ingestion fails with a configuration error, `search_knowledge` returns
a controlled unavailable result, and normal AI conversations continue.

OWNER and MANAGER have `knowledge.read|create|update|delete`. STAFF has
`knowledge.read` only.

## Automation Engine (JOB 15)

Each tenant can define rule-driven workflows. The LLM is optional: template
reminders stay deterministic; personalized follow-ups reuse the existing AI
orchestrator. Automation never calls Meta Graph or the LLM provider directly.

```
Domain change
  → automation_events (idempotent)
  → AUTOMATION_EVALUATE
  → conditions
  → automation_runs
  → AUTOMATION_RUN
  → SEND_MESSAGE | AI_GENERATE_MESSAGE
  → existing conversation + delivery outbox / AI GenerateReply
```

Supported triggers: `RESERVATION_CONFIRMED`, `RESERVATION_CANCELLED`,
`CONVERSATION_CREATED`, `CUSTOMER_CREATED`, `SCHEDULED` (reservation
`start_at` + `offset_minutes`), `CONVERSATION_INACTIVE` (scheduler only).

Supported actions: `SEND_MESSAGE` (safe `{{variable}}` templates) and
`AI_GENERATE_MESSAGE` (existing orchestrator, mutating reservation tools
disabled).

Conditions are allowlisted fields with `EQ`, `NEQ`, `IN`, `NOT_IN`, `GT`,
`GTE`, `LT`, `LTE`, `EXISTS`, `NOT_EXISTS`. Templates may use
`customer.first_name`, `customer.full_name`, `reservation.start_time`,
`reservation.start_date`, `reservation.party_size`, `reservation.status`,
`business.name`, `business.phone`. Unknown variables are rejected at
create/update. Missing runtime values render as empty.

The worker process runs a PostgreSQL-locked scheduler every
`AUTOMATION_SCHEDULER_INTERVAL` seconds. Duplicate events and scheduled
keys are unique in PostgreSQL. Runs re-check ACTIVE automation, business,
subject, and conditions before sending. Cancelled reservations do not get
reminders. A conversation that receives a new customer inbound after
scheduling does not get a stale inactivity follow-up. Outbound automation
messages are `SYSTEM` or `AI` and do not emit customer-originated events.

When `AUTOMATION_ENABLED=false`, CRUD still works; evaluation, scheduler,
and workers no-op. Chat and reservations stay unaffected.

OWNER and MANAGER have `automation.read|create|update|delete`. STAFF has
`automation.read` only.

## Human Handover & Shared Inbox (JOB 16)

AI conversations can be handed to humans without replacing the existing
OPEN/CLOSED lifecycle. Handling state is separate:

```
AI_ACTIVE
  → HUMAN_REQUESTED   (customer request, AI unable to answer, or staff handoff)
  → HUMAN_ACTIVE      (any authorized user opens or replies)
  → RESOLVED
  → AI_ACTIVE         (resume AI)
```

Handed-over conversations appear in a shared Human Handover inbox
(`GET /api/v1/conversations/human-inbox`). There is no mandatory assignee.
Owner, Manager, and Staff can see and reply when they have
`conversation.read` / `conversation.update`. Another authorized user can
always take over the same thread.

Human replies reuse the existing outbound pipeline:

```
POST /conversations/:id/reply
  → USER / OUTBOUND / QUEUED
  → delivery outbox
  → MESSAGE_DELIVERY
  → MessagingProvider
```

AI generation is suppressed unless `handling_state = AI_ACTIVE`. That
check runs at execution time, so a stale `AI_GENERATE` job queued before
handover does not send an AI reply. Automation `AI_GENERATE_MESSAGE`
follows the same rule. Template `SEND_MESSAGE` stays explicit and does
not reactivate AI.

Humans can resolve a conversation and later resume AI. Resume does not
regenerate historical messages.

Internal notes (`POST /conversations/:id/notes`) are `INTERNAL_NOTE`
messages with status `INTERNAL`. They never enter the delivery queue and
are never sent to WhatsApp.

## Analytics (JOB 17)

Business analytics are derived from existing transactional tables. There
is no `analytics_events` warehouse table. Every query is scoped by the
authenticated `X-Business-ID`; a `?business_id=` query parameter cannot
override tenant context.

```
Authorization: Bearer <Supabase JWT>
X-Business-ID: <business UUID>
```

OWNER and MANAGER have `analytics.read`. STAFF does not.

Date range query parameters:

```
?from=2026-09-01&to=2026-09-24
```

`from` and `to` accept an ISO date (`YYYY-MM-DD`) or an RFC3339 timestamp.
If both are omitted, the default is the last **30 inclusive calendar days**
in the business timezone, ending at the start of tomorrow local time.

Date-only values are interpreted in the business timezone:

* `from=2026-09-01` is `2026-09-01 00:00` local time
* `to=2026-09-24` is **inclusive** of that calendar day, implemented as
  the half-open UTC interval `[from, start of 2026-09-25 local)`

RFC3339 timestamps are used as-is after conversion to UTC. Ranges are
half-open `[from, to)` and may not exceed 366 days.

Each response uses:

```json
{
  "data": {
    "from": "2026-08-25T17:00:00Z",
    "to": "2026-09-24T17:00:00Z",
    "timezone": "Asia/Jakarta",
    "summary": {},
    "series": [{"date": "2026-09-01", "value": 12}]
  }
}
```

Daily series dates are local calendar dates in the business timezone.

### Metric definitions

Period counts (unless noted) include rows whose `created_at` is in
`[from, to)`.

| Metric | Definition |
| --- | --- |
| Conversation totals | Conversations created in range, split by current `OPEN`/`CLOSED` |
| Handling states | Current `handling_state` of conversations created in range |
| `ai_handled` | Conversations created in range with `handover_requested_at IS NULL` |
| `human_requested` | Conversations created in range with `handover_requested_at IS NOT NULL` |
| `human_handled` | Conversations created in range with `human_started_at IS NOT NULL` |
| Average conversation duration | `last_message_at` (else `updated_at`) minus `created_at`, closed conversations only |
| `total_customers` / `active_customers` | Snapshot of all customers for the tenant (not period-created) |
| `new_customers` | Customers whose `created_at` is in `[from, to)` |
| `customers_with_conversations` / `customers_with_reservations` | Distinct customer snapshots |
| Reservation totals and rates | Reservations created in range, counted by **current** status. `confirmation_rate = confirmed / total` (0 when total is 0). Same pattern for cancellation and completion |
| AI generations | Rows in `ai_generations` created in range. Status is `PENDING` / `SUCCESS` / `FAILED` / `SKIPPED` |
| `ai_messages_generated` | Customer-facing messages with `sender_type = AI` |
| Average AI latency | Mean `latency_ms` where present. Null when no timed generations exist |
| Handover counts | Conversations whose `handover_requested_at` is in `[from, to)` |
| Time to human | `human_started_at - handover_requested_at` for complete pairs only |
| Human handling duration | `resolved_at - human_started_at` for complete pairs only |
| Automation runs | `automation_runs` created in range. Success = `COMPLETED`, failure = `FAILED`, skipped = `CANCELLED` |
| Messaging | Current persisted message status. `INTERNAL_NOTE` is excluded. Statuses are not summed across transitions |

Human messages in overview are customer-facing `sender_type = USER` messages
(staff/human replies), not customer inbound.

## Observability (JOB 17)

Analytics answers “what happened for this business?” Observability answers
“is the process healthy?”

Every HTTP response includes `X-Request-ID`. An incoming `X-Request-ID` is
preserved (max 128 characters); otherwise a UUID is generated. The ID is
stored on the request context and included in structured request logs with
`method`, `path` (Gin route or `unmatched`), `status`, `duration_ms`,
`error_category`, and `business_id` / `user_id` when authenticated.

Logs do **not** include Authorization headers, JWTs, API keys, request
bodies, customer message text, Meta secrets, or LLM prompts/responses.

`GET /metrics` exposes Prometheus-compatible aggregate counters and
histograms. It is **not** protected by the business JWT. In production,
restrict it at the reverse proxy or network layer. Labels are low
cardinality (`method`, `route`, `status`, `job_type`, `queue`, `provider`,
`channel`, `operation`, `result`, `error_category`). Metrics never use
`business_id`, `conversation_id`, `customer_id`, or user-controlled
strings.

Tracked series include HTTP requests, job processed/failed/retried/dead-
lettered, queue depth (Redis stream pending), AI generation attempts, and
WhatsApp send attempts. Worker, AI provider, and messaging provider
boundaries record duration and a safe error category
(`timeout`, `rate_limited`, `provider_4xx`, `provider_5xx`,
`invalid_response`, `configuration`, `unknown`).

`/api/v1/health` is liveness only. AI provider outages do not fail it.

## Broadcast & Campaign Engine (JOB 18)

Campaigns are audience-driven and proactive. They are not automations.
A campaign decides **who** and **when**; existing message delivery decides
**how**.

```
Campaign
  → audience snapshot
  → campaign_run + campaign_recipients
  → CAMPAIGN_START / CAMPAIGN_DISPATCH
  → CreateMessage(sender=CAMPAIGN, status=QUEUED)
  → SetOnQueuedOutbound
  → job_outbox
  → MESSAGE_DELIVERY
  → MessagingProvider (WhatsApp Cloud API)
```

Supported channel in this job: `WHATSAPP` only (`TEXT` or `TEMPLATE`).
WhatsApp campaigns outside an active customer-service window should use an
approved provider template (`template_name`, `template_language`,
`template_parameters`). DIUK does not approve templates and never stores
Meta tokens.

Audience filters are structured JSON only (`customer_status`, `tags`,
`channels`, created/conversation/reservation dates and reservation
status). Unknown fields return `400`. The live CRM is queried once when a
run starts; membership is then an immutable `campaign_recipients`
snapshot (`ON CONFLICT DO NOTHING`). Later CRM changes do not add or
remove recipients on that run.

Customers have `marketing_opt_in` (default `true`). Opted-out customers
are `SKIPPED` with `MARKETING_OPT_OUT` at snapshot time and again
immediately before a message is created.

States: `DRAFT` → `SCHEDULED`/`RUNNING`/`CANCELLED`; `SCHEDULED` →
`RUNNING`/`PAUSED`/`CANCELLED`; `RUNNING` → `PAUSED`/`COMPLETED`/`CANCELLED`;
`PAUSED` → `RUNNING`/`CANCELLED`. Completed and cancelled campaigns cannot
restart. Pause leaves pending recipients pending; already queued/sent
messages are not recalled. Cancel skips remaining `PENDING` recipients
with `CAMPAIGN_CANCELLED`.

Scheduling uses `scheduled_at` (`TIMESTAMPTZ`, RFC3339 from the client,
interpreted in business-timezone terms). Past timestamps more than one
minute old are rejected. The existing worker process ticks a campaign
scheduler with PostgreSQL advisory lock `872316`. One active run per
campaign (`PENDING`/`RUNNING`/`PAUSED`) is unique.

Dispatch claims bounded batches (`CAMPAIGN_BATCH_SIZE`, default 50) and
enqueues the next `CAMPAIGN_DISPATCH` after `CAMPAIGN_DISPATCH_INTERVAL_MS`.
Workers never call Meta or `MessagingProvider.Send`. Provider 429/`Retry-After`
and 5xx stay on the existing delivery retry path. Permanent provider
rejections mark the recipient `FAILED`.

Delivery is **at-least-once**: a crash after `CreateMessage` + outbox
insert and before `message_id` is linked may leave an outbound campaign
message that a retry will attach rather than send twice through a new
claim. PostgreSQL claim (`PENDING` → `QUEUED` with `SKIP LOCKED`) is the
source of truth; Redis is not used for idempotency. Campaign messages do
not emit `conversation.created` automation events, do not change
`handling_state`, and do not reopen AI handling. A customer reply uses
the normal inbound WhatsApp flow.

OWNER and MANAGER have `campaign.read|create|update|delete`. STAFF has
`campaign.read` only. Cross-tenant campaign, run, and recipient access
returns `404`. Structured logs and Prometheus labels never include
phone numbers, message bodies, template parameters, campaign IDs,
customer IDs, or secrets. Campaign metrics
(`campaign_run_total`, `campaign_recipient_total`,
`campaign_recipient_failed_total`, `campaign_recipient_skipped_total`,
`campaign_dispatch_duration_seconds`) use low-cardinality labels only.

To run the isolated 15↔16 migrate cycle against `TEST_DATABASE_URL`, set
`TEST_CAMPAIGN_MIGRATE=1` and do not set other `TEST_*_MIGRATE` flags in
the same process.

## Usage, Quota & Entitlement Engine (JOB 19)

Plans, entitlements, and usage live in PostgreSQL. There is no second
billing system and no payment gateway. Redis is never the authoritative
counter.

```
Plan → Entitlements → Usage meter → Quota enforcement → Feature execution
```

Seeded plans (`FREE`, `STANDARD`, `PLUS`, `PRO`, `BUSINESS`) and their limits are
rows in `plans` / `plan_entitlements`, not Go constants or `.env` values.
`NULL` `limit_value` means unlimited. Resource entitlements
(`KNOWLEDGE_DOCUMENTS`, `WHATSAPP_CHANNELS`) use period `NONE` and are
checked against live counts. Monthly entitlements
(`AI_RESPONSES`, `CAMPAIGN_RECIPIENTS`, `AUTOMATION_RUNS`) reset on
calendar-month boundaries in the **business timezone** (half-open
`[start, end)` stored as `TIMESTAMPTZ`). Example: `Asia/Jakarta` on
2026-09-24 is `2026-09-01T00:00:00+07:00` → `2026-10-01T00:00:00+07:00`.
The first request of a month creates the period with
`INSERT … ON CONFLICT DO NOTHING`. No cron job is required.

New businesses receive `PLUS` (`plan_id` default, 50,000 AI Responses /
month). Existing `STANDARD` tenants were moved to `PLUS` when that tier
was introduced so they kept the previous 50k quota. `STANDARD` is now
the 25,000 AI Response entry tier. Plan assignment is not exposed on
`PATCH /api/v1/business`. Plan changes apply the **current** plan's
entitlements to the current period; historical counters are not rewritten
or reset. There is no prorating or refund.

**AI Responses** count successful customer-facing AI generations. Tool
calls, retries, failed generations, tokens, and inbound customer messages
are not counted as AI Responses. Consumption is idempotent on
`(business_id, feature_key, source_type, source_id)` using
`ai_generations.inbound_message_id` (or a per-request UUID for HTTP
generate). The AI worker checks quota before the LLM, then consumes
atomically after a successful reply and before persisting the outbound
AI message. A crash after consume and before persist is recovered by
retry: consume is a no-op and the reply can still be written. A second
concurrent job that loses the last remaining unit receives
`AI_QUOTA_EXCEEDED` and does not persist an unmetered reply.

Quota exhaustion is a permanent business error: the LLM is not called,
Redis does not retry the job, and the generation is marked `SKIPPED`
(same controlled path as human handover). HTTP generate returns `409
AI_QUOTA_EXCEEDED`. Customers never see internal IDs.

Campaign recipient and automation run metering use the same atomic
consume path (`campaign_recipient.id`, `automation_run.id`). Skipped
opt-outs and invalid recipients are not counted. Resource creates
(WhatsApp channels, knowledge documents) check the live count against
the current entitlement.

`Consume` is a single PostgreSQL transaction: insert the consumption
row, increment `usage_counters.used` only when `limit IS NULL OR used + n
<= limit`, otherwise delete the consumption and return `QUOTA_EXCEEDED`.
Concurrent consumes cannot exceed the limit.

### Current seeded limits

| Plan     | AI Responses | Campaign recipients | Automation runs | Knowledge docs | WhatsApp channels |
|----------|-------------:|--------------------:|----------------:|---------------:|------------------:|
| FREE     |         1000 |                 500 |             100 |             10 |                 1 |
| STANDARD |        25000 |              100000 |           10000 |            100 |                 3 |
| PLUS     |        50000 |              100000 |           10000 |            100 |                 3 |
| PRO      |       150000 |              500000 |           50000 |            500 |                10 |
| BUSINESS |    unlimited |           unlimited |       unlimited |      unlimited |            custom |

### API

- `GET /api/v1/usage` — current plan, monthly period, and usage/resource snapshot
- `GET /api/v1/usage/history` — previous monthly periods (`limit` default 12, max 24)

OWNER, MANAGER, and STAFF have `usage.read`. There is no
`usage.update` / `usage.consume` for normal users. Unlimited features
return `"limit": null, "remaining": null`.

Metrics: `usage_consumption_total` and `usage_quota_exceeded_total`
labeled only by `feature` and `result`. Logs never include message
bodies, phone numbers, prompts, or secrets.

Limitations: no invoices, subscriptions, payment collection, prorating,
refunds, or billing admin UI. PostgreSQL atomicity plus source
idempotency prevent duplicate accounting under at-least-once workers;
this is not distributed exactly-once delivery.

To run the isolated 16↔17 migrate cycle against `TEST_DATABASE_URL`, set
`TEST_USAGE_MIGRATE=1` and do not set other `TEST_*_MIGRATE` flags in
the same process.

## Manual Payment & Reservation Confirmation (JOB 20)

Reservation status and payment status are independent:

```
PENDING / UNPAID  →  PENDING / PAID  →  CONFIRMED / PAID
```

`PAID` never auto-confirms a reservation. Cancel never auto-refunds.
`CONFIRMED / UNPAID` is allowed when `fnb_settings.deposit_enabled` is
false (pay at venue / cash / outside DIUK). When deposit is enabled,
`ReservationService.Confirm` requires `payment.status = PAID` and
rejects `UNPAID` or `REFUNDED` with `409 RESERVATION_PAYMENT_REQUIRED`.

`fnb_settings` now also stores `deposit_enabled` (default false),
`deposit_type` (`FIXED` or `PERCENTAGE`), and `deposit_value`. Creating
a reservation snapshots the required amount onto one
`reservation_payments` row (`UNIQUE (business_id, reservation_id)`).
Existing reservations were backfilled as `UNPAID` with `amount = 0`.
`PERCENTAGE` currently snapshots `0` because reservations have no bill
total yet; the deposit flag still blocks confirmation until an admin
marks the payment `PAID`.

Money uses integer minor units (1/100). JSON `100000` means
`100000.00`. Do not use `float64` for amounts. Currency comes from the
business F&B settings, not a hardcoded `IDR`.

Manual admin operations only:

- `POST /reservation-payments/:id/confirm` — `UNPAID → PAID`, records
  `confirmed_by` / `confirmed_at`. Retry after success is idempotent.
- `POST /reservation-payments/:id/refund` — `PAID → REFUNDED` accounting
  state only. No gateway call, no partial refund.
- `PATCH /reservation-payments/:id` — operational notes only. Do not
  store card numbers, CVV, OTP, or passwords.

OWNER and MANAGER have `payment.read` + `payment.update`. STAFF has
`payment.read` only. AI and automation cannot mark payments paid.
Reservation confirmation still goes through `ReservationService`.

This job is manual payment only. No Midtrans, Xendit, Stripe, QRIS,
virtual accounts, or webhooks. A later provider can sit behind
`PaymentService` without rewriting reservation rules.

000018 already added the PLUS plan, so this schema is
`000019_create_manual_payment_tables`. To run the isolated 18↔19
migrate cycle against `TEST_DATABASE_URL`, set `TEST_PAYMENT_MIGRATE=1`
and do not set other `TEST_*_MIGRATE` flags in the same process.

## Calendar & Schedule Management (JOB 21)

Reservations, `business_hours`, and `business_schedule_exceptions` remain
the source of truth. Calendar events are a read model. `calendar_blocks`
withhold bookable capacity without replacing hours or exceptions.

A block may be business-wide (`area_id` and `table_id` null), area-wide,
or table-specific. A table block must include its area. Intervals are
half-open `[start_at, end_at)` in `TIMESTAMPTZ`. Same-scope ACTIVE blocks
cannot overlap; different scopes may.

Creating a block that overlaps a `PENDING` or `CONFIRMED` reservation
returns `409 CALENDAR_BLOCK_CONFLICT` and does **not** cancel the
booking. Availability and reservation create/reschedule treat an ACTIVE
block as unavailable (`409 CALENDAR_BLOCKED`). AI reservation tools use
the same reservation service, so they inherit the rule.

`PATCH /api/v1/reservations/:id/reschedule` moves `PENDING` or
`CONFIRMED` bookings through the existing availability checks. Payment
status and amount are not changed. `CANCELLED` / `COMPLETED` cannot be
rescheduled.

`GET /api/v1/calendar/day?date=YYYY-MM-DD` interprets the date in the
business timezone. `GET /api/v1/calendar/events` requires `from`/`to`
(max 366 days) and returns reservations plus blocks, ordered by
`start_at`, then type, then id.

OWNER and MANAGER have `calendar.read` + `calendar.update`. STAFF has
`calendar.read` only. Reschedule uses `reservation.update`. DELETE on a
block sets `status = CANCELLED`.

No Google/Outlook sync, recurring events, or frontend changes.

To run the isolated 19↔20 migrate cycle against `TEST_DATABASE_URL`, set
`TEST_CALENDAR_MIGRATE=1` and do not set other `TEST_*_MIGRATE` flags in
the same process.

## Revenue & Conversion Analytics (JOB 22)

JOB 17 analytics stay in place. This job adds descriptive funnel and
payment totals. There is still no `analytics_events` table and no
accounting or revenue-recognition model.

`GET /api/v1/analytics/reservations` is **extended** (existing fields
unchanged) with party size, area/table distribution, and booking hour /
day-of-week buckets. Hour and weekday use reservation `start_at` in the
business timezone. `day_of_week` is Sunday = 0, matching `business_hours`.
Area/table breakdowns use the reservation's current `table_id` join;
unassigned bookings appear as area name `Unassigned`. If a table later
moves areas, historical rows follow the current area.

`GET /api/v1/analytics/conversion` describes overlap, not causation.
`conversations_with_reservation` is conversations created in `[from, to)`
whose customer also has a reservation created in that same window.
`conversation_reservation_rate = conversations_with_reservation / conversations`
(0 when there are no conversations). Reservation rates use current
status among reservations created in range.

Payment metrics are operational deposit/payment amounts from
`reservation_payments`. They are **not** recognized accounting revenue.

* `reservations_requiring_payment` = payment rows with `amount > 0`
* `reservations_paid` = `status = PAID`
* `reservations_unpaid` = `status = UNPAID` and `amount > 0`
* `payment_collection_rate = paid / requiring_payment` (0 when none require payment)
* `paid_reservation_confirmation_rate = (PAID and CONFIRMED or COMPLETED) / PAID`

Zero-amount payment rows (no deposit required) are counted as payment
records but are not treated as requiring collection.

`GET /api/v1/analytics/revenue` filters `reservation_payments.created_at`
in `[from, to)`. Conversion payment counts instead follow the reservation
created-at cohort so they stay aligned with the funnel. Amounts use the
same minor-unit `money.Amount` JSON as the payment API.

Date range, timezone, half-open `[from, to)`, 366-day cap, and
`analytics.read` (OWNER + MANAGER; STAFF denied) are unchanged.

To run the isolated 20↔21 migrate cycle against `TEST_DATABASE_URL`, set
`TEST_REVENUE_MIGRATE=1` and do not set other `TEST_*_MIGRATE` flags in
the same process.

## Production hardening

JOB 23 is a reliability and security pass, not a schema change.

- HTTP timeouts, header/JSON/webhook body caps, and shutdown timeout are
  environment-driven. Production rejects wildcard CORS and test-only flags
  (`TEST_AI_LIVE`, `TEST_WHATSAPP_LIVE`).
- JSON endpoints share `HTTP_MAX_JSON_BYTES`. Menu images and knowledge
  documents keep their existing multipart limits. WhatsApp webhooks use
  `WEBHOOK_MAX_BODY_BYTES` and still verify HMAC-SHA-256 on the raw body.
- The PostgreSQL pool stays small (`DB_MAX_CONNS` default 10) for
  Supabase poolers.
- Application rate limits are in-process token buckets keyed by IP
  (webhooks) or `business_id` (AI, uploads, campaign start). They fail
  open so Redis outage cannot take the API down.
- Workers recover panics, bound concurrency, retry only classified
  transient failures, and finish in-flight jobs with their existing job
  timeout after SIGTERM. Process shutdown waits at most the longest
  worker timeout plus a short buffer.
- Distroless runtime image runs as `nonroot`. There is no shell/`curl`
  healthcheck in the image; use `/api/v1/health` and `/api/v1/health/ready`.

Migration: none. Final schema remains `000021_revenue_conversion_analytics`.
