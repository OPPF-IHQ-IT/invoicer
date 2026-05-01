# invoicer

A CLI tool for automating fraternity dues invoicing using [Airtable](https://airtable.com) as the member record source and [QuickBooks Online](https://quickbooks.intuit.com) as the accounting system.

Built for KRS (Keeper of Records and Seal) officers, but designed to be chapter-agnostic — all Airtable schema and QBO item mappings are configurable.

---

## Installation

### Homebrew (recommended for macOS)

```bash
brew install oppf-ihq-it/invoicer/invoicer
```

### Download a release binary

Download the latest binary for your platform from the [Releases](https://github.com/OPPF-IHQ-IT/invoicer/releases) page and place it on your `PATH`.

```bash
# macOS (Apple Silicon)
curl -L https://github.com/OPPF-IHQ-IT/invoicer/releases/latest/download/invoicer_darwin_arm64.tar.gz | tar xz
mv invoicer /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/OPPF-IHQ-IT/invoicer/releases/latest/download/invoicer_darwin_amd64.tar.gz | tar xz
mv invoicer /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/OPPF-IHQ-IT/invoicer/releases/latest/download/invoicer_linux_amd64.tar.gz | tar xz
mv invoicer /usr/local/bin/
```

### Build from source

Requires Go 1.22+.

```bash
go install github.com/OPPF-IHQ-IT/invoicer/cmd/invoicer@latest
```

---

## Getting started

### 1. Run the setup wizard

```bash
invoicer setup
```

This walks you through entering your QBO credentials and Airtable details, then writes `~/.config/invoicer/config.yaml`. On a fresh install with no config, running `invoicer` with no arguments launches the wizard automatically.

### 2. Authenticate with QuickBooks Online

```bash
invoicer auth login --env production
```

Opens your browser for the Intuit OAuth flow and saves a token to `~/.config/invoicer/qbo-token.json`.

### 3. Map your QBO item IDs

```bash
invoicer setup items
```

Fetches your QBO product list and walks through selecting the right product for each dues component interactively. Writes the IDs directly to your config — no copy-pasting required.

### 4. Reconcile QBO customers against Airtable members

```bash
invoicer customers reconcile
```

Matches Airtable members to QBO customers by control number (stored in the QBO Notes field) or email. Once everyone is matched, run with `--update-airtable --no-dry-run` to write Customer IDs back to Airtable.

### 5. Preview what would be invoiced

```bash
invoicer preview --fiscal-year FY2026
```

---

## Command reference

### `invoicer setup`

Interactive wizard to create `~/.config/invoicer/config.yaml`.

```bash
invoicer setup
```

#### `invoicer setup items`

Interactively map QBO products to dues components and write the IDs to config. Requires auth to be completed first.

```bash
invoicer setup items
```

---

### `invoicer auth`

#### `invoicer auth login`

Authenticate with QuickBooks Online via OAuth. Opens your browser and saves a token to `~/.config/invoicer/qbo-token.json`.

```bash
invoicer auth login --env production   # production (default)
invoicer auth login --env sandbox      # sandbox / testing
```

#### `invoicer auth status`

Show current authentication status and token expiry.

```bash
invoicer auth status
```

#### `invoicer auth logout`

Remove stored QBO credentials.

```bash
invoicer auth logout
```

---

### `invoicer customers`

#### `invoicer customers reconcile`

Match Airtable members to QBO customers and optionally write Customer IDs back to Airtable. Dry run is on by default.

```bash
# Preview matches (dry run)
invoicer customers reconcile

# Write matched IDs to Airtable
invoicer customers reconcile --update-airtable --no-dry-run

# With output CSVs for review
invoicer customers reconcile --update-airtable --no-dry-run \
  --matched-out matched.csv \
  --ambiguous-out ambiguous.csv \
  --unmatched-out unmatched.csv \
  --skipped-out skipped.csv

# Re-run and overwrite already-mapped members
invoicer customers reconcile --update-airtable --no-dry-run --overwrite
```

Members in the **ambiguous** file have multiple possible QBO matches and need manual resolution. Members in **unmatched** have no QBO customer record and may need one created. Members in **skipped** were excluded (no email address, or marked "No Longer in Chi Tau").

---

### `invoicer preview`

Preview what invoicer would create or send without making any changes.

```bash
# Preview all invoices for a fiscal year
invoicer preview --fiscal-year FY2026

# Override today's date (e.g. to test late fee logic)
invoicer preview --fiscal-year FY2026 --as-of 2026-01-15

# Preview which invoices would be sent (email status check)
invoicer preview --fiscal-year FY2026 --send

# JSON output
invoicer preview --fiscal-year FY2026 --format json

# Write report to a file
invoicer preview --fiscal-year FY2026 --out preview.json

# Override QBO environment for this run
invoicer preview --fiscal-year FY2026 --env sandbox
```

---

### `invoicer run`

Execute invoice creation or sending. Exactly one of `--create-only` or `--send` is required.

```bash
# Create invoices in QBO (does not send them — review in QBO first)
invoicer run --fiscal-year FY2026 --create-only

# Send previously-created invoices
invoicer run --fiscal-year FY2026 --send

# Override QBO environment for this run
invoicer run --fiscal-year FY2026 --create-only --env sandbox
```

---

### `invoicer config`

#### `invoicer config validate`

Validate your config file and report any missing or misconfigured values.

```bash
invoicer config validate
```

---

### `invoicer qbo`

#### `invoicer qbo doctor`

List all active QBO products and their IDs. Useful for identifying item IDs to put in your config.

```bash
invoicer qbo doctor
```

---

### `invoicer airtable`

Airtable diagnostics. Run `invoicer airtable --help` for available subcommands.

---

## Fiscal year format

Fiscal years are specified as `FY2026` or `2026`.

| CLI | Period | Airtable label |
|---|---|---|
| `FY2026` | 2025-11-01 to 2026-10-31 | 2025-2026 |
| `FY2027` | 2026-11-01 to 2027-10-31 | 2026-2027 |

Late fees apply on or after January 1 of the fiscal year.

---

## Configuration

The config file lives at `~/.config/invoicer/config.yaml` and is created by `invoicer setup`. See `config.example.yaml` in this repo for the full reference with comments.

**Secrets** (API keys, QBO client ID/secret) can be stored directly in the config file or via environment variables:

| Config key | Environment variable |
|---|---|
| `airtable.api_key` | `AIRTABLE_API_KEY` |
| `qbo.client_id` | `QBO_CLIENT_ID` |
| `qbo.client_secret` | `QBO_CLIENT_SECRET` |

All Airtable table names, field names, and QBO item IDs are configurable so any chapter can adapt `invoicer` to their own schema without code changes.

---

## Token storage

```
~/.config/invoicer/
  config.yaml       # your config (chmod 600, never committed)
  qbo-token.json    # OAuth token (chmod 600, never committed)
```

---

## Support

If `invoicer` saves you time and you'd like to show some love, consider supporting the author:

[![Support on Patreon](https://img.shields.io/badge/Patreon-Support-orange?logo=patreon)](https://www.patreon.com/wmadisondev)

---

## License

MIT
