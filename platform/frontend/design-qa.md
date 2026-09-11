# Design QA

## Comparison target

- Source visual truth: `/var/folders/v3/pnk7q191373_x0df76rfllrm0000gn/T/codex-clipboard-7e55ae53-0628-495d-a0d5-7da84dd9c348.png`
- Browser implementation: `qa/phase7-instance-list-1440.png`
- Wide-layout evidence: `qa/phase7-instance-list-1920.png`
- Narrow-layout evidence: `qa/phase7-instance-list-390.png`
- Full comparison: `qa/phase7-instance-list-full-comparison.png`
- Focused table comparison: `qa/phase7-instance-list-focus-comparison.png`
- Viewports: 1440 × 900, 1920 × 1080, and 390 × 844 CSS pixels at device scale factor 1.
- Comparison normalization: the 2458 × 952 source was scaled to 1440 × 558. The 1440 × 900 implementation was cropped to the same 1440 × 558 frame. The focused evidence stacks equal-width table crops from those normalized frames.
- State: authenticated Simplified Chinese workspace instance list with running, stopped, pending, failed, IP-only, multi-endpoint, long-name, empty, and loading states exercised.

## Findings

No actionable P0, P1, or P2 differences remain.

- Typography and density: compact neutral sans-serif labels and monospace operational values preserve the reference hierarchy. Table headers are 44px high and data rows are 56px high.
- Layout rhythm: the 228px sidebar, 16–20px shell gutters, compact page bar, thin dividers, and restrained 12px surface radii follow the selected prototype direction.
- Product truth: the implementation intentionally replaces the reference's player, ping, quick-action, and decorative count fields with game/version, provider-owned Endpoint facts, resources, region, and row navigation. No fact is repeated on the page.
- Color and accessibility: white surfaces, cool gray borders, dark primary action, and restrained green state match the source. The success text was darkened after axe reported 3.74:1 contrast; the final token passes WCAG AA for 12px text.
- Assets: the existing GamePanel brand asset and Lucide icon family are retained. No placeholder, CSS-art, emoji, handcrafted SVG, or decorative imagery was introduced.
- Responsive behavior: at narrower breakpoints lower-priority region/resource columns are removed; at 390px each instance becomes a compact readable row card without horizontal overflow.
- Data flow: the frontend renders the versioned list contract. The backend batches Provider Release and Region lookups by ID, performs no SQL JOIN, and composes the display model in Go.

## Comparison history

1. The first pass exposed a P2 mobile workspace-switcher wrap and a P2 Endpoint wording error that described every absent Endpoint as pending. The switcher now truncates safely and Endpoint copy distinguishes pending allocation from genuinely unassigned state.
2. Browser accessibility testing found a serious contrast issue for the 12px running label (`#07966f` on white, 3.74:1). The success token is now `#087a5d`; the focused Playwright accessibility check passes.
3. The final same-state full and focused comparisons found no remaining P0/P1/P2 mismatch. The reference-only player, ping, action, and count content is deliberately absent under the accepted product decisions.

## Primary interactions tested

- Loaded real workspace and instance data through the control-plane API, including the visible loading skeleton.
- Verified running, stopped, pending, and failed states plus stable/changeable, TCP/UDP, IP-only, absent, and multiple Endpoint presentations.
- Focused the row-level instance link and opened detail with Enter; the whole row remains clickable by pointer.
- Verified empty state and a long instance name.
- Verified 1440px, 1920px, and 390px layouts; the narrow layout has no document-level horizontal overflow.
- Playwright accessibility analysis found no critical or serious violations after the contrast fix.
- Browser console inspection found no application errors. The Next image sizing warning was corrected by declaring both rendered dimensions for the existing brand asset.

## Follow-up polish

- [P3] Region names currently use the canonical catalog display name (`Asia East`). A locale catalog can translate it later without changing the list contract or layout.
- [P3] Replace the olive square brand mark only after a final production brand asset is approved.

## Final result

final result: passed
