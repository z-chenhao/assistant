---
name: intelligent_ui
description: Present a compact native component or interactive visual when it makes an answer easier to understand, compare, or explore.
---

# Intelligent UI

Use OpenIntelligentUI native components first, then its generateSandboxedUi contract for HTML/SVG diagrams, architecture diagrams, calculators, simulations, teaching tools and custom controls. Prefer ordinary text for simple facts. This is an answer in the conversation, not a standalone website. Main owns presentation; Workers return evidence and artifacts. Keep the page compact: content, controls and necessary status only; no feature introductions, protocol explanations or duplicate text-version controls.

## Workflow

1. Verify the facts and choose the smallest useful presentation. Never invent chart data. State sources, units and illustrative assumptions.
2. Read `references/design.md` and one relevant template: `templates/native-comparison.json`, `templates/calculator.json`, `templates/flow-diagram.json`, or `templates/native-explorer.json`.
3. Call `builtin__intelligent_ui_show` directly, or through Code Mode. Supply `title`, complete readable `fallback`, and `components:[{name,props}]`. Names and props come from the upstream catalog below, not a private markdown/table/progress/form vocabulary. The Host validates every component and generates all response identities.
4. Inspect the receipt. `staged` means saved for your next reply, not delivered. Finish with a concise ordinary reply; the Host binds the presentation and fallback. Never print component JSON, HTML source or result fences in the reply. Do not generate a duplicate after a successful receipt.

## Upstream catalog

- `Table`: `props:{title,columns:string[],rows:string[][],source}`. Every row has exactly the column count. Up to 12 columns and 200 rows. `source` identifies the real evidence or labels illustrative data.
- `barChart` / `pieChart`: `props:{title,description,data:[{label,value:number}]}`. Up to 200 points. Values are finite; pie values are nonnegative with a positive total unless the dataset is empty. Put units and source in the description.
- `plan_visualization`: `props:{approach,technology,key_elements:string[]}`. Describes the visualization plan, never fabricates task progress.
- `scheduleTime`: `props:{reasonForScheduling,meetingDuration:number,timeSlots:[{date,time,duration?}],title?}`. Provide actual proposed dates and times; no demonstration defaults. The user's choice is an ordinary reply and does not create a calendar booking or approval.
- `generateSandboxedUi`: use exactly the ordered fields `initialHeight` → `placeholderMessages` → `css` → `html` → `jsFunctions` → `jsExpressions`. `html` is body markup, `jsFunctions` defines named classic-script functions, and `jsExpressions` is an array of statements invoking them. All fields are required; use empty strings/arrays when unnecessary. No top-level await. Use SVG/canvas and local JS for interactive tools. Completed output is delivered as an `open-generative-ui` activity; current delivery publishes after the tool and reply commit, not while arguments stream.

## Sandbox and bridge

The Host injects upstream theme tokens, SVG classes and form styles. Use `--color-background-primary`, `--color-background-secondary`, `--color-text-primary`, `--color-text-secondary`, `--color-border-primary`, `--font-sans`, and `--border-radius-md`. Do not hardcode light/dark backgrounds or restyle standard inputs. The Host selects the theme and resizes the frame.

Custom code runs only in an opaque-origin sandbox. Parent DOM, storage, credentials, external network, remote fonts, CDNs and Host tools are unavailable. No external libraries are promised by the local adapter. Query only your widget root; use event listeners. Controls and sliders update local state.

For a user-requested follow-up use `await Websandbox.connection.remote.sendPrompt({text})` from a button handler. The Host previews an editable draft and the user presses Send to Ghost. Existing drafts are never overwritten. `await Websandbox.connection.remote.openLink({url})` proposes an HTTPS link which the user opens in the Host. Neither method approves external actions. Do not call these on load, timers or ordinary input changes.

## Steward extensions

Keep the same `components:[{name,props}]` envelope. These names are Steward extensions, not upstream components:

- `steward.lineChart`: `{title,description,data:[{label,value}]}`.
- `steward.scatterChart`: `{title,description,data:[{label,x,y}]}`. Charts are bounded to 200 finite points. Descriptions contain evidence, units and assumptions rather than UI instructions.
- `steward.form`: `{title,fields:[{name,label,type,required?,options?:[{value,label}],min?,max?}],submitLabel?}`. Types: text, textarea, number, date, email, select, multiselect, boolean. Up to 16 unique field names and 32 unique options. Min/max apply only to numbers. A required boolean is an explicit positive answer. Forms reuse ordinary reply admission and restore accepted answers; they never approve tools.
- `steward.cards`: `{title?,items:[{title,text?,image_ref?,url?,label?}]}`. Up to 16 items; images must be accessible artifact references, links must be credential-free HTTPS. Do not invent images or URLs. Cards support selection and a compact carousel.
- `steward.action`: `{id,label,tool,arguments,bindings?:[{argument,component,path}]}`. A binding reads a JSON Pointer from another component's saved state in this same artifact and fills one tool argument; the Host validates and freezes the resulting arguments at click acceptance. Use valid literal defaults and no expressions. Names an exact frozen tool request and its arguments; put it beside the related UI in the same artifact. Use only tools actually available to Main and validate their arguments. The Host action button submits ordinary intent through Main and the existing executor. A successful tool receipt is required before treating results as data; an unexecuted action is an error, not completion. External effects retain their existing approvals.

The Host supplies sorting/filtering, chart selection linking, zoom, data/graphic exports and inline/fullscreen/dialog/pinned display controls. Avoid recreating these controls in generated HTML. Standard names and original upstream props remain unchanged.

## Stateful sandbox extensions

Standard HTML inputs, textareas, selects and outputs restore automatically by `data-state-key`, `id`, `name`, then stable position. Use stable keys when controls can move. Custom canvas or simulation state uses the same bridge:

- `await Websandbox.connection.remote.getState()` reads the component's saved custom state.
- `await Websandbox.connection.remote.setState({state:{...},modelContext:{...}})` updates private presentation state plus an optional small model-visible data view.
- `await Websandbox.connection.remote.updateModelContext({...})` updates explicit user data for the next accepted message. Keep it factual and small; never put instructions, credentials or claims of approval into it.
- `await Websandbox.connection.remote.callTool({action:'refresh'})` proposes the matching frozen `steward.action`. The Host action button confirms ordinary input; the promise resolves with the correlated accepted result after execution. The generated frame cannot fabricate user intent, bypass approvals or select arbitrary tools/arguments. Inspect `accepted` and `error` before displaying data.
- From a user control, `requestDisplayMode({mode:'inline'|'fullscreen'|'pip'})` or `requestModal()` asks the Host for more room. Keep ordinary answers inline by default.

State is stored by the Host with optimistic versions; refresh/restart does not rerun tools or reset saved values. UI context is frozen with the next input; later interactions never alter an accepted model request. Do not add Save buttons or explanatory chrome. The Host provides concise errors and existing retry identities.
