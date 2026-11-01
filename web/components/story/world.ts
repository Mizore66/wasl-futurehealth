import * as THREE from "three";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import { RoundedBoxGeometry } from "three/addons/geometries/RoundedBoxGeometry.js";
import type { Kit } from "./kit";
import { CITY, COLOR } from "./palette";

/**
 * The miniature city: one road from Fatima's clinic in the west to the hospital in the east, with
 * an open stretch of sand in the middle. World units, y up; one figure is a little over one unit tall.
 */

type V3 = readonly [number, number, number];

/** One bevel radius for every block and building, so it all reads as one cut-foam model. */
export const BEVEL = 0.09;

export const PLACES = {
  /** The clinic's arched door, in its south face. */
  clinicDoor: [4, 0, 1.5],
  /** The window the nurse calls from. */
  window: [8.4, 1.55, 1.5],
  nurse: [8.4, 0.55, 0.55],
  /** The hospital's door, under the canopy. */
  hospitalDoor: [72, 0, 1.5],
  /** The record slab on the hospital wall, beside the door. */
  slab: [72, 1.45, 1.58],
  /** The middle of the open sand. */
  gap: [48.5, 0, 5],
} as const satisfies Record<string, V3>;

/** The road, west to east. Fatima, the thread and the receipt all travel it. */
const ROAD: readonly [number, number][] = [
  [4, 2.7],
  [4.5, 5],
  [7.5, 6.7],
  [14, 6.5],
  [21, 7.5],
  [28, 6.6],
  [35, 7.4],
  [42, 7],
  [49, 7.4],
  [56, 6.8],
  [62, 7.5],
  [67, 6.8],
  [70.6, 5.4],
  [72, 3],
];

export function makeRoad(): THREE.CatmullRomCurve3 {
  const c = new THREE.CatmullRomCurve3(
    ROAD.map(([x, z]) => new THREE.Vector3(x, 0, z)),
    false,
    "centripetal",
  );
  c.arcLengthDivisions = 600;
  return c;
}

/** Arc-length position (0..1) on the road nearest a point. */
export function uNear(curve: THREE.Curve<THREE.Vector3>, x: number, z: number): number {
  let best = 0;
  let bestD = Infinity;
  const p = new THREE.Vector3();
  for (let i = 0; i <= 800; i++) {
    curve.getPointAt(i / 800, p);
    const d = (p.x - x) ** 2 + (p.z - z) ** 2;
    if (d < bestD) [best, bestD] = [i / 800, d];
  }
  return best;
}

/** Deterministic scatter, so the city is the same on every visit. */
export function rand(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s / 2 ** 32;
  };
}

type Rect = { x0: number; x1: number; z0: number; z1: number };

const CLINIC: Rect & { h: number } = { x0: -3, x1: 11, z0: -6.5, z1: 1.5, h: 3.2 };
/** The hospital's masses: podium, tower, raised wing. City blocks keep clear of all of them. */
const PODIUM: Rect & { h: number } = { x0: 62.5, x1: 78.5, z0: -8, z1: 1.5, h: 4.4 };
const TOWER: Rect & { h: number } = { x0: 65.5, x1: 76.5, z0: -11, z1: -2.2, h: 13.2 };
const WING: Rect & { y0: number; y1: number } = { x0: 78.5, x1: 84, z0: -7, z1: 1.2, y0: 2.8, y1: 7.2 };
const HOSPITAL: Rect[] = [PODIUM, TOWER, WING];
const GAP = { x0: 38.5, x1: 58.5 };

const TONES = CITY.map((c) => new THREE.Color(c));
const HEIGHTS = [0.7, 1.2, 1.8, 2.6, 3.6];
const WINDOW = new THREE.Color("#4a5560");
const ROOF = new THREE.Color("#8d8578");
const LEAF = new THREE.Color(COLOR.leaf);
const LEAF_DARK = new THREE.Color(COLOR.leafDark);
const TRUNK = new THREE.Color(COLOR.trunk);

function colored(g: THREE.BufferGeometry, c: THREE.Color): THREE.BufferGeometry {
  if (g.index) g = g.toNonIndexed();
  const n = g.attributes.position.count;
  const a = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) c.toArray(a, i * 3);
  g.setAttribute("color", new THREE.BufferAttribute(a, 3));
  g.deleteAttribute("uv");
  return g;
}

/** A box, placed by its centre and painted, ready to merge into a vertex-coloured mesh. */
function box(w: number, h: number, d: number, x: number, y: number, z: number, c: THREE.Color): THREE.BufferGeometry {
  return colored(new THREE.BoxGeometry(w, h, d).translate(x, y, z), c);
}

/** Window bands or a window grid on the faces toward the road, a parapet, sometimes a rooftop box. */
function blockDetail(w: number, h: number, d: number, tone: THREE.Color, r: () => number, mobile: boolean): THREE.BufferGeometry[] {
  const out: THREE.BufferGeometry[] = [];
  const glass = tone.clone().lerp(WINDOW, 0.62);
  const floors = Math.max(1, Math.floor((h - 0.25) / 0.72));
  const grid = !mobile && r() < 0.45 && h > 1;
  for (let f = 0; f < floors && h > 0.9; f++) {
    const y = 0.5 + f * 0.72 - h / 2;
    if (grid) {
      const n = Math.max(2, Math.floor(w / 0.62));
      for (let i = 0; i < n; i++) out.push(box(0.3, 0.34, 0.03, (i + 0.5 - n / 2) * ((w - 0.3) / n), y, d / 2, glass));
    } else {
      out.push(box(w - 0.34, 0.24, 0.03, 0, y, d / 2, glass));
      if (!mobile) out.push(box(0.03, 0.24, d - 0.34, -w / 2, y, 0, glass), box(0.03, 0.24, d - 0.34, w / 2, y, 0, glass));
    }
  }
  const lip = tone.clone().multiplyScalar(0.84);
  const top = h / 2 + 0.05;
  out.push(box(w, 0.12, 0.09, 0, top, d / 2 - 0.06, lip), box(w, 0.12, 0.09, 0, top, -d / 2 + 0.06, lip), box(0.09, 0.12, d, w / 2 - 0.06, top, 0, lip), box(0.09, 0.12, d, -w / 2 + 0.06, top, 0, lip));
  if (r() < 0.4 && h > 1) out.push(box(0.55 + r() * 0.5, 0.3, 0.55, (r() - 0.5) * w * 0.4, h / 2 + 0.15, (r() - 0.5) * d * 0.4, ROOF));
  return out;
}

/** A palm, its foot at the origin: a leaning trunk and a crown of blades. */
function palm(height: number): THREE.BufferGeometry[] {
  const out = [colored(new THREE.CylinderGeometry(0.05, 0.085, height, 6).translate(0, height / 2, 0), TRUNK)];
  for (let i = 0; i < 7; i++) {
    const a = (i / 7) * Math.PI * 2;
    const blade = new THREE.ConeGeometry(0.17, 1.05, 4)
      .scale(1, 1, 0.24)
      .translate(0, 0.52, 0)
      .rotateZ(-1.0 - (i % 2) * 0.32)
      .rotateY(a)
      .translate(0, height, 0);
    out.push(colored(blade, i % 2 ? LEAF_DARK : LEAF));
  }
  return out;
}

/** A round street tree, its foot at the origin. */
function tree(height: number, radius: number): THREE.BufferGeometry[] {
  return [
    colored(new THREE.CylinderGeometry(0.05, 0.07, height, 5).translate(0, height / 2, 0), TRUNK),
    colored(new THREE.IcosahedronGeometry(radius, 1).scale(1, 1.1, 1).translate(0, height + radius * 0.7, 0), LEAF),
  ];
}

export interface Chunk {
  near: THREE.Mesh;
  far: THREE.Mesh;
  shadow: THREE.Mesh;
  x: number;
}

/**
 * The city blocks, cut into chunks along x. Each chunk has a detailed (near) and a plain (far) version
 * of its blocks, one merged vertex-coloured mesh each, and its contact shadows. `wide` is the whole
 * plain city as one mesh, for the shots that see all of it at once.
 */
export function buildCity(kit: Kit, road: THREE.CatmullRomCurve3, mobile: boolean): { chunks: Chunk[]; wide: THREE.Mesh } {
  const r = rand(11);
  const roadPts = road.getSpacedPoints(300);
  const nearRoad = (x: number, z: number, pad: number) => roadPts.some((p) => Math.abs(p.x - x) < pad && Math.abs(p.z - z) < pad);
  const inRect = (x: number, z: number, rc: Rect, pad: number) => x > rc.x0 - pad && x < rc.x1 + pad && z > rc.z0 - pad && z < rc.z1 + pad;
  const CHUNK = 12;
  const bins = new Map<number, { near: THREE.BufferGeometry[]; far: THREE.BufferGeometry[]; shadow: THREE.BufferGeometry[] }>();
  const binOf = (x: number) => {
    const key = Math.floor(x / CHUNK);
    return bins.get(key) ?? bins.set(key, { near: [], far: [], shadow: [] }).get(key)!;
  };
  const shadowGeo = kit.geo.shadow;
  const step = mobile ? 4.6 : 4;

  for (let gx = -18; gx <= 96; gx += step) {
    for (let gz = -22; gz <= 22; gz += step) {
      if (r() < 0.16) continue;
      const w = 1.8 + r() * 1.7;
      const d = 1.8 + r() * 1.7;
      const x = gx + (r() - 0.5) * 0.9;
      const z = gz + (r() - 0.5) * 0.9;
      const reach = Math.max(w, d) / 2;
      if (x > GAP.x0 - reach && x < GAP.x1 + reach && z > -10) continue;
      if (nearRoad(x, z, 1.7 + reach)) continue;
      if (inRect(x, z, CLINIC, 1.8 + reach)) continue;
      if (HOSPITAL.some((h) => inRect(x, z, h, 1.8 + reach))) continue;
      // Keep the forecourts open: the street before each door and the hospital's waiting people.
      if (z > -2 && ((z < 18 && x > -4 && x < 14) || (z < 20 && x > 61 && x < 88))) continue;
      // The blocks get lower toward the road, so the walk stays in view.
      const tall = Math.abs(z - 7) < 6 ? 2 : 5;
      const h = HEIGHTS[Math.floor(r() * tall)];
      const tone = TONES[Math.floor(r() * TONES.length)];
      const yaw = (r() - 0.5) * 0.12;
      const m = new THREE.Matrix4().makeRotationY(yaw).setPosition(x, h / 2, z);
      const bin = binOf(x);
      bin.near.push(colored(new RoundedBoxGeometry(w, h, d, 1, BEVEL), tone).applyMatrix4(m));
      for (const g of blockDetail(w, h, d, tone, r, mobile)) bin.near.push(g.applyMatrix4(m));
      bin.far.push(colored(new THREE.BoxGeometry(w, h, d), tone).applyMatrix4(m));
      const s = shadowGeo.clone().applyMatrix4(new THREE.Matrix4().makeScale(w + 1.6, 1, d + 1.6).setPosition(x + 0.25, 0.018, z + 0.35));
      bin.shadow.push(s);
    }
  }

  // A few palms and trees along the road, near chunks only. The open sand stays open.
  const every = mobile ? 7.5 : 4.6;
  const length = road.getLength();
  const p = new THREE.Vector3();
  const t = new THREE.Vector3();
  for (let at = 13; at < length - 16; at += every) {
    road.getPointAt(at / length, p);
    if (p.x > GAP.x0 - 2 && p.x < GAP.x1 + 2) continue;
    road.getTangentAt(at / length, t);
    const side = r() < 0.55 ? 1 : -1;
    const off = 1.6 + r() * 0.5;
    const x = p.x - t.z * off * side;
    const z = p.z + t.x * off * side;
    const parts = r() < 0.5 ? palm(1.5 + r() * 0.6) : tree(0.75 + r() * 0.3, 0.42 + r() * 0.16);
    const m = new THREE.Matrix4().setPosition(x, 0, z);
    const bin = binOf(x);
    for (const g of parts) bin.near.push(g.applyMatrix4(m));
    bin.shadow.push(shadowGeo.clone().applyMatrix4(new THREE.Matrix4().makeScale(1.5, 1, 1.5).setPosition(x, 0.018, z)));
  }

  const chunks: Chunk[] = [];
  for (const [key, bin] of bins) {
    const mk = (list: THREE.BufferGeometry[], mat: THREE.Material) => {
      for (const g of list) g.deleteAttribute("uv");
      const mesh = new THREE.Mesh(mergeGeometries(list)!, mat);
      mesh.matrixAutoUpdate = false;
      for (const g of list) g.dispose();
      return mesh;
    };
    const shadow = new THREE.Mesh(mergeGeometries(bin.shadow)!, kit.mat.shadow);
    shadow.matrixAutoUpdate = false;
    shadow.renderOrder = 1;
    for (const g of bin.shadow) g.dispose();
    chunks.push({ near: mk(bin.near, kit.mat.city), far: mk(bin.far, kit.mat.city), shadow, x: key * CHUNK + CHUNK / 2 });
  }
  const wide = new THREE.Mesh(mergeGeometries(chunks.map((c) => c.far.geometry))!, kit.mat.city);
  wide.matrixAutoUpdate = false;
  return { chunks, wide };
}

/** An extruded wall with holes, standing in the xy plane, its face toward +z. */
function wall(width: number, height: number, depth: number, holes: THREE.Path[]): THREE.BufferGeometry {
  const s = new THREE.Shape();
  s.moveTo(-width / 2, 0);
  s.lineTo(width / 2, 0);
  s.lineTo(width / 2, height);
  s.lineTo(-width / 2, height);
  s.closePath();
  s.holes.push(...holes);
  const g = new THREE.ExtrudeGeometry(s, {
    depth: depth - BEVEL * 2,
    bevelEnabled: true,
    bevelThickness: BEVEL,
    bevelSize: BEVEL,
    bevelSegments: 2,
    curveSegments: 16,
  });
  g.translate(0, 0, -depth / 2 + BEVEL);
  return g;
}

function arch(cx: number, width: number, height: number): THREE.Path {
  const r = width / 2;
  const p = new THREE.Path();
  p.moveTo(cx - r, 0.001);
  p.lineTo(cx - r, height - r);
  p.absarc(cx, height - r, r, Math.PI, 0, true);
  p.lineTo(cx + r, 0.001);
  p.closePath();
  return p;
}

function rectHole(cx: number, cy: number, w: number, h: number): THREE.Path {
  const p = new THREE.Path();
  p.moveTo(cx - w / 2, cy - h / 2);
  p.lineTo(cx - w / 2, cy + h / 2);
  p.lineTo(cx + w / 2, cy + h / 2);
  p.lineTo(cx + w / 2, cy - h / 2);
  p.closePath();
  return p;
}

const rbox = (w: number, h: number, d: number) => new RoundedBoxGeometry(w, h, d, 2, BEVEL);

/** A bucket of vertex-coloured parts that merge into one mesh: one call per building, culled on its own. */
function parts(kit: Kit, bucket: THREE.Material) {
  const paint = (g: THREE.BufferGeometry, color: string, x = 0, y = 0, z = 0) => {
    const c = colored(g, new THREE.Color(color));
    kit.addStatic(bucket, c, new THREE.Matrix4().setPosition(x, y, z));
    c.dispose();
  };
  return {
    paint,
    /** A rounded block standing on `y0`. */
    block(color: string, x0: number, x1: number, z0: number, z1: number, y0: number, y1: number) {
      paint(rbox(x1 - x0, y1 - y0, z1 - z0), color, (x0 + x1) / 2, (y0 + y1) / 2, (z0 + z1) / 2);
    },
    /** A sharp box by its extents, for trim that sits on a face. */
    trim(color: string, x0: number, x1: number, z0: number, z1: number, y0: number, y1: number) {
      paint(new THREE.BoxGeometry(x1 - x0, y1 - y0, z1 - z0), color, (x0 + x1) / 2, (y0 + y1) / 2, (z0 + z1) / 2);
    },
    column(color: string, x: number, z: number, r: number, y0: number, y1: number) {
      paint(new THREE.CylinderGeometry(r, r, y1 - y0, 12), color, x, (y0 + y1) / 2, z);
    },
    plant(list: THREE.BufferGeometry[], x: number, z: number) {
      for (const g of list) {
        kit.addStatic(bucket, g, new THREE.Matrix4().setPosition(x, 0, z));
        g.dispose();
      }
    },
  };
}

/**
 * The clinic: a low, warm courtyard block. Its south wall has one tall arched door under a striped
 * awning and the open window the nurse calls from; behind the wall, a shallow porch where she stands.
 * A lattice screen, a low courtyard wall and a palm stand to the west of the door.
 */
export function buildClinic(kit: Kit) {
  const { x0, x1, z0, z1, h } = CLINIC;
  const { paint, block, trim, plant } = parts(kit, kit.mat.clinic);
  const cx = (x0 + x1) / 2;
  const door = PLACES.clinicDoor[0];
  const front = wall(x1 - x0, h, 0.45, [arch(door - cx, 1.7, 2.5), rectHole(PLACES.window[0] - cx, 1.55, 1.7, 1.1)]);
  paint(front, COLOR.clinic, cx, 0, z1 - 0.22);
  const porch = 2.2;
  block(COLOR.clinicShade, x0 + 0.3, x1 - 0.3, z0, z1 - porch - 0.2, 0, h - 0.3);
  block(COLOR.clinic, x0, x1, z1 - porch - 0.4, z1, h - 0.3, h);
  block(COLOR.clinic, x0, x0 + 0.45, z1 - porch - 0.2, z1 - 0.2, 0, h);
  block(COLOR.clinic, x1 - 0.45, x1, z1 - porch - 0.2, z1 - 0.2, 0, h);
  // The dark inside of the arch and the room behind the nurse's window.
  trim(COLOR.recess, door - 0.95, door + 0.95, z1 - 0.5, z1 - 0.44, 0, 2.6);
  trim(COLOR.clinicShade, x0, x1, z1 - 0.02, z1 + 0.03, h - 0.16, h);
  trim(COLOR.awning, x0, x1, z1 - 0.02, z1 + 0.03, 0, 0.22);

  // The awning: alternating stripes, sloping out over the arch.
  const stripes = 7;
  const span = 2.4;
  for (let i = 0; i < stripes; i++) {
    const x = door - span / 2 + (i + 0.5) * (span / stripes);
    const g = new THREE.BoxGeometry(span / stripes, 0.05, 1.05).rotateX(0.42);
    paint(g, i % 2 ? COLOR.surface : COLOR.awning, x, 2.78, z1 + 0.5);
  }

  // The lattice: a timber screen of crossing slats on a dark panel, west of the door.
  const L = { x: 0.1, y: 1.7, w: 1.9, h: 1.5 };
  trim(COLOR.recess, L.x - L.w / 2, L.x + L.w / 2, z1 - 0.01, z1 + 0.02, L.y - L.h / 2, L.y + L.h / 2);
  for (let i = 0; i <= 7; i++) {
    const x = L.x - L.w / 2 + (i * L.w) / 7;
    trim(COLOR.lattice, x - 0.022, x + 0.022, z1 + 0.02, z1 + 0.05, L.y - L.h / 2, L.y + L.h / 2);
  }
  for (let i = 0; i <= 5; i++) {
    const y = L.y - L.h / 2 + (i * L.h) / 5;
    trim(COLOR.lattice, L.x - L.w / 2, L.x + L.w / 2, z1 + 0.02, z1 + 0.05, y - 0.022, y + 0.022);
  }

  // The low courtyard wall, a palm inside it.
  const low = 0.62;
  block(COLOR.clinicShade, x0, 1.7, 3.85, 4.15, 0, low);
  block(COLOR.clinicShade, x0, x0 + 0.3, z1, 4.1, 0, low);
  block(COLOR.clinicShade, 1.45, 1.75, z1 + 1.1, 4.1, 0, low);
  plant(palm(2.5), -0.9, 2.75);
  kit.shadowAt(cx + 0.4, (z0 + z1) / 2 + 0.5, x1 - x0 + 3, z1 - z0 + 3);
  kit.shadowAt(-0.6, 3.1, 5.4, 1.9);
  front.dispose();
}

/**
 * The hospital, the hero of the skyline: a white podium with a glazed base, a tall tower with a glass
 * curtain wall behind vertical fins, a wing raised on columns, a helipad, a columned portico over the
 * door, a covered ambulance bay and a planted forecourt. No signs, no cross.
 */
export function buildHospital(kit: Kit) {
  const { paint, block, trim, column, plant } = parts(kit, kit.mat.hospital);
  const P = PODIUM;
  const T = TOWER;
  const W = WING;
  const D = PLACES.hospitalDoor;
  const front = P.z1;

  // The forecourt: pale paving from wall to road.
  trim(COLOR.forecourt, 61, 88, 1.5, 9.6, -0.1, 0.004);

  block(COLOR.hospital, P.x0, P.x1, P.z0, P.z1, 0, P.h);
  trim(COLOR.hospitalTeal, P.x0 - 0.03, P.x1 + 0.03, P.z0, P.z1 + 0.03, P.h - 0.36, P.h - 0.12);
  // Glazed ground floor either side of the door and the record, with slim fins.
  for (const [a, b] of [
    [63.2, 69.3],
    [75.3, 78],
  ]) {
    trim(COLOR.glass, a, b, front - 0.02, front + 0.025, 0.42, 2.62);
    for (let x = a; x <= b + 0.01; x += (b - a) / Math.round((b - a) / 0.74)) trim(COLOR.hospital, x - 0.04, x + 0.04, front, front + 0.11, 0.42, 2.62);
  }
  // A ribbon of upper windows along the podium.
  trim(COLOR.glassDeep, P.x0 + 0.7, P.x1 - 0.7, front - 0.02, front + 0.025, 3.25, 3.75);

  // The tower: a glass front behind fins, a white crown, a teal band at the roofline.
  block(COLOR.hospital, T.x0, T.x1, T.z0, T.z1, P.h - 0.1, T.h);
  const face = T.z1;
  trim(COLOR.glass, T.x0 + 0.5, T.x1 - 0.5, face - 0.03, face + 0.03, P.h + 0.3, T.h - 1);
  const fins = 15;
  for (let i = 0; i <= fins; i++) {
    const x = T.x0 + 0.5 + (i * (T.x1 - T.x0 - 1)) / fins;
    trim(COLOR.hospital, x - 0.055, x + 0.055, face, face + 0.26, P.h + 0.3, T.h - 1);
  }
  for (let y = P.h + 1.15; y < T.h - 1; y += 1.15) trim(COLOR.hospitalShade, T.x0 + 0.5, T.x1 - 0.5, face, face + 0.12, y - 0.03, y + 0.03);
  trim(COLOR.hospitalTeal, T.x0 - 0.04, T.x1 + 0.04, T.z0 - 0.04, T.z1 + 0.04, T.h - 0.62, T.h - 0.2);
  // Side glazing, so the tower also reads from the west on the way in.
  trim(COLOR.glassDeep, T.x0 - 0.03, T.x0 + 0.03, T.z0 + 0.8, T.z1 - 0.8, P.h + 0.5, T.h - 1);
  trim(COLOR.glassDeep, T.x1 - 0.03, T.x1 + 0.03, T.z0 + 0.8, T.z1 - 0.8, P.h + 0.5, T.h - 1);

  // The helipad: a plinth, a white disc, a teal ring and an H.
  const hx = (T.x0 + T.x1) / 2;
  const hz = (T.z0 + T.z1) / 2;
  const pad = T.h + 0.34;
  column(COLOR.hospitalShade, hx, hz, 1.1, T.h, pad - 0.1);
  paint(new THREE.CylinderGeometry(3.15, 3.15, 0.14, 40), COLOR.hospital, hx, pad, hz);
  paint(new THREE.RingGeometry(2.35, 2.72, 40).rotateX(-Math.PI / 2), COLOR.hospitalTeal, hx, pad + 0.075, hz);
  trim(COLOR.hospitalTeal, hx - 0.85, hx - 0.47, hz - 1.2, hz + 1.2, pad + 0.07, pad + 0.09);
  trim(COLOR.hospitalTeal, hx + 0.47, hx + 0.85, hz - 1.2, hz + 1.2, pad + 0.07, pad + 0.09);
  trim(COLOR.hospitalTeal, hx - 0.47, hx + 0.47, hz - 0.19, hz + 0.19, pad + 0.07, pad + 0.09);

  // The raised wing, floating on columns over the ground; teal glass wraps it.
  block(COLOR.hospital, W.x0, W.x1, W.z0, W.z1, W.y0, W.y1);
  trim(COLOR.hospitalTeal, W.x0 + 0.2, W.x1 + 0.03, W.z1 - 0.1, W.z1 + 0.03, W.y0 + 0.5, W.y1 - 0.5);
  trim(COLOR.hospitalTeal, W.x1 - 0.1, W.x1 + 0.03, W.z0 + 0.4, W.z1, W.y0 + 0.5, W.y1 - 0.5);
  for (const x of [80.2, 82.9]) for (const z of [W.z1 - 0.6, W.z0 + 1]) column(COLOR.hospitalShade, x, z, 0.17, 0, W.y0);

  // The record's dark frame on the wall, so a white card reads on a white wall.
  trim(COLOR.hospitalFrame, PLACES.slab[0] - 0.74, PLACES.slab[0] + 0.74, front - 0.02, front + 0.03, PLACES.slab[1] - 0.94, PLACES.slab[1] + 0.94);

  // The doorway: a dark glass recess beside the record.
  trim(COLOR.glassDeep, D[0] + 0.75, D[0] + 2.45, front - 0.02, front + 0.025, 0, 2.5);
  trim(COLOR.hospitalTeal, D[0] + 0.66, D[0] + 2.54, front, front + 0.05, 2.5, 2.62);

  // The portico: the deep canopy over the door, on columns, with a teal fascia.
  block(COLOR.hospital, D[0] - 2, D[0] + 3.4, front, front + 3.4, 2.74, 3.06);
  trim(COLOR.hospitalTeal, D[0] - 2.03, D[0] + 3.43, front + 3.32, front + 3.44, 2.82, 3.0);
  for (const x of [D[0] - 1.8, D[0] + 2.35, D[0] + 3.2]) column(COLOR.hospital, x, front + 3.05, 0.11, 0, 2.76);
  kit.shadowAt(D[0] - 0.3, front + 1.7, 4.6, 3.6);

  // The ambulance bay: a low roof on two columns and an ambulance waiting under it.
  block(COLOR.hospitalShade, 63.2, 68.6, front, front + 3.2, 2.2, 2.42);
  for (const x of [63.6, 68.2]) column(COLOR.hospitalShade, x, front + 2.9, 0.09, 0, 2.22);
  const A = { x: 65.9, z: front + 1.75 };
  block(COLOR.hospital, A.x - 1.1, A.x + 0.5, A.z - 0.48, A.z + 0.48, 0.2, 1.22);
  block(COLOR.hospital, A.x + 0.5, A.x + 1.12, A.z - 0.45, A.z + 0.45, 0.2, 0.82);
  trim(COLOR.hospitalTeal, A.x - 1.11, A.x + 1.13, A.z + 0.44, A.z + 0.5, 0.52, 0.66);
  trim(COLOR.glassDeep, A.x + 0.52, A.x + 1.0, A.z + 0.44, A.z + 0.48, 0.72, 1.0);
  for (const x of [A.x - 0.68, A.x + 0.72]) paint(new THREE.CylinderGeometry(0.17, 0.17, 1.02, 12).rotateX(Math.PI / 2), COLOR.recess, x, 0.17, A.z);
  kit.shadowAt(A.x, A.z, 3.6, 2.4);
  kit.shadowAt(66, front + 1.6, 6.4, 4);

  // Planting: low hedges along the forecourt edge, trees and palms inside it.
  for (const [a, b] of [
    [75.4, 86.4],
    [61.2, 66.4],
  ]) block(COLOR.leafDark, a, b, 8.55, 9.1, 0, 0.42);
  block(COLOR.leafDark, 78.6, 85.5, 1.45, 2.05, 0, 0.42);
  for (const [x, z] of [
    [84.2, 6.8],
    [80.2, 7.6],
    [62.2, 7.0],
  ]) {
    plant(palm(2.6), x, z);
    kit.shadowAt(x, z, 2);
  }
  plant(tree(1.05, 0.62), 87.2, 3.4);
  kit.shadowAt(87.2, 3.4, 2);

  kit.shadowAt((P.x0 + W.x1) / 2 + 0.5, (P.z0 + P.z1) / 2 - 1, W.x1 - P.x0 + 4, P.z1 - P.z0 + 5);
}

/** The road: a dark strip along the curve, with a pale kerb either side. One mesh, painted per vertex. */
export function buildRoad(kit: Kit, road: THREE.CatmullRomCurve3): THREE.Mesh {
  const n = 320;
  const p = new THREE.Vector3();
  const t = new THREE.Vector3();
  const road2 = new THREE.Color(COLOR.road);
  const kerb = new THREE.Color(COLOR.kerb);
  const strips: [number, number, THREE.Color][] = [
    [1.0, 0.007, kerb],
    [0.86, 0.013, road2],
  ];
  const pos = new Float32Array((n + 1) * 6 * strips.length);
  const col = new Float32Array((n + 1) * 6 * strips.length);
  const index: number[] = [];
  strips.forEach(([half, y, color], k) => {
    const start = k * (n + 1) * 2;
    for (let i = 0; i <= n; i++) {
      road.getPointAt(i / n, p);
      road.getTangentAt(i / n, t);
      const sx = -t.z * half;
      const sz = t.x * half;
      pos.set([p.x - sx, y, p.z - sz, p.x + sx, y, p.z + sz], (start + i * 2) * 3);
      color.toArray(col, (start + i * 2) * 3);
      color.toArray(col, (start + i * 2 + 1) * 3);
    }
    for (let i = 0; i < n; i++) {
      const a = start + i * 2;
      index.push(a, a + 1, a + 2, a + 1, a + 3, a + 2);
    }
  });
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.BufferAttribute(pos, 3));
  g.setAttribute("color", new THREE.BufferAttribute(col, 3));
  g.setIndex(index);
  const mesh = new THREE.Mesh(g, kit.mat.road);
  mesh.matrixAutoUpdate = false;
  return mesh;
}

export const vec = (a: readonly number[]) => new THREE.Vector3(a[0], a[1], a[2]);
