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

## Setup

### 1. Run the setup wizard

```bash
invoicer setup
```

This walks you through entering your QBO credentials and Airtable details, then writes `~/.config/invoicer/config.yaml` for you.

### 2. Authenticate with QuickBooks Online

```bash
invoicer auth login --env production
```

This opens your browser for the QBO OAuth flow and saves a token to `~/.config/invoicer/qbo-token.json`.

### 3. Map your QBO item IDs

```bash
invoicer qbo doctor
```

This lists all products in your QBO account. Copy the IDs for each dues component into the `qbo_items` section of `~/.config/invoicer/config.yaml`.

---

## Usage

### Reconcile QBO customers against Airtable members

Pulls customers directly from QBO and matches them to Airtable members by control number (stored in the QBO Notes field) or email.

```bash
# Dry run — preview matches without making changes
invoicer customers reconcile

# Write matched QBO Customer IDs back to Airtable
invoicer customers reconcile --update-airtable --no-dry-run \
  --matched-out matched.csv \
  --ambiguous-out ambiguous.csv \
  --unmatched-out unmatched.csv \
  --skipped-out skipped.csv
```

Members flagged as ambiguous (multiple QBO matches) or unmatched (no QBO customer found) will need to be resolved manually before invoicing.

### Preview what would be invoiced

```bash
invoicer preview --fiscal-year FY2026
invoicer preview --fiscal-year FY2026 --as-of 2026-01-15
invoicer preview --fiscal-year FY2026 --format json
invoicer preview --fiscal-year FY2026 --send   # preview which invoices would be sent
```

### Create invoices

```bash
# Create invoices in QBO — does not send them
invoicer run --fiscal-year FY2026 --create-only

# After reviewing invoices in QBO, send them
invoicer run --fiscal-year FY2026 --send
```

### Validate config

```bash
invoicer config validate
```

---

## Fiscal Year Format

Fiscal years are specified as `FY2026` or `2026`.

| CLI | Period | Airtable Label |
|---|---|---|
| `FY2026` | 2025-11-01 to 2026-10-31 | 2025-2026 |
| `FY2027` | 2026-11-01 to 2027-10-31 | 2026-2027 |

Late fees apply on or after January 1 of the fiscal year.

---

## Configuration

Copy `config.example.yaml` to `~/.config/invoicer/config.yaml` and customize.

**Secrets must never be committed.** Use environment variables:

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
  config.yaml       # your config (not committed)
  qbo-token.json    # OAuth token (not committed, 0600 permissions)
```

---

## Support

If `invoicer` saves you time and you'd like to show some love, consider supporting the author:

[![Support on Patreon](https://img.shields.io/badge/Patreon-Support-orange?logo=patreon)](https://www.patreon.com/wmadisondev)

---

## License

MIT
