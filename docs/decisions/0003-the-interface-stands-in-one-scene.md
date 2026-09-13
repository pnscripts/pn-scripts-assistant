# ADR 0003 — The interface stands in one scene

**Status:** accepted · **Date:** 2026-09-13

## The real question

"The whole UI in three.js" can mean three different programs:

1. **Everything drawn by WebGL.** Every letter, field and list is a mesh.
2. **One scene, real panels.** three.js draws a world, and the page's own
   panels stand in it, placed by the same camera.
3. **A 3D backdrop.** A scene behind flat panels, and the charts redrawn in it.

Its owner chose the second. This records why that is the one that works, and
the rules that keep it working.

## Why not everything in WebGL

The interface is twenty thousand lines of forms, lists, transcripts and
settings. On a canvas, typing, selecting, copying, scrolling, focus, the text
cursor and being read aloud would all be rebuilt by hand, and every one would be
worse than the browser's. The machine it was built on has no graphics card, and
the page was already the most expensive part of the program.

## How it works

| Piece | Where |
|---|---|
| The world: floor, dust, the light, a cover for every panel | `internal/stage/stage.js`, three.js |
| three.js itself, shared by the program and setup | `internal/stage/three.module.js`, served at `/stage/` |
| The panels | the page's own elements, where the layout put them |

- **Panels on a ring.** Every view (or every setup step) stands on a ring
  around the brain's light, in the order the column lists them. Going
  somewhere turns the camera round the ring to face that panel, stepping back
  as it turns.
- **The same camera for both.** The panels are moved with CSS `matrix3d`
  transforms worked out from the three.js camera, the arithmetic of
  CSS3DRenderer. The panels are not moved into a container of their own,
  because a panel that has been picked up and put down again loses its
  scroll position and its canvases' sizes.
- **One unit, one pixel.** The camera looks through the middle of the frame.
  At rest it stands exactly as far from the panel as makes a world unit one
  screen pixel, so the panel at rest covers its frame to the pixel.
- **Flat at rest.** Once a turn ends, the transforms are removed. The panel in
  front is ordinary layout, sharp and as cheap to scroll as before. The world
  is drawn once and is not drawn again until something changes.
  `window.brainStageFrames` counts every frame, so this can be checked.
- **The page decides, the stage follows.** Every way of going somewhere ends
  in a panel's `hidden` attribute changing, and a MutationObserver on that
  attribute is the only place the stage hooks in. The code that navigates does
  not know the stage exists.
- **Stepping back.** A button on the bar pulls the camera back to show every
  panel. Drag to turn, scroll to zoom, arrows and Enter, or click a panel to
  go there.
- **No column of places.** While the stage runs, the column of places is not
  shown. You get around with the turner on the bar (‹ name ›, or Alt and an
  arrow) and by stepping back. The step-back button carries the total of the
  counts the column used to show beside each place.
- **The column is kept, hidden.** A dozen scripts send somebody to a panel by
  clicking its entry, and the stage does the same. The brain's name moved into
  the top bar and the voice into the bottom bar, in the markup, so there is
  one arrangement with or without the stage. The search box and the status
  tiles went at the same time; what they showed is on the panels.

## Rules

- **Nothing without WebGL changes.** Every style hangs off `.has-stage`, which
  is set only after the renderer has started. Without WebGL, or after the
  context is lost, the page is the flat one it was.
- **Nothing is invented.** A panel out in the world shows only its mark, its
  name and the count its column entry shows. The light takes the status colour
  and the measured sound level the core uses, from the same place.
- **Reduced motion is honoured.** Turns take no time.
- **The bars are glass in front of the world.** A panel turning past goes
  behind them, not across their text.

## Consequences

- One more WebGL context on the page (three in all), and a full-window canvas
  that is still at rest.
- The panels are still ordinary HTML, so nothing about how a view is written
  changed. A new view that has a column entry takes its place on the ring
  without any change to the stage.
- The idle cost has been checked by counting frames. It has not been profiled
  in the real window on the machine it was built on: the pane throttles
  animation frames, which makes any measurement taken there meaningless.
