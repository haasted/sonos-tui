# flashsweep

A one-shot text reveal: a light travels left to right across a line, characters
brighten permanently into view as it crosses them, and each one flashes briefly at
the moment of crossing.

Characters stay revealed without any bookkeeping. The reveal envelope depends only on
`characterIndex - lightPos`, and `lightPos` only increases, so once the light has
passed a character its envelope is pinned at fully-revealed by the maths alone.

`Glint` is the same travelling light without the reveal: every character stays
fully visible and only the flash moves across, settling back to base at both
ends. It takes `progress` in 0-1 rather than a light position, so a sweep takes
the same wall-clock time regardless of how long the text is — what a one-shot UI
flourish wants, where `Line`'s characters-per-second pacing suits typing.

Colours come from a `Ramp{Base, Flash}`. Pick the pair for contrast, not just
taste: the glint is only as visible as the distance between them. A base that is
already at full brightness — white text, say — has no headroom left to flash *up*
into, and the result is close to invisible however long it runs. Warm mid-tone
text against a dark ground has plenty, and reads as briefly white-hot.

Direction matters too. On a dark background the flash should brighten; on a light
one it should deepen or saturate instead, since flashing toward white on white
just erases the text. `theme.LightDark` can supply a different `Flash` for each.

Give the enclosing style's own colour as `Base` — ideally the very same variable —
so the glint settles into the static style with no visible jump when it ends.

`FromColor` converts any `color.Color` (a lipgloss colour, say) into movy's `V3`,
so callers can feed `theme.LightDark` straight into either function.

Ported from the `text-reveal` example of the movy-go port vendored in
`../movy`, and so covered by the same MIT licence and copyright — see
`../movy/LICENSE`.

Colour is emitted as raw ANSI truecolor escapes rather than through lipgloss,
exactly as the original did. This composes correctly inside a lipgloss style:
`38;2` sets only the foreground, so an enclosing background survives, the single
trailing reset lands immediately before lipgloss's own, and `lipgloss.Width`
measures the result unchanged — so per-character colour costs no layout jitter
and no style allocation per character. Bubbletea parses the view into a cell
buffer and applies the terminal's colour profile, so these escapes get
downsampled along with everything else rather than bypassing it.
