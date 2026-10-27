# Generate the VideoHelper "three lanes" logo kit SVGs from fixed geometry + outlined wordmark JSON.
import json, os, sys

out = sys.argv[1]; os.makedirs(out, exist_ok=True)
INK, PAPER, RED = "#0f0e15", "#f3ede2", "#f43d3f"

# (left start, left end, right start, right end, y); lanes top→bottom = visual, speech, audio
REGULAR = dict(lanes=[(24, 116, 164, 200, 54), (72, 116, 164, 232, 110), (44, 116, 164, 212, 166)], h=36, ph=(128, 22, 24, 212))
SMALL = dict(lanes=[(16, 96, 168, 216, 32), (48, 96, 168, 240, 104), (32, 96, 168, 224, 176)], h=48, ph=(116, 8, 32, 240))

def rect(x, y, w, h, fill):
    return f'<rect x="{x:g}" y="{y:g}" width="{w:g}" height="{h:g}" rx="{min(w, h) / 2:g}" fill="{fill}"/>'

def symbol(g, lane, head, inset=0):
    h = g["h"] - 2 * inset; parts = []
    for ls, le, rs, re, y in g["lanes"]:
        parts += [rect(ls + inset, y + inset, le - ls - 2 * inset, h, lane), rect(rs + inset, y + inset, re - rs - 2 * inset, h, lane)]
    x, y, w, hh = g["ph"]
    parts.append(rect(x + inset, y + inset, w - 2 * inset, hh - 2 * inset, head))
    return "\n  ".join(parts)

def svg(name, vb, body, title="VideoHelper logo"):
    with open(os.path.join(out, name), "w") as f:
        f.write(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}">\n  <title>{title}</title>\n  {body}\n</svg>\n')

wm84 = json.load(open(sys.argv[2])); wm56 = json.load(open(sys.argv[3]))

def word(wm, x, base, fill):
    return f'<path transform="translate({x:g} {base:g})" fill="{fill}" d="{wm["d"]}"/>'

SCHEMES = {"color": (INK, RED, INK, 0), "reversed": (PAPER, RED, PAPER, 1), "black": ("#000", "#000", "#000", 0)}
for scheme, (lane, head, text, inset) in SCHEMES.items():
    svg(f"symbol-{scheme}.svg", "0 0 256 256", symbol(REGULAR, lane, head, inset))
    svg(f"symbol-small-{scheme}.svg", "0 0 256 256", symbol(SMALL, lane, head, inset))
    # horizontal: cap height 84 centred on the mark, gap after the mark = 1.5 lane heights
    x0 = 232 + 54; w = wm84["bounds"][2]
    svg(f"horizontal-{scheme}.svg", f"24 0 {x0 + w - 24 + 4:.0f} 256", symbol(REGULAR, lane, head, inset) + "\n  " + word(wm84, x0, 170, text))
    # stacked: mark above, wordmark (cap 56) centred, gap 44 below the playhead
    w56 = wm56["bounds"][2] - wm56["bounds"][0]; W = round(w56 + 8)
    sx = (W - 256) / 2; base = 234 + 44 + 56
    body = f'<g transform="translate({sx:g} 0)">\n  {symbol(REGULAR, lane, head, inset)}\n  </g>\n  ' + word(wm56, (W - w56) / 2 - wm56["bounds"][0], base, text)
    svg(f"stacked-{scheme}.svg", f"0 0 {W} {base + 24}", body)
    b = wm84["bounds"]
    svg(f"wordmark-{scheme}.svg", f"0 {b[1] - 4:.0f} {b[2] + 8:.0f} {b[3] - b[1] + 8:.0f}", word(wm84, 0, 0, text))

# favicon tile: small cut, reversed colours, on an ink rounded square so it reads on light and dark tabs
svg("symbol-small-tile.svg", "0 0 256 256", f'<rect width="256" height="256" rx="56" fill="{INK}"/>\n  <g transform="translate(28 28) scale(0.78125)">\n  {symbol(SMALL, PAPER, RED, 1)}\n  </g>')
