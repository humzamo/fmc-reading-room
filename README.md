# FMC Reading Room Sync

A tool that mirrors the Federal Maritime Commission's public
[reading room](https://www2.fmc.gov/readingroom/) — every proceeding and
every document filed under it (~19,000 documents across ~700 proceedings,
and growing) — to local disk and a SQLite database, keeps that mirror up to
date daily, and publishes it as a searchable website.

The reading room has no public API; everything is only reachable by
clicking through a legacy ASP.NET UI one proceeding at a time, with no way
to bulk-download or know what's changed since you last looked. This tool
builds a queryable copy instead: real files on disk, a SQLite database you
can run SQL against, and a `sync` command that only fetches what's new.

Live search UI: **https://humzamo.github.io/fmc-reading-room/**

## How it works

**Discovery.** The site's `ProceedingSearch` page is the only way to learn a
proceeding's type or closed status — neither is shown anywhere else — so
discovery runs one search per proceeding type, plus one for `Is Closed =
Yes`, plus one unfiltered catch-all (at least one real proceeding has no
type set and is invisible to every type filter). Every run does this full
pass; it's cheap (well under a minute).

**Finding documents**, two strategies:
- **Default** — fetch every proceeding's detail page
  (`/readingroom/proceeding/{number}/`), which lists every document ever
  filed under it, and download whatever isn't already recorded. Slower
  (one request per proceeding) but always complete.
- **`--since YYYY-MM-DD`** — query the site's `DocumentSearch` page directly
  for documents served on/after that date. Much cheaper for a small daily
  delta, and just as reliable at catching a new filing on an old, dormant
  proceeding, since the filter is on the *document's* serve date, not the
  *proceeding's* creation date.

A document's key is `{proceeding_number}_{document_number}` — permanent and
never reused, which makes the whole thing idempotent: a document already in
the database is never re-fetched, so re-running `sync` after any previous
run (complete, partial, or interrupted) only does what's actually left.

**Downloads and naming.** Each proceeding gets a folder at
`proceedings/{number}_{title}/`; each document is saved as
`{doc_number}_{description}.{ext}` inside it. Both names are computed once,
at first download, and never changed again, even if the site later edits
its title/description text. A document whose link 404s (a handful of the
site's own links are dead) is still recorded — with `source_url` set to the
literal string `file_unavailable` — so future runs don't retry it forever.

**The database** (`fmc.db`, plain SQLite — open it directly with `sqlite3`
or any client):
- `proceedings` — number (PK), title, permanent folder name, dates, type,
  closed status.
- `documents` — unique key, proceeding/document number, served date,
  description, source URL, permanent file name/path, content type, size.
- `sync_runs` — the run log (start/finish time, status, counts). Latest row
  = last run's outcome.

## Running it locally

```bash
go build -o bin/fmc-reading-room ./cmd/fmc-reading-room
```

Then, from the directory where `fmc.db` and `proceedings/` should live
(this repo's root, normally):

```bash
./bin/fmc-reading-room sync                       # download everything new
./bin/fmc-reading-room sync --since 2026-09-01    # fast path (see above)
./bin/fmc-reading-room sync --dry-run             # show what would happen, write nothing
./bin/fmc-reading-room status                     # last run's outcome + current totals
./bin/fmc-reading-room export                     # regenerate docs/data/documents.json for the search UI
./bin/fmc-reading-room verify                     # re-check every downloaded file is a real PDF, fix any that aren't
```

`--dry-run` and `--since` are the only flags exposed at runtime; everything
else (DB path, download folder, concurrency, rate limit, base URL) is a
constant at the top of `cmd/fmc-reading-room/main.go` — edit and rebuild for
a different value. `verify` exists because the site occasionally returns
HTTP 200 with a fake error page instead of a real 404; it re-checks every
file's magic bytes and marks any fakes `file_unavailable`.

## The hosted pieces

**Search UI** ([docs/](docs/), served by GitHub Pages): a static page that
loads `docs/data/documents.json` and filters/sorts/paginates client-side —
no backend. Each result links straight to the file via
`raw.githubusercontent.com` (works because the repo is public).

**Daily sync** ([.github/workflows/daily-sync.yml](.github/workflows/daily-sync.yml)):
runs at 06:00 UTC (and on demand from the Actions tab), and:
1. Checks out the repo *without* the ~6GB `proceedings/` folder — it uses a
   partial clone (`filter: blob:none`) plus sparse-checkout limited to the
   code, `docs/`, and root files like `fmc.db`, since `sync` only needs the
   database to know what's already downloaded, never the existing files
   themselves. This keeps every run's checkout small regardless of how big
   the archive gets.
2. Builds the CLI and runs `sync --since <3 days ago>` — the few days of
   overlap cheaply cover a missed or failed run (`sync` is idempotent, so
   overlap costs nothing but a few redundant existence checks).
3. Runs `export` to refresh `docs/data/documents.json`.
4. Commits and pushes anything new (new files, updated `fmc.db`, updated
   JSON) — which also triggers a fresh Pages deploy automatically.

## Keeping a local mirror in sync (Windows)

If you want a full local copy of the files and database for further
analysis — kept in sync automatically, without needing to run any commands
yourself day to day — [scripts/windows/](scripts/windows/) has what you
need. One-time setup:

1. **Install [Git for Windows](https://git-scm.com/download/win)** (default
   options are fine). This gives you a `git` command and "Git Bash", a
   terminal you can paste commands into.
2. **Clone the repo once.** Open Git Bash (Start menu → search "Git Bash"),
   navigate to where you want the copy to live (e.g. `cd Documents`), and
   run:
   ```bash
   git clone https://github.com/humzamo/fmc-reading-room.git
   ```
   This downloads everything — currently several GB, and it'll keep
   growing — so give it a few minutes on a normal connection.
3. **Double-click `scripts\windows\setup-daily-pull.bat`** inside the folder
   you just cloned. This registers a Windows Scheduled Task that pulls the
   latest changes automatically every day at 11:00 (your machine's local
   time) — no admin rights needed.

That's it — from then on, the folder updates itself daily. To check it's
actually working: open `pull.log` in the repo folder (each run appends a
timestamped line), or open the **Task Scheduler** app (Start menu → search
"Task Scheduler") and look for "FMC Reading Room Daily Pull" in the
library. Two things worth knowing: the pull only happens if the machine is
on and you're logged in at 11:00, and it needs an internet connection at
that moment.

## Assumptions and limitations

- **Proceeding/document numbers are permanent and never reused** — the
  basis for the idempotent, diff-based design.
- **Titles and descriptions aren't re-checked after first download.** If
  the site edits one later, the on-disk name doesn't follow.
- **Scraping a UI, not an API.** The site's search grids are paginated via
  full-page ASP.NET postbacks, reverse-engineered field-by-field. A
  significant site change could break discovery without much warning
  beyond a parsing/HTTP error.
- **`--since` trusts the site's own date index.** If that index is ever
  behind reality for some document, this tool would be too, until a full
  run catches it — which is why the default (non-`--since`) mode never
  trusts a date cutoff at all.
- **Git is the file store and the database's source of truth.** Everything
  — every PDF and `fmc.db` itself — lives in this git repo, kept in sync by
  the daily GitHub Actions commit. That's a deliberate shortcut for a
  zero-cost proof of concept, not something to keep doing in a real
  deployment: git has no business being a multi-GB (and growing) document
  store or a shared database. A production version of this would put the
  files in cloud object storage (e.g. S3/R2) and treat the SQLite file as
  disposable/rebuildable rather than as the durable copy.
- **Single local SQLite file, single writer** — not designed for concurrent
  `sync` runs against the same database.
- **A handful of documents genuinely 404 on the site itself** (pre-existing
  dead links, not a bug here) — recorded as unavailable rather than retried
  forever.
- **Politeness over raw speed.** Requests are rate-limited with a generic,
  descriptive User-Agent — this is a low-volume research tool, not built to
  maximize throughput against a public government service.
