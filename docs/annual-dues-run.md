# Annual dues run

The runbook for billing a fiscal year's dues. The fiscal year starts **November 1**.
Invoices are dated November 1 and due on receipt. Late fees apply from **January 1**.

Examples use FY2027 (November 1, 2026 to October 31, 2027). Change `-f` and `--as-of` for other years.

## Setting up

The technology chair can do this part for the KRS. Run everything from the repo root.

1. **Build a fresh binary** and always use `./invoicer`. An old binary is missing later fixes.
   ```
   go build -o ./invoicer ./cmd/invoicer
   ```
2. **Load the secrets.** The config reads `QBO_CLIENT_ID`, `QBO_CLIENT_SECRET` and `AIRTABLE_API_KEY` from the
   environment. Keep them in `~/.config/invoicer/env` (`chmod 600`, never in the repo) and load them in each new shell:
   ```
   set -a; . ~/.config/invoicer/env; set +a
   ./invoicer config validate
   ```
   `config validate` is the only command that checks these up front, so run it first.

## The week before

1. **Authenticate.** Access tokens refresh on their own, but the refresh token dies after about 100 days unused,
   which is always the case after a year. Plan to log in again.
   ```
   ./invoicer auth login --env production
   ./invoicer auth status
   ```
   Always pass `--env production`. Without it, login goes to sandbox, whatever the config says.
2. **Dues Schedule.** Airtable needs a row per level for the fiscal year (e.g. `2026-2027`) or an `All` row.
   Fiscal-year rows win over `All` rows.
3. **Member statuses.** Only `Invoicable` and `Reclaimable` members are billed. Move every brother who
   should get an invoice to one of those statuses. Life members are included too; they get a $0 sales receipt.
4. **Customers.** Every billed member needs a linked QBO customer with an email.
   ```
   ./invoicer customers reconcile --dry-run
   ```
   Matching uses the control number and email only. Before creating a customer, search QBO by name to avoid
   duplicates. Then link the customer ID in Airtable.
5. **Credits.** Bring the poll worker credits and the Member Credits table up to date. The invoicer applies
   whatever is available on run day.
6. **Preview.**
   ```
   ./invoicer preview -f FY2027 --as-of 2026-11-01
   ```
   Check the totals and the skipped list. `missing_email` and customer skips mean that member won't be invoiced.
7. **Check what the math can't.** Totals only reflect Airtable, so also confirm:
   - **Recent MSP.** The offset is prepaid for three years, and nothing tracks when that ends. Airtable has no
     initiation date, so check each `Recent MSP?` box against the initiation records. A Fall 2023 brother's last
     prepaid year was FY2026. Chapter initiates go from $0 to full dues when it ends, so warn them.
   - **Emails.** Invoices go to the QBO customer's email, not Airtable's. Fix differences in QBO before create.
     Mailchimp shows which address a brother actually uses.
   - **New names.** Anyone with no dues history in QBO is getting their first invoice from us. Confirm they
     should be `Invoicable` rather than `Reclaimable`.
   - **Everyone left out.** `At Large`, `Pending` and remitted statuses are not billed.

## Run day

1. **Authenticate** (`./invoicer auth login --env production`).
2. **Preview again** to catch any changes since last week.
   Dates come from `--as-of`, so create can run before midnight. Hold send until November 1: QBO emails right away.
3. **Create** the invoices. This does not email anyone. Members who owe $0 get a sales receipt instead of an invoice.
   ```
   ./invoicer run -f FY2027 --create-only --as-of 2026-11-01 | tee fy2027-create.json
   ```
   Creating each invoice also:
   - moves the member to `Invoiced`, or to `Active` if they owe $0;
   - adds what was applied to the poll worker and Member Credits `Spent` columns.
4. **Spot-check in QBO.** Check a standard, a retiree, a life member ($0 receipt), and anyone with credits.
   - Fix a wrong invoice by editing it in QBO *before* sending.
   - Do not rerun create for him. He's now `Invoiced`, so a rerun skips him.
5. **Preview the send.**
   ```
   ./invoicer preview -f FY2027 --send --as-of 2026-11-01
   ```
6. **Send.**
   ```
   ./invoicer run -f FY2027 --send --as-of 2026-11-01 | tee fy2027-send.json
   ```
   Send finds each member's invoice or $0 receipt by the Airtable record ID in its private note. It skips
   anything already emailed or paid. The Sales form copies (KF, KRS) go out with receipts too.
7. **Check for bounces.** Look in QBO later that day and the next. A bounced invoice still shows as sent, so a
   rerun skips it. Get a good address, fix it on the customer and the invoice, and resend from QBO.

## Skip reasons

| Reason | Meaning / fix |
|---|---|
| `missing_email` | No QBO customer or email for the member. Link or create a customer, then rerun create. |
| `no_invoice_to_send` | Send found no invoice for this fiscal year. Run create first. |
| `existing_invoice_already_sent` / `existing_invoice_paid` | Nothing to do. |
| `duplicate_invoice_conflict` | More than one invoice for the member this year. Void the extra in QBO. |
| `duplicate_receipt_conflict` | More than one $0 receipt for the member this year. Void the extra in QBO. |
| `qbo_item_mapping_missing` | A `qbo_items` ID is blank in the config for a line this member needs. |
| `dues_schedule_missing` / `dues_schedule_conflict` | Fix the Dues Schedule rows for the fiscal year. |

## Rerunning

- Rerunning create is safe. Members already invoiced are `Invoiced`, so a rerun only picks up members
  still in `Invoicable` / `Reclaimable`, such as ones that were skipped or errored.
- Rerunning send is safe. Invoices and receipts that have already been emailed are skipped.
- If create reports `Airtable status update failed` for a member, set him to `Invoiced` by hand before rerunning.
  Otherwise a rerun can create a second invoice for him.
