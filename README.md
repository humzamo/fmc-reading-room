# FMC Reading Room Sync

A command-line tool that mirrors the Federal Maritime Commission's public
[reading room](https://www2.fmc.gov/readingroom/) — every proceeding and
every document filed under it — to local disk and a SQLite database, and
keeps that mirror up to date.

## Why this exists

The reading room has no public API. Every document (orders, complaints,
briefs, notices — ~19,000 of them across ~700 proceedings, and growing) is
only reachable by clicking through a legacy ASP.NET web UI, one proceeding
at a time. There's no way to search, script against, or bulk-download the
underlying files, and no way to know what's changed since you last looked
without re-checking everything by hand.

This tool builds a queryable local copy: real files on disk you can
`grep`/`open`, a SQLite database you can run SQL against, and a repeatable
`sync` command that only fetches what's actually new.

## How it works

### Discovery: finding every proceeding

The reading room exposes a `ProceedingSearch` page with filters for
Proceeding Type (6 values: Dockets, Petition, Notice of Inquiry, Fact
Finding, Special Permission, Special Investigation) and Is Closed. Neither
field is displayed anywhere else on the site — not in search results, not
on a proceeding's own page — so the only way to learn a proceeding's type
or closed status is to run one search per type value plus one for
`Is Closed = Yes`, and note which proceeding numbers come back from each.

A plain, unfiltered search is *also* run, because at least one real
proceeding on the site has no type value set at all and is invisible to
every type-specific filter — it only shows up under "-- ALL --". Skipping
this catch-all pass would silently and permanently miss any proceeding like
it.

Every run does this full discovery pass and upserts the result: new
proceedings get a row (and a permanent on-disk folder — see below);
existing ones get their title/type/closed/created-date bookkeeping
refreshed. This part is cheap (well under a minute) regardless of how the
tool is invoked.

### Finding documents: two strategies

**Default.** For every proceeding, fetch its detail page
(`/readingroom/proceeding/{number}/`) — a plain, non-paginated page listing
every document ever filed under it (number, serve date, description, a
direct file link) — and download whatever document numbers aren't already
recorded. This is a full, from-scratch-equivalent check: safe, always
complete, but does one HTTP request per proceeding every time.

**`--since YYYY-MM-DD`.** Instead of checking every proceeding, run one
query against the site's `DocumentSearch` page filtered by
"document serve date on or after this date" and download whatever comes
back that isn't already recorded. This is much cheaper when only a small
delta is expected, and — because the filter is on each *document's* own
serve date, not on when its *proceeding* was created — it's just as
reliable at finding a brand-new filing on a years-old, otherwise-dormant
proceeding as one on a proceeding created yesterday. (Proceeding discovery,
above, always runs in full either way; only the document-finding step
changes.)

Either way, a document is identified by `{proceeding_number}_{document_number}`.
Document numbers are assigned once and never reused, so this pair is a
stable, permanent key — which is what makes the whole thing idempotent: a
document already in the database is never re-fetched, so re-running `sync`
after a previous run (complete or partial/interrupted) only ever does the
work that's actually left.

### Downloading and naming

Each proceeding gets a folder at `proceedings/{number}_{title}/`; each
document is saved as `{doc_number}_{description}.{ext}` inside it. Both
names are **computed once, at first-download time, and never changed
again** — even if the site later edits a title or description — so nothing
you've already downloaded ever gets silently renamed or moved out from
under you. The extension is taken from the response's `Content-Type` (or
its suggested filename), defaulting to `.pdf` since that's effectively
every document on this site.

A document whose link 404s (this happens — a handful of the site's own
historical links are dead) is still recorded, with every field the site's
own listing provided intact, but with `source_url` set to the literal
string `file_unavailable` and no file. Recording it (rather than leaving it
missing) is what stops future runs from retrying the same dead link
forever; query for `source_url = 'file_unavailable'` to find every one and
check it by hand.

### The database

Three tables in `fmc.db`:

- **`proceedings`** — number (primary key), title, permanent folder name,
  created date, last-updated date, type, closed status.
- **`documents`** — unique key, proceeding number, document number, served
  date, description, source URL, permanent file name/path, content type,
  size, downloaded-at.
- **`sync_runs`** — the run log: start/finish time, status, and counts
  (proceedings scanned, new proceedings, new/downloaded/unavailable/failed
  documents). `status` reads its latest row.

It's a plain SQLite file — open it with the `sqlite3` CLI or any SQLite
client and query it directly.

## How to run it

```bash
go build -o bin/fmc-reading-room ./cmd/fmc-reading-room
```

Then, from the directory where you want `fmc.db` and `proceedings/` to
live (this repo's root, normally):

```bash
./bin/fmc-reading-room sync              # download everything new
./bin/fmc-reading-room sync --since 2026-09-01   # fast path: only check for documents served on/after this date
./bin/fmc-reading-room sync --dry-run    # show what would happen without writing anything
./bin/fmc-reading-room status            # show the last run's outcome and current totals
```

Those two flags are the only ones exposed at runtime. Everything else
(database path, download folder, base URL, concurrency, per-request delay,
a proceeding-count limit used only for development smoke-testing) is a
constant at the top of `cmd/fmc-reading-room/main.go` — edit and rebuild if
you need a different value. The concurrency (20) and per-request delay
(75ms) currently set were tuned empirically against the live site: they cut
the initial ~19,000-document backfill from ~2h20m to a bit over an hour
with no sign of the server straining.

There's no built-in scheduler — run `sync` by hand, or point cron/launchd
at it (with its working directory set to this repo) for a recurring job.

## Assumptions and limitations

- **Proceeding numbers and document numbers are permanent and never
  reused.** This is the whole basis for the idempotent, diff-based design.
- **Titles and descriptions may change on the site after the fact; this
  tool doesn't track that.** The first-seen value is what gets used to
  name the folder/file, and it's never revisited.
- **Scraping a UI, not an API.** The site is a legacy ASP.NET WebForms app;
  its search grids are paginated via full-page postbacks that this tool
  reverse-engineered field-by-field (view state, event validation, the
  Telerik widgets' client-state JSON). A significant markup or behavior
  change on the site could break discovery or search without much warning
  beyond a parsing/HTTP error.
- **`--since` trusts the site's own date filter and index.** If the site's
  `DocumentSearch` index is ever behind reality for some document, this
  tool would be too, until a full (no-`--since`) run catches it — the
  default mode never trusts a date cutoff, by design, for exactly this
  reason.
- **Single local SQLite file, single writer.** Not designed for two `sync`
  runs against the same database at once.
- **A handful of documents genuinely 404 on the site itself** — pre-
  existing dead links in the reading room's own records, not a bug here.
  They're recorded as unavailable (see above) rather than retried forever.
- **Politeness over raw speed.** Requests are rate-limited and use a
  generic descriptive User-Agent; this is a low-volume personal research
  tool, not built to maximize throughput against a public government
  service.
