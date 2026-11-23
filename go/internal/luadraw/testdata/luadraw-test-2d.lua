-- 2D stress test: graph2d path + patterns/plotmarks/decorations runtime usage
local ld = luadraw
local M = ld.pt3d.M  -- pt3d.M works for 2D too (z defaults to 0); or use complex...
-- graph2d points can be given as complex numbers or (x,y); use ld.I(x,y) via cpx
local I = function(x, y) return ld.cpx.Z(x, y) end  -- Z = cartesian constructor (I is the imaginary unit)
local pi = math.pi
local cos, sin, exp = math.cos, math.sin, math.exp

local g = ld.graph:new{window = {-3, 3, -2, 2.2}, size = {13, 9}}  -- 2D graph class is ld.graph
g:Labelsize("small")

-- 1) grid (Dgrid takes {bottom-left, top-right}) + axes (Daxes takes {origin, xstep, ystep})
g:Dgrid({I(-3, -2), I(3, 2.2)}, {gridcolor = "gray!40", gridwidth = 2, nbsubdiv = {1, 1}})
g:Daxes({I(0, 0), 1, 1}, {arrows = "->"})
g:Dlabel("$x$", I(2.9, 0), {pos = "right"})
g:Dlabel("$y$", I(0, 2.1), {pos = "above"})

-- 2) function plot: damped oscillation (bounded by 1, stays in window)
g:Dcartesian(function(x) return exp(-math.abs(x))*cos(3*x) end,
  {x = {-3, 3}, nbdots = 300, draw_options = "blue, line width=1pt"})

-- 3) Lissajous parametric curve with decoration (decorations.markings runtime):
--    arrow marks along the path
g:Dparametric(function(t) return I(1.8*sin(3*t + pi/2), 0.9*sin(2*t)) end,
  {t = {0, 2*pi}, nbdots = 400,
   draw_options = "red, line width=0.9pt, postaction={decorate, decoration={markings, mark=between positions 0.15 and 0.9 step 0.25 with {\\arrow{Stealth[length=2mm]}}}}"})

-- 4) hatched region under a bump (patterns runtime)
g:Dcartesian(function(x) return exp(-x*x) end,
  {x = {-1.5, 1.5}, nbdots = 120, draw_options = "black!70, pattern=north east lines, pattern color=teal"})

-- 5) dots with different plotmarks (plotmarks runtime)
local pts = {}
for k = 0, 8 do
  pts[k+1] = I(-2.4 + 0.6*k, -1.5 + 0.4*((k % 3) - 1))
end
g:Ddots(pts, "mark=*, mark size=2pt, color=orange!80!black")
g:Ddots({I(-2.2, 1.6), I(-1.8, 1.75), I(-1.4, 1.65)}, "mark=square*, mark size=2.4pt, color=violet")

-- 6) filled polygon with semi-transparency (vertex near center -> radius ~0.5)
g:Dpolyline(ld.polyreg(I(2.2, 1.2), I(2.2 + 0.45, 1.2 + 0.2), 7), true,
  "fill=cyan!40, draw=blue, line width=0.8pt, fill opacity=0.6")

g:Show()
