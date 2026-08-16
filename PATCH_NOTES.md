# Mouse double-toggle fix

Fixes reasoning/tool mouse clicks being processed twice in Bubble Tea v2.

Root cause: View.OnMouse does not replace the original MouseMsg. Bubble Tea runs
renderer.onMouse(msg), asynchronously sends the returned Cmd result, and then still
calls Model.Update(msg) with the original mouse event. The previous TUI handled both
paths, causing a single physical click to toggle twice.

Changes:
- remove View.OnMouse routed mouse path
- remove routedMouseMsg
- keep raw MouseClickMsg / MouseWheelMsg / MouseMotionMsg handling synchronous
- keep MouseModeCellMotion
- add regression tests that OnMouse is nil and one raw reasoning click toggles once
