# flashsweep

A one-shot text reveal: a light travels left to right across a line, characters
brighten permanently into view as it crosses them, and each one flashes briefly at
the moment of crossing.

Characters stay revealed without any bookkeeping. The reveal envelope depends only on
`characterIndex - lightPos`, and `lightPos` only increases, so once the light has
passed a character its envelope is pinned at fully-revealed by the maths alone.

Ported from the `text-reveal` example of the movy-go port vendored in
`../movy`, and so covered by the same MIT licence and copyright — see
`../movy/LICENSE`.

Unlike the vendored core, this renders colour as raw ANSI truecolor escapes rather
than through lipgloss, exactly as the original did. That bypasses lipgloss's colour
profile downsampling, so output on a non-truecolor terminal is untested. Deciding
whether to route this through lipgloss belongs with the visual integration work.
