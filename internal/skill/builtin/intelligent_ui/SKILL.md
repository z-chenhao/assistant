---
name: intelligent_ui
description: Present a compact native component or interactive visual when it makes an answer easier to understand, compare, or explore.
---

# Intelligent UI

Use for meaningful comparisons, diagrams, small calculators and simulations. Prefer ordinary text for simple facts or commands. This is an inline answer, not a website, full application or slide deck. Main owns presentation; Workers return evidence and artifacts for Main to inspect.

## Workflow

1. Verify the facts and decide whether a visual improves the answer. Never invent data to fill a chart.
2. Read `references/design.md` and one matching ready template: `templates/native-comparison.json`, `templates/calculator.json`, or `templates/flow-diagram.json`. Replace illustrative data with verified inputs; adapt the smallest useful template. Use a native table/form for supported structured content; use custom HTML/CSS/JavaScript for relationships or local interaction.
3. Discover `builtin__intelligent_ui_show` with Code Mode `describeTool`. Call it through `tools[name](args)`. Supply a concise title and a complete, readable fallback for non-Web delivery and rendering failures.
4. Supply either `components`, or an `html` fragment with optional `css` and `javascript`. Do not include fenced code, a full page wrapper, external scripts or remote assets. Code appears only in the tool arguments, never in the final answer.
5. Inspect the tool receipt. `staged` means the Host saved the widget for your next reply, not that delivery occurred. Finish with a concise ordinary reply; the Host attaches the saved presentation and its text alternative. Do not generate JSON result fences.

## Rendering contract

Native `markdown`, `table`, `progress` and `form` components use the controlled Host renderer, including the OpenIntelligentUI-derived DataTable. Forms collect user input and never approve an action. The tool schema defines fields and limits.

Custom answers follow OpenIntelligentUI's isolated answer pattern: HTML/CSS/JavaScript runs inside an opaque-origin iframe. It cannot access parent DOM, storage, credentials, network, Host tools or approval APIs. Use only inline SVG, canvas, CSS and local computation. The iframe supports automatic bounded height and a Host-owned expanded view and text fallback.

Use CSS variables `--background`, `--foreground`, `--muted`, `--border`, `--brand`. The Host sets `:root[data-widget-theme="light"|"dark"]`; support both. Keep DOM queries within a widget root. Use event listeners, not inline handlers. JavaScript is a classic script: put asynchronous code inside async functions, not top-level await.

Controls update local state. When the user explicitly asks to discuss selected values, call `steward.sendPrompt({text: "...selected values and question..."})` from a button handler. This proposes text in a Host-owned draft; the user must press Send to Ghost. It does not grant permissions, run tools, approve writes or transmit arbitrary state automatically. Do not call it on load, timers or ordinary input changes.

On a validation failure, repair the same intended presentation or provide text. Do not create duplicate widgets after a successful receipt.
