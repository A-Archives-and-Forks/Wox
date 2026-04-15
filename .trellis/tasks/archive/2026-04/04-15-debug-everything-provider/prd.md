# Debug Everything Provider Connection Failure

## Goal
Diagnose why the file search plugin cannot surface Everything results after the system provider was enabled, and fix the provider integration so Wox either returns Everything results or reports the real connection failure.

## Requirements
- Confirm whether Everything results are dropped during provider execution, aggregation, or UI query handling.
- Preserve the current merged file search behavior for local index plus system providers.
- Replace misleading "everything is not running" errors with actionable diagnostics when the SDK connection fails for another reason.
- Keep the fix within the Windows-specific Everything integration and file search flow.

## Acceptance Criteria
- [ ] Wox no longer misclassifies generic Everything SDK failures as "everything is not running".
- [ ] Logs include enough context to distinguish connection, IPC, and search execution failures.
- [ ] If Everything is reachable, provider results can flow into the file search aggregator unchanged.
- [ ] Targeted verification covers the failing path introduced by this bugfix.

## Technical Notes
- This is a Windows-specific backend fix in `wox.core/util/filesearch`.
- The current logs show `providers=2`, so the provider is registered and invoked.
- The current failure happens before aggregation, during Everything provider execution.
