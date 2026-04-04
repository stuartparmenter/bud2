---
name: vm-control
description: "Observe and control the Sandmill Mac OS 8 emulator. Use when debugging emulator state, running interactive VM sessions, or automating Mac OS 8 interactions. Triggers on: vm session, emulator, mac os 8, netscape, sandmill vm."
user-invocable: true
---

You are running an interactive VM control session for the Sandmill Mac OS 8 emulator.

Read the full guide before taking any actions:

```
state/system/guides/vm-control.md
```

## Session Protocol

1. **Start the server** — call `vm_start` first. It's a singleton; safe to call if already running.
2. **Orient** — take a screenshot with `vm_screenshot` to see current emulator state.
3. **Act** — use MCP tools for interactions. Check the guide for coordinate space, gotchas, and what doesn't work.
4. **Report** — after each significant action, describe what you see and what changed.

## Key Reminders

- Canvas coordinates are 640×480. The guide has a coordinate reference table.
- OCR is unreliable on small bitmap fonts — use coordinates for navigation when possible.
- AppleScript is NOT available (Classic Mac, no OSA).
- `vm_click_text`, `vm_launch_app`, `vm_open_menu` exist but may fail silently on small text — verify with a screenshot after.
- If the emulator shows a dialog, dismiss it before doing anything else.

Proceed step by step. Take a screenshot after each non-trivial action to confirm the result.
