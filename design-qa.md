# Product Design QA

## Evidence

- Source visual truth: Codex in-app browser capture of the authenticated `/servers` page at commit `6c3ef902`, before this migration pass.
- Source implementation path: `git:6c3ef902:apps/web/components/app-shell.tsx` and `git:6c3ef902:apps/web/app/servers/page.tsx`.
- Implementation URL and screenshot source: `http://localhost:3005/servers`, captured in the Codex in-app browser after the changes in this working tree.
- Mobile viewport: 490 × 814 CSS pixels, device pixel ratio 2. Source and implementation used the same browser surface and density.
- Desktop viewport: 1280 × 900 CSS pixels, device pixel ratio 1, applied with the browser's device-metrics override for the responsive check.
- State: authenticated temporary local admin session, Chinese locale, one stopped instance. The repository database was not modified.

## Findings

- No actionable P0, P1, or P2 findings remain.
- Fonts and typography: the system sans and monospace data treatment match the existing light SaaS console. Page titles, labels, prices, and resource values retain the established hierarchy without decorative count badges.
- Spacing and layout rhythm: desktop keeps the compact sidebar and content split. Mobile now uses a single compact horizontal navigation row, keeping the page action and first data surface above the fold.
- Colors and visual tokens: existing slate surfaces, restrained emerald selection states, purple tModLoader state, micro borders, and subtle elevation remain consistent.
- Image quality and asset fidelity: these management screens contain no product imagery. Existing Lucide icons are used consistently; no placeholder images, emoji, custom SVG, or CSS-drawn assets were introduced.
- Copy and content: navigation now names the actual Settings destination. Deploy copy explains Region-level selection and automatic Node scheduling. Plan prices and resources come from the backend catalog.
- Accessibility and interaction: the deploy surface exposes dialog semantics and a labelled close control. Engine, Region, and plan selection work with native buttons. World and mod actions open typed file inputs and provide visible operation status.

## Full-view Comparison Evidence

- Before: the 490-pixel viewport rendered the complete desktop sidebar above the Instances content, pushing the primary task below the fold.
- After: the same viewport renders the top bar, compact navigation, page header, and first table row in the initial view.
- Desktop: the 1280 × 900 check retains the sidebar, aligned page header, and high-density table without changing the established shell proportions.

## Focused Region Comparison Evidence

- Deploy dialog: inspected Vanilla and tModLoader states at the mobile viewport. Changing engine updates the available catalog plans; Region and plan selections remain visible without horizontal clipping. Resource specifications and price are readable at the decision point.
- Worlds, Mods, and Settings: inspected as authenticated mobile routes. Headers, actions, empty/data states, and settings form align to the shared console frame.
- Browser console: no new warning or error entries were recorded during the final three-minute verification window after a clean preview restart.

## Comparison History

1. P1 mobile navigation: the full sidebar consumed the first screen. Fixed by using a compact mobile navigation and keeping the sidebar at desktop widths.
2. P2 navigation semantics: Dashboard duplicated Instances, while Audit Logs routed to Settings. Fixed by removing the duplicate link and naming Settings accurately.
3. P2 deployment model: the old dialog requested a host port and hardcoded resources. Fixed by reading Region and plan availability from the commerce catalog, passing the selected plan resources, and leaving Node placement to the scheduler.
4. Post-fix evidence: recaptured `/servers`, the Deploy dialog, `/worlds`, `/mods`, and `/settings`; verified engine-plan interaction and found no remaining P0/P1/P2 issue.

## Implementation Checklist

- [x] Reuse one page-header component across management routes.
- [x] Replace the mobile full sidebar with compact navigation.
- [x] Read deployment plans and Region availability from the backend catalog.
- [x] Keep Node scheduling automatic.
- [x] Make world import and mod upload actions functional.
- [x] Pass lint, typecheck, production build, route checks, and browser console review.

## Follow-up Polish

- No blocking polish remains. A future catalog iteration can expose additional Region-specific plans without frontend structural changes.

final result: passed
