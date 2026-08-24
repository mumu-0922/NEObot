# Runtime symptoms

- Screenshot 1: `New Chat` has an active spinner; composer still contains
  `洛阳天气预报`; no accepted user bubble is visible.
- Screenshot 2: an older turn requesting seven days of gold prices and an XLSX
  shows a downloadable XLSX attachment and then a generic
  `Server generation failed` block.
- The immediately preceding release recreated both Backend and Frontend. The
  old Backend image is retained locally, so rollback remains available.
- Investigation must distinguish a code regression from an in-flight Run that
  was interrupted by that recreation.

## Confirmed runtime evidence

- The weather Run was accepted with HTTP 201 and its assistant stream completed
  with HTTP 200 after 52.8 seconds. The apparent send failure is a Frontend
  submission-lock lifetime bug, not Backend rejection.
- The older XLSX assistant is durably `failed` with
  `AGENT_VERIFICATION_REQUIRED`, has one retained attachment, and has no stored
  error message. Reload normalization therefore falls back to the misleading
  generic `Server generation failed` string.
- Sanitized Agent events show successful rounds:
  `search_web`, `search_web`, four foreground `terminal` calls, another
  `search_web`, and finally `publish_file` on round 8. No round remained for a
  later read, `verify_completion`, or final narration.
- The repair must preserve the evidence gate. A bounded verification grace
  after the ordinary round limit is safe because any still-unverified mutation
  continues to fail closed when the grace budget expires.
