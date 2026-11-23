# LuaDraw integration provenance

The runtime, documentation index/navigation, frequency cache, standalone Lua
launcher and complex 2D/3D fixtures were ported from compare-figure 0.2.1
(MIT; see LICENSE.compare-figure). Local adaptations embed resources, share
indexes across DocVision sessions, and preserve process-group timeout cleanup.

resources.tar.gz contains the unmodified LuaDraw 3.7 `files/`, `doc/` and
README, together with compare-figure's `scripts/luadraw_runtime.lua`.
LuaDraw is by Patrick Fradin, distributed under LPPL 1.3c or later:
https://github.com/pfradin/luadraw
https://www.latex-project.org/lppl/lppl-1-3c/

Resource extraction uses a content-addressed user cache; documentation indexes
and frequently consulted excerpts are shared per DocVision project. No runtime
resource is resolved through a developer's local checkout.

The LuaDraw complex test fixtures are synthetic library coverage examples,
not document samples. Their upstream SHA-256 checks are retained.
