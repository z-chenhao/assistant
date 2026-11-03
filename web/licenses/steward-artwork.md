# Steward / Ghost artwork

The production identity is the owner-selected charcoal and ivory plush Ghost,
originally provided on 2026-11-02. The selected source cutouts and generation
prompts are retained in `web/artwork/source/` and `web/artwork/manifest.json`.
The source cutouts are build inputs, not public runtime images.

The four lossless WebP atlases contain the complete character, including eyes
and smile. `web/artwork/icons.mjs` exports their first frame as the two static
PNG icons. README, favicon and the Web avatar therefore share the same artwork;
the former hand-drawn SVG and unused prompt experiment have been removed.

The current project artwork is distributed under the project's Apache-2.0
license. Third-party fonts retain their adjacent SIL OFL notices. Font origins
and hashes are in `web/licenses/typography.json`; Source Han subset coverage and
hashes are in `web/licenses/source-han-subsets.json`. No trademark rights are granted.
