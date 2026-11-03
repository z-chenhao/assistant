---
name: Steward Web
description: Your personal AI steward.
colors:
  background: "#f9f9f9"
  sidebar: "#f3f3f3"
  card: "#fcfcfc"
  foreground: "#212121"
  muted: "#eee"
  muted-foreground: "#666"
  border: "#dfdfdf"
  hover: "#e9e9e9"
  accent: "#212121"
  accent-ink: "#f9f9f9"
  focus: "#767676"
  success: "#414141"
  success-surface: "#eee"
  warning: "#505050"
  warning-surface: "#eee"
  danger: "#8e3838"
  danger-surface: "#f6eded"
  backdrop: "#21212166"
  dark-background: "#212121"
  dark-sidebar: "#181818"
  dark-card: "#282828"
  dark-foreground: "#ededed"
  dark-muted: "#303030"
  dark-muted-foreground: "#afafaf"
  dark-border: "#3e3e3e"
  dark-hover: "#303030"
  dark-accent: "#ededed"
  dark-accent-ink: "#212121"
  dark-focus: "#9f9f9f"
  dark-success: "#d2d2d2"
  dark-success-surface: "#303030"
  dark-warning: "#c9c9c9"
  dark-warning-surface: "#303030"
  dark-danger: "#edb3b3"
  dark-danger-surface: "#362727"
  dark-backdrop: "#10101099"
  ghost-light-ink: "#f6f5f1"
  ghost-dark-ink: "#232323"
typography:
  wordmark:
    fontFamily: "StewardWordmark, Arial, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: "28px"
    letterSpacing: "0.015em"
  display:
    fontFamily: "'Source Serif 4', Georgia, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "36px"
    fontWeight: 400
    lineHeight: 1.35
    letterSpacing: "-0.02em"
  headline:
    fontFamily: "Inter, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "28px"
    fontWeight: 550
    lineHeight: 1.4
    letterSpacing: "-0.02em"
  title:
    fontFamily: "Inter, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "17px"
    fontWeight: 550
    lineHeight: 1.5
  body:
    fontFamily: "Inter, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "14px"
    lineHeight: 1.6
  reply:
    fontFamily: "'Source Serif 4', Georgia, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "16px"
    lineHeight: 1.75
  label:
    fontFamily: "Inter, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "12px"
    fontWeight: 500
  code:
    fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Consolas, 'PingFang SC', 'Source Han Sans SC', 'Microsoft YaHei', sans-serif"
    fontSize: "13px"
rounded:
  button: "9px"
  field: "10px"
  panel: "12px"
  pill: "20px"
  user-message: "22px"
  composer: "23px"
  circular: "50%"
spacing:
  control-gap: "8px"
  action-gap: "10px"
  panel: "24px"
  panel-mobile: "18px"
  workspace-gutter: "40px"
  workspace-gutter-mobile: "20px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-ink}"
    rounded: "{rounded.button}"
    padding: "9px 15px"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    rounded: "{rounded.button}"
    padding: "9px 15px"
  button-plain:
    backgroundColor: "transparent"
    textColor: "{colors.muted-foreground}"
    padding: "0"
  field:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.field}"
    padding: "10px 12px"
  navigation-item:
    textColor: "{colors.muted-foreground}"
    rounded: "{rounded.button}"
    padding: "10px 11px"
  status-pill:
    backgroundColor: "{colors.background}"
    textColor: "{colors.muted-foreground}"
    rounded: "{rounded.pill}"
    padding: "3px 9px"
  panel:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.panel}"
    padding: "{spacing.panel}"
  composer:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.composer}"
    padding: "17px 17px 12px"
---

# Design System: Steward Web

## Overview

**Creative North Star: "Your personal AI steward."**

Steward is a familiar, quiet conversation workspace: soft monochrome surfaces, readable document-style replies, restrained controls and a small plush ghost. The owner-approved `tmp/neutral-web/dist` demo defines this production presentation. This document records implemented values rather than adding a new product direction.

This is a private Web presentation system for the owner's ongoing conversation and delegated work. Server-owned HTML, bounded live-region patches and existing React UISpec islands remain the mechanism. Steward names the interface; Collie remains the compatible backend, CLI and storage identity. No public design API, domain rename or storage migration is implied.

**Key Characteristics:**
- One collapsible navigation sidebar and a centered conversation.
- Near-white and charcoal surfaces with grayscale status treatments and restrained danger color.
- Inter controls, serif reply prose, a Michroma wordmark and a stable, expressive ghost.
- Real tasks, artifacts and frozen approvals presented with their existing semantics.

## Colors

The normative light and dark values above come from `src/foundation.css`; `dark-*` keys map to the same CSS roles under `:root.dark`.

The sidecar's generated tonal ramps support design-panel previews; they are not additional production color tokens.

### Primary
- **Charcoal / soft-light action:** `accent` and `accent-ink` invert with the theme. Use for primary actions and the circular send control.

### Neutral
- **Soft workspace:** `background`, `sidebar` and `card` separate the canvas, single navigation and panels through tone.
- **Ink and quiet text:** `foreground` carries readable content; `muted-foreground` carries labels, timestamps and supporting text.
- **Muted surface, hover and border:** use the corresponding semantic tokens for user bubbles, selected work filters, rows and fine dividers.
- **Focus:** use `focus` for visible keyboard outlines and the focused composer border.

Success and warning use grayscale text/surfaces. Danger retains its muted red pair; textual status and error messages carry meaning alongside color. Ghost facial ink uses its own two theme-adapted values from `src/avatar.css`.

Theme initializes from the saved `steward-theme` preference, otherwise system appearance at page load. The appearance control stores an explicit local light/dark choice.

## Typography

`StewardWordmark` loads Michroma and is reserved for the text brand. Inter is the UI family. Source Serif 4 with Georgia fallback supplies welcome display and assistant reply prose. JetBrains Mono supplies code. All content stacks include PingFang SC, Source Han Sans SC and Microsoft YaHei for Chinese; bundled font faces use `font-display: swap`.

- **Display:** 36px/1.35, weight 400; welcome heading, reduced to 30px on mobile.
- **Headline:** 28px/1.4, weight 550; standard page title, reduced to 26px at 760px.
- **Title:** 17px/1.5, weight 550; section headings.
- **Body:** 14px/1.6 UI text. Conversation messages and composer input use 16px; reply prose uses 16px/1.75.
- **Label:** 12px, weight 500 for small supporting labels; fields use 13px and message labels use 14px.
- **Code:** 13px with ligatures disabled; technical disclosures may use 12px.

Keep controls and artifact links in the UI family even when they appear beside serif responses. Preserve wrapping for long text and identifiers.

## Layout

Desktop has one fixed 256px sidebar and a main workspace offset by its width. Collapsing it removes that offset. The Chat sticky header is 120px high with a centered two-row name/avatar; other pages use a 72px topbar. The conversation's 840px border box includes 36px horizontal padding, yielding a 768px dialogue; the composer also caps at 768px. Standard workspace pages cap at 1040px, settings at 920px, with 40px workspace gutters.

At 760px and below, the sidebar becomes an on-demand 256px overlay with backdrop, the workspace fills the viewport, the Chat header becomes 116px (other topbars 60px) and gutters become 20px. The dialogue uses 20px horizontal padding; composer gutters become 12px with safe-area bottom support. Without JavaScript, mobile navigation remains available in normal flow. Task detail has a local 290px supporting column (250px below 1000px) and stacks below 800px; this is page content, not another application sidebar. Page headings and choice cards adapt below 700px. The document minimum width is 320px.

The conversation footer is fixed for an active transcript with bottom clearance in the thread. On the welcome view it returns to normal flow below the greeting. Read-only Computer/Worker detail panes reserve 460px on desktop; mobile uses one modal sheet.

## Elevation & Depth

Panels, navigation and composer use tonal surfaces and 1px borders without drop shadows. Sticky/fixed placement and the mobile backdrop establish stacking. The ghost alone has a small grounding oval: foreground at 0.12 opacity with a 1px blur. Do not turn this character detail into a panel shadow vocabulary.

## Shapes

Controls have 9px corners, fields 10px and panels 12px. Status pills use 20px, user bubbles 22px and the composer 23px (22px on mobile). The send button is a 36px circle. List rows and tab underlines keep the utility pages flatter than the conversation's rounded forms.

## Components

### Controls, fields and navigation

Primary buttons use the action pair, 9px corners, 9px 15px padding and a 40px minimum height. Secondary buttons retain a fine border and transparent fill; plain icon buttons use quiet text and a hover surface. Background/color transitions take 180ms. Hover uses the existing foreground/hover treatments, and disabled controls use 0.45 opacity. Fields use card surfaces, 1px borders and 10px corners. Global keyboard focus is a 2px `focus` outline with 4px offset.

Sidebar links are 14px with 18px stroked icons, 42px minimum height and 9px corners. Hover and current navigation use `hover`; current links gain weight 500. Mobile navigation traps focus, makes main content inert, closes with Escape/backdrop and restores focus to its opener. Section tabs remain horizontal and scroll when necessary.

### Conversation and composer

There is one ongoing conversation per Agent, with bounded earlier-message navigation. Initial canonical home navigation resumes the latest turn; explicit earlier-history pages, hash anchors and back/forward navigation keep their reading position. Assistant replies read as documents; user messages are right-aligned muted bubbles. The multiline composer retains actual attachment, task context and durable source-ID behavior. Readiness controls send availability. An uncertain acknowledgement retains the draft and attachment; the presentation must not turn uncertainty into a successful send or automatic resend. The composer border changes on focus, and its send control shows the existing 900ms waiting spinner.

### Tasks, results and approvals

Computer and Worker open mutually exclusive read-only detail panes beside Chat. Worker history includes ongoing and finished executions in execution order; Tasks remains the independent task list and detail workspace. Pane changes keep the composer and its attachment nodes mounted.

Task lists/detail retain search, filters, pagination, progress, related updates, milestone history and real artifact links. Approval panels retain readable action fields, exact-action disclosure, frozen hash and task-scoped return links. Approval is a decision about the frozen action, not proof of external completion. Models/accounts, explicit model testing/selection, notification preferences and capability editors remain real settings pages; Developer remains a specialist inspection surface.

### Ghost and accessible feedback

The plush ghost uses complete lossless WebP atlas frames, with Canvas2D playback at about24fps during a2.1s greeting or320ms blink. It sleeps between blinks and uses a fixed hitbox/cooldown. Gentle whole-artwork drift is5.6s at ±2px (Chat header ±2px); the shadow stays fixed. No runtime mesh/WebGL is loaded. Small instances decode96px frames and large instances256px; CSS first-frame fallback, explicit pause, reduced motion and hidden/offscreen guards remain available.

Keep the skip link, semantic landmarks, native details, field labels, named icon buttons, `aria-current`, `aria-expanded`, pressed filter states and polite status regions. Errors use alerts. Preserve focus and scroll during bounded live updates, keyboard access to attachments and the text status paired with every visual status pill.

## Do's and Don'ts

### Do:
- **Do** use the semantic light/dark tokens and implemented font roles.
- **Do** keep one application sidebar, a centered conversation and on-demand real task results.
- **Do** preserve durable send, readiness, approval, attachment and replay contracts when changing presentation.
- **Do** keep technical detail available through its existing disclosures and Developer pages.

### Don't:
- **Don't** fabricate multiple chats, recent items, task counts, a global memory browser or a file library.
- **Don't** imply that approval means execution completed, or resend an unknown external outcome.
- **Don't** rename Collie backend/CLI/storage contracts to implement the Steward Web brand.
- **Don't** introduce a permanent second navigation rail, decorative panel shadows or unapproved scaling/rotation of the ghost; the approved vertical drift is intentional.

Evidence: `BRIEF.md`, `../PRODUCT.md`, `src/foundation.css`, `src/shell.css`, `src/conversation.css`, `src/avatar.css`, `src/workspace.css`, `src/shell.ts`, `src/live-updates.ts`, `src/appearance.ts`, `src/avatar.js` and `../internal/interaction/web/templates/`. Production Web authority is the approved demo and implementation, with authorization and backend compatibility recorded in PRODUCT.md and BRIEF.md.

Interface language is English by default. Settings → Preferences → Interface language supports English and 简体中文, persisted for this browser. Translate all application-authored labels, states, errors and accessible names using the shared catalog; preserve business content and technical identifiers. The Chinese UI uses the existing PingFang SC / Source Han Sans SC stack and retains the same component geometry.


### Chat workspace and Computer / Worker viewers · 2026-11-02

Navigation contains exactly Chat, Tasks, Settings and Developer. Work filters remain inside Tasks. Chat places Steward's name above its animated avatar at the top center, with continuous conversation below. Computer and current Worker triggers sit at the top right. Mutually exclusive detail panes preserve the conversation/composer DOM. Worker records come from a read-only, agent-scoped bounded query, including running and finished executions ordered by coalesced started/created time with a deterministic ID tie-break. The current running query is independent of history limits; backend states are preserved and unavailable reads labeled. Task routes remain independent list/detail pages. Task detail preserves goal, approvals, milestones, delivered files and continuation links.

Computer opens a read-only right pane on desktop, expandable to a wider view; narrow viewports and the expanded overlay use a modal sheet to keep keyboard focus out of covered content. Only an open visible pane polls authenticated status/activity and desktop PNG endpoints, about2s after each completed image. It keeps the last decoded image during refresh and labels interrupted/stale reception. Closing, hiding or navigation cancels scheduling. Preview cannot start/resume Computer, choose VM targets or send input. The Host captures the already-ready instance in a separate Driver session and rechecks the instance epoch before returning pixels. A missing/paused computer has a truthful empty state. Real macOS Guest compatibility remains a live-environment check.

A pending-decision summary is derived from the same authoritative approvals as displayed cards, counting inline and separate pending cards once. It links to the exact card without losing the composer; history views refresh to latest when necessary. Latest-message navigation appears when reading above the bottom. Composer height is measured to reserve space, including feedback and attachments. Mobile icon controls have44px targets and important auxiliary text is at least12px.

Source Han Sans retains PingFang priority. Disjoint unicode-range subsets preserve the original30926 codepoints and variable weights; the common6243-character pack is1283768bytes versus7995716bytes for the full source. Remaining characters load their own blocks. The original source is retained for provenance, so this is a network-loading improvement rather than a binary-size reduction. Hashes and ranges: licenses/source-han-subsets.json.
