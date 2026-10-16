# Annual dues run

The runbook for billing a fiscal year's dues. The fiscal year starts **November 1**.
Invoices are dated November 1 and due on receipt. Late fees apply from **January 1**.

Examples use FY2027 (November 1, 2026 to October 31, 2027). Change `-f` and `--as-of` for other years.

## The week before

1. **Authenticate.** QBO tokens expire often.
   ```
   ./invoicer auth status
   ./invoicer auth login
   ```
2. **Dues Schedule.** Airtable needs a row per level for the fiscal year (e.g. `2026-2027`) or an `All` row.
   Fiscal-year rows win over `All` rows.
3. **Member statuses.** Only `Invoicable` and `Reclaimable` members are billed. Move every brother who
   should get an invoice to one of those statuses. Life members are included too; their invoice is $0.
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

## Run day

1. **Authenticate** (`./invoicer auth login`).
2. **Preview again** to catch any changes since last week.
3. **Create** the invoices. This does not email anyone.
   ```
   ./invoicer run -f FY2027 --create-only --as-of 2026-11-01 | tee fy2027-create.json
   ```
   Creating each invoice also:
   - moves the member to `Invoiced`, or to `Active` if his total is $0;
   - adds what was applied to the poll worker and Member Credits `Spent` columns.
4. **Spot-check in QBO.** Check a standard, a retiree, a life member ($0), and anyone with credits.
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
   Send finds each member's invoice by the Airtable record ID in its private note. It skips invoices that
   have already been emailed or paid, and $0 invoices.

## Skip reasons

| Reason | Meaning / fix |
|---|---|
| `missing_email` | No QBO customer or email for the member. Link or create a customer, then rerun create. |
| `no_invoice_to_send` | Send found no invoice for this fiscal year. Run create first. |
| `existing_invoice_already_sent` / `existing_invoice_paid` | Nothing to do. |
| `duplicate_invoice_conflict` | More than one invoice for the member this year. Void the extra in QBO. |
| `qbo_item_mapping_missing` | A `qbo_items` ID is blank in the config for a line this member needs. |
| `dues_schedule_missing` / `dues_schedule_conflict` | Fix the Dues Schedule rows for the fiscal year. |

## Rerunning

- Rerunning create is safe. Members already invoiced are `Invoiced`, so a rerun only picks up members
  still in `Invoicable` / `Reclaimable`, such as ones that were skipped or errored.
- Rerunning send is safe. Invoices that have already been emailed are skipped.
- If create reports `Airtable status update failed` for a member, set him to `Invoiced` by hand before rerunning.
  Otherwise a rerun can create a second invoice for him.
