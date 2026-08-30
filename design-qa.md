# Login design-system QA

## Comparison target

- Authoritative design source: `/Users/stephenqiu/Desktop/StephenQiu/HotKey/hotkey-server/design.md`
- Latest defect baseline supplied by the user: `/var/folders/r5/lm_1_1hd321dzlfq0lctjdnw0000gn/T/codex-clipboard-d6ba6500-52d5-4334-b2cc-ca4acdf03397.png`
- Rendered implementation: `/tmp/hotkey-login-agent-browser/screenshots/login-final-1754x967.png`
- Route: `http://localhost:8010/login`
- State: light theme, signed out, empty login form
- Viewport: 1754×967 CSS pixels, device density 1
- Source pixels: 3508×1934, including external browser chrome at approximately 2× density
- Implementation pixels: 1754×967
- Normalization: the browser chrome in the supplied 3508×1934 screenshot is excluded from product judgments. Its app content represents approximately the same 1754×967 CSS viewport at 2× density. The screenshot is used as the before-state evidence; `design.md` defines the intended palette, borderless structure, typography, motion, and Three.js behavior.

## Findings and comparison history

### Iteration 1 — blocked

- [P1] Structural surfaces created visible color blocks.
  - Baseline evidence: the right half used a full `surface-muted` column, the form used a white elevated card, and the left half used a full grid layer.
  - Design-system conflict: `design.md` requires low-interference neutral hierarchy and borderless structure through whitespace before adding nested surfaces.
  - Fix: unified both columns on `background` (`#FAFAFA`), removed the form card fill and shadow, removed the left grid, and retained white only for editable controls.
- [P1] Decorative elements behaved like additional cards.
  - Baseline evidence: the capability items, top link, authentication kicker, registration prompt, and signal scene all had rounded gray or white backgrounds.
  - Fix: converted them to text/icon groupings on the page canvas and added an explicit transparent `ambient` variant to `SignalOrbitScene`.
- [P2] Chinese display typography used aggressive negative tracking and exceeded the documented 48–56px display range.
  - Fix: capped the editorial title at 56px, changed its line height to 1.05, and restored normal Chinese letter spacing. The form title also uses normal tracking.

### Iteration 2 — reopened after browser evidence

- Post-fix full-view evidence: the final screenshot presents a single continuous `#FAFAFA` canvas. Left narrative and right form are separated by whitespace and alignment rather than column fills or elevated containers.
- Post-fix runtime evidence: `main` is `rgb(250, 250, 250)`; form, capabilities, return link, registration prompt, and ambient Three.js figure all compute to transparent backgrounds. Inputs compute to `rgb(255, 255, 255)` as the permitted editable-control surface.
- [P1] The static radar fallback and WebGL canvas still used a 500ms opacity crossfade, allowing both images to be visible during scene activation.
- [P2] The transparent form card still inherited the default `elevated` surface shadow; the ghost shadow remained visible below the form.
- [P2] The form column used vertical centering, so its heading drifted substantially below the left narrative on tall desktop viewports.

### Iteration 3 — passed

- Fix: canvas/fallback state changes now update both `opacity` and `visibility`. The ambient scene has no opacity transition, so only one renderer can be painted at a time.
- Fix: the authentication card now uses `variant="flat"`; computed card shadow is fully transparent.
- Fix: the form is top-aligned at the desktop breakpoint and narrowed to 384px. At 1754×967 its top is 96px and the left kicker top is 102px.
- Agent Browser evidence: immediate-load screenshot `/tmp/hotkey-login-agent-browser/screenshots/fixed-immediate.png`, stable annotated screenshot `/tmp/hotkey-login-agent-browser/screenshots/fixed-stable-annotated.png`, and matched-viewport final screenshot `/tmp/hotkey-login-agent-browser/screenshots/login-final-1754x967.png`.
- Runtime state: canvas is `opacity: 1; visibility: visible`; fallback is `opacity: 0; visibility: hidden`; page scroll height equals the 967px viewport.
- No actionable P0, P1, or P2 issue remains in login, registration, or password-recovery authentication screens.

## Required fidelity surfaces

| Surface | Result | Evidence |
| --- | --- | --- |
| Fonts and typography | Passed | Geist stack retained; Chinese titles use normal tracking; editorial title is capped at 56px with 1.05 line height. |
| Spacing and layout rhythm | Passed | Two-column hierarchy remains legible without a column divider; form and narrative share the same top anchor; page height equals the 967px viewport. |
| Colors and visual tokens | Passed | One `#FAFAFA` canvas, `#262626` foreground hierarchy, transparent structural groups, and white editable controls only. No chromatic accents or large white/gray nested blocks remain. |
| Image quality and asset fidelity | Passed | The Three.js scene and static fallback are mutually exclusive in transparent ambient mode; no rounded panel, shadow, gradient, or double image remains. |
| Copy and content | Passed | Login purpose, monitoring proposition, capability labels, recovery route, account-creation route, and homepage return remain unchanged. |

## Focused region comparison

- Form region: the baseline showed a gray column containing a white rounded card; the final render shows a transparent form region on the page canvas. Only input controls and the disabled primary action use functional surfaces.
- Signal region: the latest baseline showed the static radar and Three.js scene superimposed; the final render shows one WebGL signal scene with the fallback hidden.
- Header and supporting labels: baseline rounded fills were removed; focus rings remain available for keyboard interaction.

## Interaction and runtime verification

- Empty form starts disabled.
- Entering email and password enables the primary action.
- Password visibility toggle changes the input type to `text`; reload restores the empty disabled state.
- Verified navigation targets: `/`, `/register`, and `/forgot-password`.
- Registration and password-recovery pages were rendered and visually checked through Agent Browser.
- Agent Browser WCAG A/AA audit: 0 violations on login, registration, and password recovery.
- Production console errors: 0.
- Desktop page overflow: none at 1754×967.

## Automated verification

- Focused unit and design-contract tests: 25 passed.
- Full unit suite: 56 files, 295 tests passed.
- TypeScript: passed.
- Next.js production build: passed.
- Docker production rebuild: passed.

final result: passed
