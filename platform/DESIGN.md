# GamePanel Platform Design System

This document governs the new `platform/frontend` implementation. The legacy frontend is not a visual reference.

## Product character

GamePanel is a calm, precise operations product for game infrastructure. It should feel trustworthy under load, readable during incidents, and approachable to a first-time server owner. Use game artwork only where it helps identify a game or instance.

## Foundations

- Use semantic design tokens for color, type, spacing, radius, elevation, motion, and data visualization.
- Support light, dark, and system themes from the first shell implementation.
- Store theme and locale as User Preferences; use a local bootstrap value only to prevent first-paint flicker.
- Use one language per rendered session. All visible copy, accessibility labels, validation messages, dates, and numbers use message catalogs and locale formatters.
- Start with a neutral sans-serif interface and a monospace face only for identifiers, endpoints, logs, and measurements.
- Use one restrained brand accent. Status colors communicate state and are not decoration.

## Layout

- Desktop uses a stable sidebar, a single contextual header, and a content canvas with a readable maximum width where appropriate.
- Mobile uses a compact header and navigation drawer while preserving the same information hierarchy.
- Workspace Console, Platform Console, and Region Operations each have an explicit title and navigation model.
- Dense operational pages use tables with visible labels, filters, pagination, empty states, and row-level actions.
- Forms group fields by user decision. Advanced infrastructure fields are absent from tenant workflows.

## Interaction

- The Workspace switcher changes tenant scope and then navigates to a safe landing page for that Workspace.
- Entering Platform Console is an explicit navigation action, not a role toggle.
- Region selection in instance creation explains latency and availability. Node is scheduled automatically.
- Region switching exists only inside Region Operations and preserves the current subpage when possible.
- Destructive operations explain impact and require confirmation. Routine navigation and filters do not.
- Loading, empty, stale, partial, unavailable, forbidden, and failed states are designed for every data view.
- Avoid unlabeled icon-only actions for consequential operations.
- Avoid menu labels with tiny count badges. Put counts in the page content when they support a decision.

## Visual constraints

- No gradients, glass panels, neon glow, ornamental background art, or excessive shadows.
- Do not represent every section as a floating card. Use page hierarchy, whitespace, dividers, and tables first.
- Use badges only for categorical state such as `Running`, `Pending payment`, or `Degraded`.
- Never use a badge as the sole carrier of a metric.
- Motion is limited to focus, disclosure, navigation continuity, and state transitions; respect reduced-motion preferences.

## Component ownership

- `foundation`: tokens, typography, icons, theme, locale, and responsive primitives.
- `ui`: reusable accessible controls with no domain knowledge.
- `shell`: operating-area navigation, contextual headers, and account controls.
- `features`: vertical product modules that own their queries, mutations, copy keys, and composed views.
- Pages assemble feature interfaces and must not contain raw HTTP calls or duplicate permission logic.

The implementation must pass keyboard navigation, visible focus, semantic landmark, contrast, responsive, theme, and both locale smoke checks before a page is accepted.
