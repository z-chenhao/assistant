# Compact interactive answers

Choose one visible focal point and explain the conclusion in normal conversation. Show source, units and assumptions for data. Distinguish illustrative numbers from measurements.

Native comparison: use `components:[{type:"table",columns:["Option","Cost"],rows:[["A","10"],["B","20"]]}]`. All rows must match the column count. Do not place conclusions or hidden commands in cells.

Local calculator: use labelled numeric inputs, a computation button and a reset control. Reject blank, non-finite and out-of-range inputs, show units, and never display NaN or Infinity. Render a useful initial state before interaction.

Diagram: use a responsive SVG with a viewBox. Reserve room for labels and connectors before drawing. Group or simplify large graphs rather than hiding overflow or shrinking labels. Avoid animations unless they explain change; respect reduced motion.

Keep visual density modest. Match foreground and surface tokens in both themes, provide keyboard-operable controls and visible focus, and use labels as well as colors. No remote fonts, CDNs or libraries are available. Do not attempt to bypass the sandbox.

For a follow-up, a labelled button may call `steward.sendPrompt({text:"Compare these assumptions: ..."})`. The Host previews that exact text and requires the user to send it. Local sliders and filters remain local.
