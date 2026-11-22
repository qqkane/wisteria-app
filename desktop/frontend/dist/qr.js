// Minimal QR encoder (byte mode, EC level L, versions 1..20) — self-contained,
// no dependencies. Returns a boolean matrix (true = dark).
"use strict";
// Offline QR encoder for the share screen (byte mode, EC level L, v1..20).
// No dependencies and no network: the APK ships it as a plain asset.

// version: [totalCodewords, ecPerBlock, group1Blocks, group1DataCodewords, group2Blocks, group2DataCodewords]
const QR_BLOCKS_L = {
  1: [26, 7, 1, 19, 0, 0],
  2: [44, 10, 1, 34, 0, 0],
  3: [70, 15, 1, 55, 0, 0],
  4: [100, 20, 1, 80, 0, 0],
  5: [134, 26, 1, 108, 0, 0],
  6: [172, 18, 2, 68, 0, 0],
  7: [196, 20, 2, 78, 0, 0],
  8: [242, 24, 2, 97, 0, 0],
  9: [292, 30, 2, 116, 0, 0],
  10: [346, 18, 2, 68, 2, 69],
  11: [404, 20, 4, 81, 0, 0],
  12: [466, 24, 2, 92, 2, 93],
  13: [532, 26, 4, 107, 0, 0],
  14: [581, 30, 3, 115, 1, 116],
  15: [655, 22, 5, 87, 1, 88],
  16: [733, 24, 5, 98, 1, 99],
  17: [815, 28, 1, 107, 5, 108],
  18: [901, 30, 5, 120, 1, 121],
  19: [991, 28, 3, 113, 4, 114],
  20: [1085, 28, 3, 107, 5, 108],
};

const QR_ALIGN = {
  1: [], 2: [6, 18], 3: [6, 22], 4: [6, 26], 5: [6, 30], 6: [6, 34],
  7: [6, 22, 38], 8: [6, 24, 42], 9: [6, 26, 46], 10: [6, 28, 50],
  11: [6, 30, 54], 12: [6, 32, 58], 13: [6, 34, 62], 14: [6, 26, 46, 66],
  15: [6, 26, 48, 70], 16: [6, 26, 50, 74], 17: [6, 30, 54, 78],
  18: [6, 30, 56, 82], 19: [6, 30, 58, 86], 20: [6, 34, 62, 90],
};

// GF(256) tables, primitive polynomial 0x11D
const QR_EXP = new Uint8Array(512);
const QR_LOG = new Uint8Array(256);
(function () {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    QR_EXP[i] = x;
    QR_LOG[x] = i;
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  for (let i = 255; i < 512; i++) QR_EXP[i] = QR_EXP[i - 255];
})();

function gfMul(a, b) {
  if (a === 0 || b === 0) return 0;
  return QR_EXP[QR_LOG[a] + QR_LOG[b]];
}

function rsGenerator(degree) {
  let poly = [1];
  for (let i = 0; i < degree; i++) {
    const next = new Array(poly.length + 1).fill(0);
    for (let j = 0; j < poly.length; j++) {
      next[j] ^= poly[j];
      next[j + 1] ^= gfMul(poly[j], QR_EXP[i]);
    }
    poly = next;
  }
  return poly;
}

function rsEncode(data, ecLen) {
  const gen = rsGenerator(ecLen);
  const res = new Array(data.length + ecLen).fill(0);
  for (let i = 0; i < data.length; i++) res[i] = data[i];
  for (let i = 0; i < data.length; i++) {
    const factor = res[i];
    if (factor === 0) continue;
    for (let j = 0; j < gen.length; j++) {
      res[i + j] ^= gfMul(gen[j], factor);
    }
  }
  return res.slice(data.length);
}

function utf8Bytes(str) {
  const out = [];
  for (let i = 0; i < str.length; i++) {
    let c = str.charCodeAt(i);
    if (c < 0x80) out.push(c);
    else if (c < 0x800) {
      out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
    } else if (c >= 0xd800 && c <= 0xdbff && i + 1 < str.length) {
      const c2 = str.charCodeAt(++i);
      const cp = 0x10000 + ((c - 0xd800) << 10) + (c2 - 0xdc00);
      out.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
    } else {
      out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
    }
  }
  return out;
}

function dataCapacityBytes(ver) {
  const t = QR_BLOCKS_L[ver];
  return t[2] * t[3] + t[4] * t[5];
}

function chooseVersion(byteLen) {
  for (let v = 1; v <= 20; v++) {
    const lenBits = v < 10 ? 8 : 16;
    const need = Math.ceil((4 + lenBits + byteLen * 8) / 8);
    if (need <= dataCapacityBytes(v)) return v;
  }
  throw new Error("QR: данные слишком длинные");
}

function buildCodewords(ver, bytes) {
  const t = QR_BLOCKS_L[ver];
  const [, ecPer, g1n, g1d, g2n, g2d] = t;
  const cap = g1n * g1d + g2n * g2d;
  const lenBits = ver < 10 ? 8 : 16;
  const bits = [];
  const push = (val, n) => { for (let i = n - 1; i >= 0; i--) bits.push((val >> i) & 1); };
  push(4, 4);          // byte mode
  push(bytes.length, lenBits);
  for (const b of bytes) push(b, 8);
  // terminator + byte align
  const maxBits = cap * 8;
  for (let i = 0; i < 4 && bits.length < maxBits; i++) bits.push(0);
  while (bits.length % 8 !== 0) bits.push(0);
  const data = [];
  for (let i = 0; i < bits.length; i += 8) {
    let v = 0;
    for (let j = 0; j < 8; j++) v = (v << 1) | bits[i + j];
    data.push(v);
  }
  const pad = [0xec, 0x11];
  let pi = 0;
  while (data.length < cap) data.push(pad[pi++ % 2]);

  const blocks = [];
  let off = 0;
  for (let i = 0; i < g1n; i++) { blocks.push(data.slice(off, off + g1d)); off += g1d; }
  for (let i = 0; i < g2n; i++) { blocks.push(data.slice(off, off + g2d)); off += g2d; }
  const eccs = blocks.map((b) => rsEncode(b, ecPer));

  const out = [];
  const maxData = Math.max(g1d, g2d || 0);
  for (let i = 0; i < maxData; i++) {
    for (let b = 0; b < blocks.length; b++) {
      if (i < blocks[b].length) out.push(blocks[b][i]);
    }
  }
  for (let i = 0; i < ecPer; i++) {
    for (let b = 0; b < eccs.length; b++) out.push(eccs[b][i]);
  }
  return out;
}

function makeMatrix(ver) {
  const size = ver * 4 + 17;
  const m = [];
  const fn = [];
  for (let y = 0; y < size; y++) {
    m.push(new Array(size).fill(false));
    fn.push(new Array(size).fill(false));
  }
  const set = (x, y, dark) => { m[y][x] = !!dark; fn[y][x] = true; };

  // finder patterns + separators
  const finder = (cx, cy) => {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const x = cx + dx, y = cy + dy;
        if (x < 0 || y < 0 || x >= size || y >= size) continue;
        const d = Math.max(Math.abs(dx), Math.abs(dy));
        set(x, y, d !== 2 && d !== 4);
      }
    }
  };
  finder(3, 3); finder(size - 4, 3); finder(3, size - 4);

  // timing patterns
  for (let i = 8; i < size - 8; i++) {
    set(i, 6, i % 2 === 0);
    set(6, i, i % 2 === 0);
  }

  // alignment patterns
  const pos = QR_ALIGN[ver];
  for (let i = 0; i < pos.length; i++) {
    for (let j = 0; j < pos.length; j++) {
      const cx = pos[i], cy = pos[j];
      if ((i === 0 && j === 0) || (i === 0 && j === pos.length - 1) || (i === pos.length - 1 && j === 0)) continue;
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) {
          const d = Math.max(Math.abs(dx), Math.abs(dy));
          set(cx + dx, cy + dy, d !== 1);
        }
      }
    }
  }

  // dark module + reserve format areas
  set(8, size - 8, true);
  for (let i = 0; i < 9; i++) {
    if (!fn[8][i]) set(i, 8, false);
    if (!fn[i][8]) set(8, i, false);
  }
  for (let i = 0; i < 8; i++) {
    if (!fn[8][size - 1 - i]) set(size - 1 - i, 8, false);
    if (!fn[size - 1 - i][8]) set(8, size - 1 - i, false);
  }
  // version info (v>=7)
  if (ver >= 7) {
    for (let i = 0; i < 6; i++) {
      for (let j = 0; j < 3; j++) {
        set(size - 11 + j, i, false);
        set(i, size - 11 + j, false);
      }
    }
  }
  return { m, fn, size };
}

function formatBits(ecBits, mask) {
  // ecBits for L = 01
  let data = (ecBits << 3) | mask;
  let rem = data;
  for (let i = 0; i < 10; i++) rem = (rem << 1) ^ (((rem >> 9) & 1) * 0x537);
  const bits = ((data << 10) | rem) ^ 0x5412;
  return bits;
}

function versionBits(ver) {
  let rem = ver;
  for (let i = 0; i < 12; i++) rem = (rem << 1) ^ (((rem >> 11) & 1) * 0x1f25);
  return (ver << 12) | rem;
}

function placeFormat(m, fn, size, mask) {
  const bits = formatBits(1, mask); // EC L
  const bit = (i) => ((bits >> i) & 1) === 1;
  // Vertical copy, column 8: bits 0..5 rows 0..5, bit 6 row 7, bit 7 row 8,
  // bits 8..14 rows size-7..size-1.
  for (let i = 0; i < 6; i++) m[i][8] = bit(i);
  m[7][8] = bit(6);
  m[8][8] = bit(7);
  for (let i = 8; i < 15; i++) m[size - 15 + i][8] = bit(i);
  // Horizontal copy, row 8: bits 0..7 columns size-1..size-8, bit 8 column 7,
  // bits 9..14 columns 5..0.
  for (let i = 0; i < 8; i++) m[8][size - 1 - i] = bit(i);
  m[8][7] = bit(8);
  for (let i = 9; i < 15; i++) m[8][14 - i] = bit(i);
  m[size - 8][8] = true; // dark module
}

function placeVersion(m, size, ver) {
  if (ver < 7) return;
  const bits = versionBits(ver);
  for (let i = 0; i < 18; i++) {
    const b = ((bits >> i) & 1) === 1;
    const a = Math.floor(i / 3);
    const bb = i % 3;
    m[size - 11 + bb][a] = b;
    m[a][size - 11 + bb] = b;
  }
}

function maskFn(mask, x, y) {
  switch (mask) {
    case 0: return (x + y) % 2 === 0;
    case 1: return y % 2 === 0;
    case 2: return x % 3 === 0;
    case 3: return (x + y) % 3 === 0;
    case 4: return (Math.floor(y / 2) + Math.floor(x / 3)) % 2 === 0;
    case 5: return ((x * y) % 2) + ((x * y) % 3) === 0;
    case 6: return (((x * y) % 2) + ((x * y) % 3)) % 2 === 0;
    default: return (((x + y) % 2) + ((x * y) % 3)) % 2 === 0;
  }
}

function penalty(m, size) {
  let score = 0;
  // N1: runs of 5+
  for (let y = 0; y < size; y++) {
    let run = 1;
    for (let x = 1; x < size; x++) {
      if (m[y][x] === m[y][x - 1]) run++;
      else { if (run >= 5) score += 3 + (run - 5); run = 1; }
    }
    if (run >= 5) score += 3 + (run - 5);
  }
  for (let x = 0; x < size; x++) {
    let run = 1;
    for (let y = 1; y < size; y++) {
      if (m[y][x] === m[y - 1][x]) run++;
      else { if (run >= 5) score += 3 + (run - 5); run = 1; }
    }
    if (run >= 5) score += 3 + (run - 5);
  }
  // N2: 2x2 blocks
  for (let y = 0; y < size - 1; y++) {
    for (let x = 0; x < size - 1; x++) {
      const c = m[y][x];
      if (c === m[y][x + 1] && c === m[y + 1][x] && c === m[y + 1][x + 1]) score += 3;
    }
  }
  // N3: finder-like 1:1:3:1:1 with 4 light
  const pat = [true, false, true, true, true, false, true];
  const checkLine = (get, len) => {
    for (let i = 0; i + 11 <= len; i++) {
      let ok = true;
      for (let j = 0; j < 7; j++) if (get(i + j) !== pat[j]) { ok = false; break; }
      if (!ok) continue;
      let light1 = true, light2 = true;
      for (let j = 0; j < 4; j++) {
        if (get(i - 1 - j) !== false) { light1 = false; break; }
      }
      for (let j = 0; j < 4; j++) {
        if (get(i + 7 + j) !== false) { light2 = false; break; }
      }
      if (i - 4 < 0) light1 = false;
      if (i + 11 > len) light2 = false;
      if (light1 || light2) score += 40;
    }
  };
  const at = (x, y) => (x >= 0 && y >= 0 && x < size && y < size ? m[y][x] : false);
  for (let y = 0; y < size; y++) checkLine((i) => at(i, y), size);
  for (let x = 0; x < size; x++) checkLine((i) => at(x, i), size);
  // N4: dark ratio
  let dark = 0;
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) if (m[y][x]) dark++;
  const pct = (dark * 100) / (size * size);
  score += Math.floor(Math.abs(pct - 50) / 5) * 10;
  return score;
}

function qrMatrix(text, forceMask) {
  const bytes = utf8Bytes(text);
  const ver = chooseVersion(bytes.length);
  const codewords = buildCodewords(ver, bytes);
  const { m, fn, size } = makeMatrix(ver);

  // place data
  let bitIdx = 0;
  const totalBits = codewords.length * 8;
  const bitAt = (i) => ((codewords[i >> 3] >> (7 - (i & 7))) & 1) === 1;
  let upward = true;
  for (let right = size - 1; right > 0; right -= 2) {
    if (right === 6) right = 5;
    for (let i = 0; i < size; i++) {
      const y = upward ? size - 1 - i : i;
      for (let c = 0; c < 2; c++) {
        const x = right - c;
        if (fn[y][x]) continue;
        let v = false;
        if (bitIdx < totalBits) v = bitAt(bitIdx);
        bitIdx++;
        m[y][x] = v;
      }
    }
    upward = !upward;
  }

  // choose mask (forceMask is for tests / deterministic rendering)
  let best = 0, bestScore = Infinity;
  const base = m.map((row) => row.slice());
  const maskList = forceMask == null ? [0,1,2,3,4,5,6,7] : [forceMask];
  for (const mask of maskList) {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        if (!fn[y][x] && maskFn(mask, x, y)) m[y][x] = !base[y][x];
        else if (!fn[y][x]) m[y][x] = base[y][x];
      }
    }
    placeFormat(m, fn, size, mask);
    placeVersion(m, size, ver);
    const s = penalty(m, size);
    if (s < bestScore) { bestScore = s; best = mask; }
  }
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      m[y][x] = !fn[y][x] && maskFn(best, x, y) ? !base[y][x] : base[y][x];
    }
  }
  placeFormat(m, fn, size, best);
  placeVersion(m, size, ver);
  return { size, ver, mask: best, matrix: m };
}

function qrSvg(text, opts) {
  const r = qrMatrix(text);
  const quiet = (opts && opts.quiet != null) ? opts.quiet : 4;
  const side = r.size + quiet * 2;
  // Merge horizontal runs into single path commands: one <path> keeps the
  // SVG small (a 61x61 symbol is ~3.7k modules).
  let d = "";
  for (let y = 0; y < r.size; y++) {
    let x = 0;
    while (x < r.size) {
      if (!r.matrix[y][x]) { x++; continue; }
      let run = 1;
      while (x + run < r.size && r.matrix[y][x + run]) run++;
      d += "M" + (x + quiet) + " " + (y + quiet) + "h" + run + "v1h-" + run + "z";
      x += run;
    }
  }
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ' + side + ' ' + side +
    '" shape-rendering="crispEdges" role="img" aria-label="QR">' +
    '<rect width="' + side + '" height="' + side + '" fill="#ffffff"/>' +
    '<path d="' + d + '" fill="#000000"/></svg>';
  return { svg: svg, version: r.ver, size: r.size, mask: r.mask };
}

const QR = { qrMatrix: qrMatrix, qrSvg: qrSvg };
if (typeof module !== "undefined" && module.exports) module.exports = QR;
if (typeof window !== "undefined") window.WisteriaQR = QR;
