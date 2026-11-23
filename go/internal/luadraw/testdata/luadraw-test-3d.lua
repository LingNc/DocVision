-- complex stress test for the luadraw2tikz pipeline
-- exercises: cartesian3d surfaces, cutfacet level-splitting, palette colors,
-- shaded facets (mShaded), axes box with grid, parametric 3d curves with
-- arrows.meta, 3d circle, ball dots, math labels, 2D polyreg in 3D scene
local ld = luadraw
local pt3d = ld.pt3d
local M, vecK = pt3d.M, pt3d.vecK
local cos, sin, pi = math.cos, math.sin, math.pi

local g = ld.graph3d:new{
  window3d = {-3,3, -3,3, -2,3},
  viewdir  = {35,55},
  size     = {13,11},
}
g:Labelsize("small")

-- 1) axes box with grid
g:Dboxaxes3d{grid=true, gridcolor="gray!45", fillcolor="black!8"}

-- 2) wave surface z = cos(u)*sin(v), split into palette-colored level bands
local S = ld.cartesian3d(function(u,v) return cos(u)*sin(v) end, -3,3,-3,3, {36,36})
local n = 6
local Colors = ld.getpalette(ld.palGasFlame, n+1, true)
local niv = {}
for k = 1, n do
  local S1
  S1, S = ld.cutfacet(S, {M(0,0,-1 + 2*k/n), -vecK}) -- S1 = below plane, S = above
  ld.insert(niv, {S1, {color=Colors[k], mode=ld.mShaded, edgewidth=0.4}})
end
ld.insert(niv, {S, {color=Colors[n+1], mode=ld.mShaded, edgewidth=0.4}})
g:Dmixfacet(table.unpack(niv))

-- 3) helix with arrow head (exercises arrows.meta)
g:Dparametric3d(
  function(t) return M(2*cos(t), 2*sin(t), -1.5 + t/(6*pi)*3) end,
  {t={0, 6*pi}, nbdots=240, draw_options="red, line width=1.1pt, -{Stealth[length=2.5mm]}"})

-- 4) blue circle on the top plane
g:Dcircle3d(M(0,0,1.5), 2, vecK, "blue, line width=1pt", false)

-- 5) endpoints: ball dots + math labels
g:Dballdots3d(M(2,0,-1.5), "red")
g:Dballdots3d(M(2,0,1.5),  "blue")
g:Dlabel3d("$A(2,0,-\\frac{3}{2})$", M(2,0,-1.5), {pos="below right"})
g:Dlabel3d("$B(2,0,\\frac{3}{2})$",  M(2,0,1.5),  {pos="above right"})
g:Dlabel3d("$z=\\cos x\\,\\sin y$", M(0,-2.6,1.2), {pos="below"})

-- 6) color scale: ball dot row on the x axis (palette reference)
for k = 1, n+1 do
  g:Dballdots3d(M(-3 + 6*k/(n+1), 3, -2), ld.rgb(Colors[k]))
end

-- 7) original red pentagon (2D polyreg in the 3D scene)
local P = ld.polyreg(0, ld.cpx.Z(2,3), 5)
g:Dpolyline(P, true, "green!50!black, line width=1pt")

g:Show()
