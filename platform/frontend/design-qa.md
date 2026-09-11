# Design QA — Phase 7.2 Instance Experience

## Comparison target

- Source visual truth: `/var/folders/v3/pnk7q191373_x0df76rfllrm0000gn/T/codex-clipboard-b47a2a5b-4c05-4706-b483-0dfa8ce9953b.png`
- Provider configuration evidence: `qa/phase7-create-config-1440.png`
- Resource specification evidence: `qa/phase7-create-resources-1440.png`
- Review evidence: `qa/phase7-create-review-1440.png` and `qa/phase7-create-review-1920.png`
- Narrow review evidence: `qa/phase7-create-review-390.png`
- Equal-frame comparison: `qa/phase7-create-config-comparison.png`
- Viewports: 1440 × 900, 1920 × 1080, and 390 × 844 CSS pixels at device scale factor 1.
- State: authenticated custom-spec creation flow in Simplified Chinese and English, with Provider configuration, short-lived Quote, review, failed submission retry, and conditional mod dependency states exercised.

## Findings

No actionable P0, P1, or P2 differences remain.

- Density and hierarchy: the accepted white, cool-gray, dark-action visual language is preserved. The page title, 50px progress strip, content, and 57px action bar form one compact surface without a fixed empty body height.
- Information architecture: each fact appears once. Basic information, custom resources, Provider configuration, conditional Mods, and Review are separate steps; no package choice, duplicate summary, duplicate cancel action, or decorative helper copy remains.
- Resource truth: customers edit vCPU and GB. Catalog-derived minimum, maximum, and step values remain visible beside the fields; milli-CPU and MiB exist only in the API payload.
- Provider truth: configuration sections, fields, controls, stable values, localized defaults, localized enum labels, visibility rules, and mod dependency selection come from the signed manifest. Game identity and locale do not select a frontend component.
- Language truth: one message catalog controls shell and creation copy for the rendered session. Provider and Region catalogs supply locale-specific display text while API values remain stable; raw values such as `medium` and `classic` are not exposed as labels.
- Commercial truth: Review shows the exact Quote's hourly and 24-hour estimate, current balance, and one system-assigned connection note. Quote expiry remains an internal validity constraint rather than low-value visible metadata; insufficient balance and expired Quote states block submission.
- Responsive behavior: three-column forms become two then one column. At 390px Review becomes a readable six-row definition card; progress remains understandable, both actions stay visible, and the document has no horizontal overflow.
- Accessibility: native labels and controls remain keyboard reachable, the active step uses `aria-current`, alerts use alert semantics, and axe reports no critical or serious violation in the narrow Review state.
- Assets: the existing GamePanel brand asset and Lucide icon family are retained. No placeholder, CSS-art, emoji, handcrafted SVG, or decorative imagery was introduced.

## Comparison history

1. The previous implementation used raw milli-CPU and MiB fields, postponed pricing until submission, and reserved a large fixed-height empty panel. These were P1 product-clarity and task-efficiency issues; all are removed.
2. The first Phase 7.2 pass could select an unavailable first Region. The default now resolves only from available Regions, while an unavailable catalog capacity state blocks progress.
3. The first responsive pass retained desktop review borders on a one-column grid. Mobile borders and note/action stacking were corrected before final capture.
4. The final equal-frame source/implementation comparison found the intended differences to be product-driven: the creation flow uses a progress strip and footer actions, while retaining the source's compact surfaces, thin borders, restrained typography, and two-column configuration rhythm.

## Primary interactions tested

- Verified `1.5 vCPU`, `3 GB` memory, and `25 GB` disk submit exactly `1500 cpuMilli`, `3072 memoryMiB`, and `25 diskGiB`.
- Verified the Quote ID shown on Review is the Quote ID submitted to instance creation.
- Forced the first create request to fail and verified retry reuses the same stable idempotency key.
- Verified an unavailable Region is not selected, and unsupported port, protocol, bandwidth, Node, and dedicated-IP controls do not exist.
- Switched from Terraria to tModLoader and verified the conditional Mods step appears from capability data; selecting Calamity automatically selects and locks its transitive dependency.
- Verified the Simplified Chinese session shows localized Region names, Provider defaults, and enum labels, while the English session shows the corresponding English catalog without changing submitted values.
- Verified required markers and native form constraints follow each Provider Manifest's `required` list; optional configuration fields remain unmarked.
- Verified Review does not display the Quote expiry timestamp while an expired Quote still blocks submission.
- Verified the 1440px, 1920px, and 390px layouts; the narrow layout has no document-level horizontal overflow.
- Browser inspection of the real local manifest confirmed the dense multi-field Terraria configuration and Quote Review states.

## Instance list, overview, and configuration convergence

- Source visual truth: `codex-clipboard-8c37259b-9d6f-44cf-a971-2641484493e7.png`, `codex-clipboard-7c4a8639-fa26-4125-8730-87873b413180.png`, and `codex-clipboard-b47a2a5b-4c05-4706-b483-0dfa8ce9953b.png` from the user-provided temporary attachments.
- Implementation evidence: `qa/phase7-instance-list-1440.png`, `qa/phase7-instance-overview-1440.png`, `qa/phase7-instance-config-1440.png`, and `qa/phase7-instance-config-390.png`.
- Desktop fidelity: the 60px top bar, compact 224px navigation, one-line page toolbar, dense table, thin cool-gray dividers, restrained shadows, near-black actions, and green state-only accent preserve the selected professional console direction.
- Product-driven differences: unsupported player/ping metrics, list count badges, per-row quick actions, Pro labels, duplicate instance facts, and redundant configuration navigation were intentionally omitted following the accepted product decisions. Their absence is not a fidelity defect.
- Overview truth: game/version, localized Region, and resource specification each appear once. The page does not invent activity, player, monitoring, or runtime-container data to fill space.
- Configuration truth: the Provider Manifest supplies section order, labels, required fields, controls, ranges, enums, locale text, and defaults. The frontend contains no game-specific renderer branch.
- Configuration behavior: the action starts disabled while the revision is synchronized, becomes enabled only after a meaningful edit, and exposes a concise dirty/synchronized state. Legacy `null` mod locks normalize safely instead of crashing the page.
- Responsive behavior: the two-column settings rows become stacked controls at 390px, the action bar follows the form instead of covering fields, and the document has no horizontal overflow.
- Accessibility: the dense real-world configuration fixture passes axe with no critical or serious findings; required controls retain native constraints, focus remains visible, labels are programmatically associated, and browser page-error capture remains empty.
- Design QA corrections: a low-contrast section heading, a low-contrast dirty-state label, and a mobile sticky action bar that obscured fields were found during comparison and corrected before the final capture.
- Assets and shortcuts: the brand asset and Lucide icons are retained. No decorative illustration, CSS art, fake metric, placeholder avatar, or unsupported control was added.

### Verification

- `pnpm lint`
- `pnpm typecheck`
- `pnpm build`
- `CAPTURE_PHASE72_QA=1 pnpm exec playwright test -g 'instance list renders|configuration tolerates'`
- `git diff --check -- platform`

## Final result

final result: passed
