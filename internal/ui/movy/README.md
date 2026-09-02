# movy (vendored)

Terminal graphics core — pixel/colour types, a half-block `RenderSurface`, sprites,
and a float-framebuffer glow/CRT post-fx stack. Renders to a plain Go string.
Standard library only; no dependencies, no CGo.

## Provenance

Derived from [`M64GitHub/movy`](https://github.com/M64GitHub/movy), a Zig terminal
graphics library, MIT licensed, Copyright (c) 2025 M64. See `LICENSE`.

## Local modifications

Upstream describes the port as a vertical slice rather than a complete port, so expect
gaps against the Zig original. Verify behaviour before building on any given part.
