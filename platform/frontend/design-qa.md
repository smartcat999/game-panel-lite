# Design QA

## Visual source

- Instance list: `codex-clipboard-8c37259b-9d6f-44cf-a971-2641484493e7.png`
- Instance detail: `codex-clipboard-b47a2a5b-4c05-4706-b483-0dfa8ce9953b.png`
- Direction: compact light operations console, rounded low-shadow panels, near-black primary actions, restrained green status, monospace operational data.

## Comparison

The reference and implementation were rendered together in the Codex in-app browser for both the instance list and instance configuration state. The browser panel was also checked at its narrow responsive width.

Intentional product differences from the visual source:

- Removed the Pro badge, dashboard, navigation counts, Worlds & Saves, global Mod Workshop, player and ping columns, and all duplicated instance summaries.
- Replaced plan-based deployment with Region + resource specification + hourly CNY quote.
- Public address, port, and transport are read-only system allocation results.
- Mods appear only when the selected Provider Manifest declares the capability.

## Checks

- [x] No gradient, glass, neon, decorative copy, oversized title block, or repeated summary.
- [x] Instance list retains useful operational density and scrolls horizontally at narrow widths.
- [x] Tabs do not wrap vertically on mobile; the tab strip scrolls.
- [x] Login, invitation, create wizard, conditional mods, quote, deployment operation, instance lifecycle, logs, console, backup, restore, billing, and promotional credit controls are connected.
- [x] Forbidden, failed, stale, unsupported, and insufficient-credit states are renderable through fixture state.
- [x] Provider fields render through a controlled type switch; unsupported field types are absent from the allowlist.
- [x] No public port, protocol, bandwidth, plan, order, payment, entitlement, or World control appears.
- [x] The production build, lint, and typecheck complete without errors.
- [x] The deployment-operation console warning found during interaction testing was fixed and retested without an issue badge.

## Final result

passed
