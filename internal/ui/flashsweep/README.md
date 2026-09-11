# flashsweep

Travelling-light text effects. A light crosses a line, characters brighten
permanently into view as it passes them, and each one flashes briefly at the
moment of crossing.

`Line` is that in its original form, running left to right.

Characters stay revealed without any bookkeeping. The reveal envelope depends only on
`characterIndex - lightPos`, and `lightPos` only increases, so once the light has
passed a character its envelope is pinned at fully-revealed by the maths alone.

`Glint` is the same travelling light without the reveal: every character stays
fully visible and only the flash moves across, settling back to base at both
ends. It takes `progress` in 0-1 rather than a light position, so a sweep takes
the same wall-clock time regardless of how long the text is — what a one-shot UI
flourish wants, where `Line`'s characters-per-second pacing suits typing.

`Reveal` is the same envelope folded in half: the text opens from its **centre
outward to both ends**, for a label that has to be noticed rather than merely
shown. It is configured rather than called — a `Style`, a `Ramp` and a `Speed` —
and like `Glint` it renders a frame for a progress in 0-1 instead of owning a
timer, so it drops into an animation loop the caller already runs.

```go
r := flashsweep.Reveal{Style: flashsweep.RevealBloom, Ramp: ramp}
line := r.Open(" ℹ press ? for help ", progress) // progress 0 -> 1
```

Four styles, all sharing the geometry and differing in what they do with colour:
`RevealMirror` is `Line` folded, `RevealPrism` gives the light a travelling hue,
`RevealBloom` opens fully lit and then cools into place, and `RevealSplit` starts
every rune stacked on the centre column and slides them apart. `RevealStyles`
lists them for a caller offering a choice, and `RevealMirror` is the zero value.

`Close` runs the entrance backwards, for a label that has to leave as
deliberately as it arrived. It is defined as `Open` with the progress reversed
rather than as a second implementation, so the two can never drift apart as a
style is tuned.

`Fill` is what makes a collapse land cleanly. Set it to a rune and the cells the
text has not claimed are drawn with it rather than left blank, so an entrance
*parts* something instead of appearing on a gap — a divider passes its own rule
rune and its rule colour, the text opens the rule, and the collapse closes the
rule over it. Without that the exit has to end by swapping letters back for
rule, which snaps. Set `FillColor` whenever `Fill` is set: its zero value is
black, which on most grounds reads as a hole.

Every style starts from a completely empty frame at progress 0, which is the
property a collapse depends on — a style that left one rune stranded at the
centre would leave it sitting inside a closed divider.

`Duration` scales with **half** the text, since both wavefronts travel at once —
which is why a `Reveal` is quicker than a `Line` over the same string at the same
speed. `Speed` left at zero means `DefaultRevealSpeed`.

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

`Breath` is the odd one out: no light travels, and nothing is revealed. It is the
brightness of a light that breathes, from 0 at rest to 1 at the top of a breath,
for a mark that should be noticed without asking to be looked at.

It is a pair of gaussians rather than a sine. A sine turns too sharply at both
ends, where a breath pauses, and the two halves differ: breathing in is quicker
than breathing out, so the light fills in about two seconds and empties in about
three. The fall's width is derived from the rise's rather than chosen, so a cycle
ends at exactly the value it began at and the wrap cannot be seen.

The period is five seconds, about twelve breaths a minute, which is a person at
rest. Modelled on the sleep light of an older Mac (US patent 6,658,577), at the
slower rate those machines actually used rather than the one the patent states.

`BreathAt` gives the colour directly, for a caller that wants to set a style
rather than do the mixing itself.

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
