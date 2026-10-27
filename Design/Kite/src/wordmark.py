# Outline a string with HarfBuzz shaping (kerning) at a variable-font instance; print SVG path + metrics as JSON.
import json, sys
import uharfbuzz as hb
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.boundsPen import BoundsPen

font_path, text, wght, cap_px, track = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4]), float(sys.argv[5])
blob = hb.Blob.from_file_path(font_path)
face = hb.Face(blob); f = hb.Font(face); f.set_variations({"wght": wght})
buf = hb.Buffer(); buf.add_str(text); buf.guess_segment_properties()
hb.shape(f, buf, {"kern": True, "liga": False})

tt = TTFont(font_path)
from fontTools.varLib.instancer import instantiateVariableFont
tt = instantiateVariableFont(tt, {"wght": wght, "wdth": 100})
gs = tt.getGlyphSet(); order = tt.getGlyphOrder()
cap = tt["OS/2"].sCapHeight
s = cap_px / cap
pen = SVGPathPen(gs, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
bp = BoundsPen(gs)
x = 0
for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
    g = order[info.codepoint]
    # flip y: font units are y-up
    t = (s, 0, 0, -s, (x + pos.x_offset) * s, 0)
    gs[g].draw(TransformPen(pen, t)); gs[g].draw(TransformPen(bp, t))
    x += pos.x_advance + track / s
print(json.dumps({"d": pen.getCommands(), "bounds": bp.bounds, "cap": cap_px}))
