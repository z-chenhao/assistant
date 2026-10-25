# baseline-02 — ADR006 reseal provenance

The [original mapping request](baseline-02-reseal.json) was submitted at
`0790fa8df19beee0152e4939d6c009cfb0ef37a3`. Its bytes remain historical provenance;
its pending labels describe that request, not current admission status.

Owner comment `01a16e1d-3e2c-78fb-ba1d-5621d178a9bd` on
[ERII-232](https://multica.ai/erii-studio/issues/01a16938-d29b-7d7d-a011-673bb15a4653)
accepted the two old/new mappings and assigned three direct ADR006 additions to
baseline-02. The coordinator independently verified the new seal and removed
the obsolete admission blocker on 2026-10-24. The [current audit](baseline-02.md)
records all 83 current blocks and their evidence.

| Frozen block | Current block | ADR006 change |
| --- | --- | --- |
| §3.9 L224 | §3.9 L225 | Manage union adds `complete` |
| §3.9 L237, fenced contract | §3.9 L240, fenced contract | Adds create `completionPolicy` and complete arm |

The new §3.6 L176 and §3.9 L229–230 belong to baseline-02. The original 80
identities are preserved; 78 uniquely unchanged blocks keep their provenance.
Changed/new blocks were reviewed afresh rather than inheriting a conclusion.

The original manifest remains immutable, SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
The new 385378-byte manifest has SHA256
`f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837`.
It assigns all 1588 current blocks exactly once across 36 packages. The same
ADR's Task storage/lifecycle rows retain their baseline-11/baseline-13 owners.
This mechanical reseal changes no product gate and certifies no requirement.
See [ADR006](../adr/006-explicit-task-completion-policy.md).
