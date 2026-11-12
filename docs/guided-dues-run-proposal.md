# Proposal: guided dues run

**Status:** built November 2026. See "Running dues each fiscal year" in the README for how to use it. The sections below are the original design. Differences from it:

- Added `--dry-run`, and `--all-members` for practicing before statuses are reset.
- The binary age check, and the check for a QBO customer that's inactive, weren't built as named checks. An inactive customer shows up as a skipped member.
- A second, separate recompute lives in `internal/duesrun`. It's dollars only and checked against the calculator across every flag combination.
- The review file is a CSV.
- **MSP Prepaid Through** is added, and it's inclusive. Once it passes, the offset stops automatically and a real create unchecks `Recent MSP?`. Pre-flight stays read-only. Members with a blank value are flagged.
- The real create and send stages haven't run against live data. Tests use fakes, and the live runs so far were read-only.

## Why

The FY2027 run worked, but only because the KRS and an engineer caught problems by hand. The hard parts weren't the invoicing:

- The login silently defaulted to sandbox, and the old token had expired.
- Secrets were missing, and an old binary was easy to run by mistake.
- Recent MSP expiry lived in people's heads.
- QBO and Airtable disagreed on emails, and invoices go to QBO's address.
- A send preview total looked wrong and stopped the run.
- A line-by-line audit took hours of custom scripts.
- Bounces kept arriving for hours, and bounced invoices still read as sent.

The next KRS may not be technical. This proposal moves that knowledge into the tool, so a KRS reviews and approves the numbers while the technology chair handles setup.

We're not proposing a web interface. It would mean hosting QBO and Airtable credentials and maintaining an app year-round for something used once a year. We can revisit that after FY2028.

## Who does what

| Technology chair | KRS |
|---|---|
| Installs the tool and keeps secrets | Keeps Airtable member data current |
| Logs in to QBO | Reviews the spreadsheet and resolves flags |
| Starts the run and types the approvals the KRS gives | Approves create and send |
| Handles errors | Follows up on bounces and member questions |

## The command

```
invoicer dues-run -f FY2028
```

It walks through the run in plain English, one stage at a time. It can be stopped and restarted at any point, and it works out where it left off from QBO and Airtable.

| Stage | What happens | Writes anything? |
|---|---|---|
| 1. Pre-flight | Runs every check below | No |
| 2. Review | Writes the review spreadsheet, and waits for flags to be resolved | No |
| 3. Create | Creates invoices and $0 receipts | QBO and Airtable; emails no one |
| 4. Verify create | Reads every document back from QBO and compares it to what was reviewed | No |
| 5. Send | Emails invoices and receipts | Emails members |
| 6. Verify send | Confirms every document shows as sent | No |
| 7. Follow-up | `invoicer dues-run status` reports bounces and what's still open | No |

### Safety rules

- **Approvals are typed.** Create asks you to type `create 191 + 34`, using the real counts. Send works the same way. A stray Enter does nothing.
- **Approve exactly what was reviewed.** The review saves a fingerprint of every member's lines. If Airtable or QBO changes before create, the run stops and shows what changed.
- **Production is explicit.** The run shows the environment and company name, and refuses to continue if the login doesn't match the config.
- **Send waits for its date.** Create can run early, but send warns if it's before the invoice date.
- **Nothing runs in the background of a stage.** A stage either finishes and verifies, or says exactly what's incomplete.

## Pre-flight checks

**Blockers** stop the run. **Warnings** go on the review spreadsheet for a person to decide.

### Setup

| Check | Level |
|---|---|
| Config and secrets are present | Blocker |
| QBO login works, and it's production | Blocker |
| The binary is current with the latest release | Warning |
| Every QBO item ID exists and is active | Blocker |
| Terms ID is valid | Blocker |

### Dues data

| Check | Level |
|---|---|
| Dues Schedule has every level for the year | Blocker |
| The year's rates differ from last year's, or are confirmed unchanged | Warning |
| Credit rows link to exactly one member | Warning |
| Available credits equal earned minus spent | Warning |

### Members

| Check | Level |
|---|---|
| No email and no QBO customer | Warning |
| QBO and Airtable emails differ | Warning |
| Email has stray spaces | Warning |
| QBO customer is missing or inactive | Warning |
| Member and QBO names look like different people | Warning |
| Two members share a QBO customer | Blocker |
| Possible duplicate QBO customers | Warning |
| Recent MSP period has ended, or end year is blank | Warning |
| No dues record in QBO for last year | Warning |
| Poll worker credit can't be applied because local dues are $0 | Warning |

### Compared with last year

Each member's plan is compared with their last dues document. Any of these gets flagged:

| Check | Level |
|---|---|
| Life or Basileus Emeritus status gained | Warning |
| MSP or undergraduate rate gained or lost | Warning |
| Retiree rate gained or lost | Warning |
| At Large gained or lost | Warning |
| Bill changed by more than the rate increase | Warning |

### QBO balances

| Check | Level |
|---|---|
| Already has a document for this fiscal year | Blocker for that member |
| Unapplied credit in QBO with no Member Credits row | Warning |
| Payment in the last 90 days that looks like a prepayment | Warning |
| Open balance from earlier years | Warning |

## The review spreadsheet

One row per member, sorted with flagged members first:

| Column | Example |
|---|---|
| Member and control number | |
| Document | Invoice or $0 receipt |
| Total, and last year's total | |
| Line items | In plain words |
| Credits applied | |
| Email it will go to | QBO's address |
| Flags, and why each matters | "MSP ended: going from $0 to full dues" |
| Decision | Filled in by the KRS |

A short summary sits on top: counts, total billed, and members left out, by status. It contains member data, so it's written outside the repo.

Every total is also recomputed separately from the billing code, the way the FY2027 audit did it, and any difference blocks the run.

## Airtable change

Add **MSP Prepaid Through** to Members, as a fiscal year such as `FY2026`. It replaces memory and the manual `Recent MSP?` box:

- Pre-flight warns when the year has passed or the field is blank.
- `Recent MSP?` can become a formula once the field has been filled in for current MSPs.

The field needs filling in once from initiation records.

## Plan

| Phase | Scope |
|---|---|
| 1. Pre-flight | `invoicer preflight`, read-only. Useful on its own the week before |
| 2. Review spreadsheet | Year-over-year comparison and the separate recompute |
| 3. Verify and status | Read-back after create and send, and the bounce report |
| 4. Guided command | `dues-run` ties the stages together with typed approvals |

Each phase is useful alone. Phases 1 and 2 hold most of the value, so the run can still use today's commands if phase 4 slips.

**Before any of this,** test the new $0 receipt code on one real member. It has passed tests and previews, but it has never created a live receipt.

## Questions to settle

1. What counts as a first-time billing issue: should someone with no dues last year be `Invoicable` or `Reclaimable`?
2. Should a poll worker credit apply to any dues when local dues are already $0? It stays banked today.
3. Who approves if the KRS and technology chair disagree on a flag?
4. Should pre-flight run automatically in early October, or be started by hand?
