# Plugin Troubleshooting Design

Date: 2026-11-12. Scope: Pico Connect and Steam Link device availability guidance.

## Intent

When an enabled plugin process is running but its device reports `error` or `disconnected`, show a help button beside the warning in both Overview and Plugins. The button opens a keyboard-accessible modal with troubleshooting steps authored by that plugin. Text follows the application's current English or Simplified Chinese language preference, including changes while the modal is open.

## Data contract

`manifest.json` may include `troubleshooting` with `en-US` and `zh-CN` arrays of plain-text steps. Both locales are required together; each has one to eight nonblank steps of at most 512 UTF-8 bytes. The existing 64 KiB manifest limit still applies. An absent field means no help button. The Host copies validated content into its runtime snapshot, and the Wails plugin DTO exposes only these bounded strings. The frontend selects the current locale and renders text, not HTML or links. No plugin process protocol change is needed.

Pico Connect's steps cover starting the app, setting transfer protocol 2, enabling headset tracking, and resolving the loopback port conflict. Steam Link's steps cover enabling tracking and OSC output, matching port 9015 by default, and checking that Steam Link is connected. The guide describes setup and recovery; it does not claim that all hardware fields are verified.

## Behavior and acceptance

The help button appears only for an unavailable device with a guide. It appears in the Overview row's warning area and the Plugins card's device warning area. Both buttons open the same localized guide, restore focus on close, and update on language change. If the device recovers while the dialog is open, focus moves to its persistent card or overview row. Existing plugins without guidance remain valid and keep their current UI. Tests cover manifest validation, ownership across snapshots and DTOs, locale selection, both button locations, and modal content/focus. Relevant Go race tests, frontend checks/tests, and desktop staging must pass.
