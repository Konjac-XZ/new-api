# Default UI Migration Kanban

This board tracks migration of local Classic UI customizations into the new
Default UI. Keep implementation style aligned with `web/default`.

## Done

- [x] Create migration branch: `migrate-classic-custom-to-default-ui`.
- [x] Add Default UI Monitor route at `/monitor` for admin users.
- [x] Add Monitor sidebar entry under General for admin users.
- [x] Port Monitor websocket summary handling with live `channel_update` retry
      count refresh.
- [x] Port Monitor model-name client filter.
- [x] Port Monitor summary metrics: status, model, channel, retry badge,
      duration, TTFT, throughput, and input/output token bubble.
- [x] Port Monitor request detail fetch, body tabs, and request interruption.
- [x] Port Monitor fullscreen mode with screen wake-lock support.
- [x] Validate Default UI build for this increment.

## In Progress

- [x] Audit current Default UI channel editor against Classic custom fields and
      backend channel settings.

## Pending

- [x] Channel table multi-rule sorting UI and request parameters.
- [x] Channel editable remark clear/persist behavior.
- [x] Channel breaker status card/dialog and inactive-channel badge behavior.
- [x] Channel edit-mode clipboard quick-paste parity.
- [x] Channel external configuration fields and advanced settings parity.
- [x] Default UI polish for any Classic custom filters in logs, models, tokens,
      users, and redemption tables.
- [x] Final end-to-end audit against `upstream/main...HEAD` classic/default diff.
- [x] Final validation: relevant Go tests, Default UI lint/type/build, and diff
      hygiene.

## Notes

- Do not add or update locale files outside Simplified Chinese, Traditional
  Chinese, and English for this migration task.
- Prefer recovering behavior from git history and existing Classic code before
  designing a new interaction.
