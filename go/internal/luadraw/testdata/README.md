# LuaDraw complex coverage fixtures

This package contains the two complex LuaDraw test programs supplied by the
user. The files are copied byte-for-byte from the supplied attachments; the
manifest records only stable attachment names and complete SHA256 identifiers,
not machine-local attachment paths. No source-level patch or API normalization
was applied.

The 2D fixture covers graph2d construction, grid and axes, a damped Cartesian
plot, a Lissajous parametric curve with `decorations.markings`, pattern fill,
plotmarks, a filled polygon, and transparency. The 3D fixture covers a
cartesian surface, cutfacet level splitting, palette-colored shaded facets,
parametric curves with `arrows.meta`, a 3D circle, ball dots, math labels, and
a 2D polygon in a 3D scene.

The source is provided by the user for testing. No separate source license or
copyright claim is asserted here. The repository's MIT license covers the test
package as repository material; the vendored LuaDraw 3.7 implementation keeps
its own upstream license.

## Run

From the repository root:

```sh
cd go
GOCACHE=/tmp/dsh-gocache go test ./internal/luadraw -run 'TestLuaDrawComplex(2D|3D)$' -count=1
```

The tests verify both fixture SHA256 values before compiling. Each fixture is
compiled with both LuaLaTeX directly and the standalone Lua fallback path. A
missing external engine may be reported as an environment skip, but compile or
TikZ errors are test failures and are not swallowed. Generated PDF/TikZ output
is written beneath each test's temporary directory; the tests also emit a PNG
preview path when `COMPARE_FIGURE_FULL_LUADRAW_TEST=1` is set.
