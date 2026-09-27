CREATE TABLE IF NOT EXISTS proceedings (
    proceeding_number TEXT PRIMARY KEY,
    title             TEXT NOT NULL,
    folder_name       TEXT NOT NULL,
    created_date      TEXT,
    last_updated      TEXT,
    proceeding_type   TEXT,
    is_closed         INTEGER NOT NULL DEFAULT 0,
    last_synced_at    TEXT
);

CREATE TABLE IF NOT EXISTS documents (
    unique_key         TEXT PRIMARY KEY,
    proceeding_number  TEXT NOT NULL REFERENCES proceedings(proceeding_number),
    document_number    INTEGER NOT NULL,
    served_date        TEXT,
    description        TEXT,
    source_url         TEXT NOT NULL,
    file_name          TEXT NOT NULL,
    file_path          TEXT NOT NULL,
    content_type       TEXT,
    file_size          INTEGER,
    downloaded_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_proceeding_number ON documents(proceeding_number);

CREATE TABLE IF NOT EXISTS sync_runs (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at           TEXT NOT NULL,
    finished_at          TEXT,
    status               TEXT NOT NULL,
    proceedings_scanned  INTEGER NOT NULL DEFAULT 0,
    new_proceedings      INTEGER NOT NULL DEFAULT 0,
    new_documents        INTEGER NOT NULL DEFAULT 0,
    documents_downloaded INTEGER NOT NULL DEFAULT 0,
    documents_failed     INTEGER NOT NULL DEFAULT 0,
    error                TEXT
);
