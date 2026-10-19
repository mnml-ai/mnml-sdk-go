# Changelog

All notable changes to this module are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## 0.1.0 — unreleased

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
