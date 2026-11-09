# Changelog

All notable changes to this module are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## 0.2.0 — unreleased

Amounts in US dollars. API v2 now answers every amount in dollars beside the credits; the SDK
has the dollar fields and steers you to them. Nothing is removed, so 0.1.0 code still builds.

- New fields, each a `float64` of US dollars to four decimals: `ChargedUSD` on `RenderStarted`
  and `JobStarted`; `ChargedUSD` and `RefundedUSD` on `Job`; `RefundedUSD` on `JobCanceled`;
  `BalanceUSD` on `Account`, `DailyLimitUSD` on `AccountKey` and `DailyUSD` on `AccountLimits`
  (both `*float64`, nil for no limit); `RenderUSD` and `EditUSD` on `EnginePrices`; `USD` on
  `VideoPrice`; `BalanceUSD` and `ThresholdUSD` on the `credits.low` webhook's `CreditsLow`.
- Deprecated (a `Deprecated:` paragraph, removed in a later release): `CreditsCharged`,
  `CreditsRefunded`, `Account.Credits` (`AccountCredits`), `DailyCreditLimit`, `DailyCredits`,
  `EnginePrices.Render` and `Edit`, `VideoPrice.Credits`, and `CreditsLow.Balance` and
  `Threshold`. Each names the dollar field to read instead.
- `spec/openapi.json` refreshed (Nov 9 2026). A new test checks that the SDK deprecates exactly
  the answer fields the spec does.
- `Version` is `0.2.0`; README reads the dollar fields.

## 0.1.0

First release.

- Every endpoint of API v2: `Renders`, `Edits`, `Enhancements`, `Videos`, `Jobs` (Get, Cancel,
  Wait, WaitAll, Stream), `Files.Download`, `Account`, `Engines`.
- `CreateAndWait` on every create: a render waits in the held answer (`wait=90`) and reads on
  only for what is still running.
- `Jobs.Stream` as an `iter.Seq2[*JobStreamEvent, error]`, for a `range` loop.
- Retries for a 429, a 5xx or a dropped connection, after `Retry-After` (up to 60 s) or a
  backoff, with one idempotency key per call kept across them.
- `VerifyWebhook` and `SignWebhook` (Standard Webhooks, HMAC-SHA256).
- No dependencies outside the standard library. Go 1.23 or newer. The tests check every
  request and answer field against the API's OpenAPI document.
