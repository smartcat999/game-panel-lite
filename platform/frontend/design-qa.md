# Design QA

## Comparison target

- Source visual truth path: `/var/folders/v3/pnk7q191373_x0df76rfllrm0000gn/T/codex-clipboard-b47a2a5b-4c05-4706-b483-0dfa8ce9953b.png`
- Browser-rendered implementation screenshot: `qa/phase6-config-implementation.png`
- Create-flow screenshot: `qa/phase6-create-implementation.png`
- Full-view comparison evidence: `qa/phase6-config-full-comparison.png`
- Focused-region comparison evidence: `qa/phase6-config-focus-comparison.png`
- Viewport: 1207 x 720 CSS pixels in the Codex in-app browser.
- Pixels and density: source 2464 x 1096 pixels; implementation 1207 x 720 pixels. The source was scaled to 720 pixels high and left-cropped to 1207 x 720; the implementation was captured at 1207 x 720. Both comparison halves are therefore 1207 x 720 at device scale factor 1.
- State: authenticated Simplified Chinese Workspace Console, running tModLoader instance, configuration tab, light theme.

## Findings

No actionable P0, P1, or P2 differences remain.

- Fonts and typography: both use a compact neutral sans-serif hierarchy with monospace operational values. The implementation keeps labels and controls legible at the denser target rhythm.
- Spacing and layout rhythm: the two-column configuration grid, narrow sidebar, thin dividers, compact tabs, and restrained panel spacing match the selected direction. Removing the reference's second category-tab row is intentional because the accepted product model forbids duplicate navigation and repeated facts.
- Colors and visual tokens: both use white surfaces, cool gray borders, near-black primary actions, and restrained green status. The olive GamePanel mark is a minor brand-token difference and is accepted as P3.
- Image quality and asset fidelity: the screen has no content imagery. Visible icons use one consistent Lucide stroke family; no placeholder, CSS-art, emoji, or fabricated SVG asset replaces reference content.
- Copy and content: labels are concise and product-specific. Unsupported player, ping, bandwidth, public-port, protocol, plan, decorative count, and World-management controls are absent.
- Behavior and accessibility: focus styles are present; native labeled inputs/selects/checkboxes expose roles and values; the active tab and disabled dependency state are programmatically visible.

## Comparison history

1. Initial Phase 6 comparison found a P2 state mismatch: `Calamity Mod Music` was displayed as an independent unchecked option even though the signed Provider Manifest declares it as a Calamity dependency. The frontend now resolves the manifest dependency graph recursively and renders transitive dependencies checked, disabled, and labeled `自动依赖`. Post-fix evidence is `qa/phase6-config-implementation.png`; browser accessibility state confirmed both Calamity entries selected and the music dependency disabled.
2. Candidate create-flow inspection found a P2 duplicate version: the immutable legacy and current tModLoader Provider Releases shared the same customer-visible game version. The selector now groups by game key/version and presents only the highest current Provider release while old instances remain operable by release ID. Post-fix evidence is `qa/phase6-create-implementation.png`; the browser exposed exactly one Terraria and one tModLoader option.
3. The final same-viewport full and focused comparisons found no further P0/P1/P2 mismatch. No visual fix was made after this pass.

## Primary interactions tested

- Opened the running instance and switched to the configuration tab.
- Opened create instance, selected tModLoader, and advanced through custom resources, Provider-driven game configuration, and conditional mod selection.
- Selected Calamity and verified its required music dependency became checked and non-editable.
- Verified endpoint address, port, transport, and stability are read-only runtime facts rather than creation controls.
- Browser accessibility snapshots showed no error dialog or error state. The available in-app browser API does not expose a JavaScript-console reader; nginx and Web service health/log checks are used as the runtime error check for this deployment.

## Follow-up polish

- [P3] Replace the olive square brand mark with the final mint `GP` production asset after brand assets are approved; this does not affect layout or task completion.

## Final result

final result: passed
