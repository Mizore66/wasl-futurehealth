import * as THREE from "three";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import { RoundedBoxGeometry } from "three/addons/geometries/RoundedBoxGeometry.js";
import type { Kit } from "./kit";
import { COLOR } from "./palette";

/**
 * The miniature city: one road from Fatima's clinic in the west to the hospital in the east, with
 * an open stretch of sand in the middle. World units, y up; one pebble person is one unit tall.
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
const HOSPITAL: (Rect & { h: number })[] = [
  { x0: 63, x1: 81, z0: -7, z1: 1.5, h: 4.2 },
  { x0: 65.5, x1: 78.5, z0: -9.5, z1: -1.5, h: 6.6 },
  { x0: 68.5, x1: 75.5, z0: -12, z1: -4.5, h: 9 },
];
const GAP = { x0: 38.5, x1: 58.5 };

/** Stone tones of the model: three, close together, warm. */
const TONES = [COLOR.block1, COLOR.block2, COLOR.block3].map((c) => new THREE.Color(c));
const HEIGHTS = [0.7, 1.2, 1.8, 2.6, 3.6];

function colored(g: THREE.BufferGeometry, c: THREE.Color): THREE.BufferGeometry {
  const n = g.attributes.position.count;
  const a = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) c.toArray(a, i * 3);
  g.setAttribute("color", new THREE.BufferAttribute(a, 3));
  g.deleteAttribute("uv");
  return g;
}

export interface Chunk {
  near: THREE.Mesh;
  far: THREE.Mesh;
  shadow: THREE.Mesh;
  x: number;
}

/**
 * The city blocks, cut into chunks along x. Each chunk has a rounded (near) and a plain (far) version of
 * its blocks, one merged mesh each, and its contact shadows.
 */
export function buildCity(kit: Kit, road: THREE.CatmullRomCurve3, mobile: boolean): Chunk[] {
  const r = rand(11);
  const roadPts = road.getSpacedPoints(300);
  const nearRoad = (x: number, z: number, pad: number) => roadPts.some((p) => Math.abs(p.x - x) < pad && Math.abs(p.z - z) < pad);
  const inRect = (x: number, z: number, rc: Rect, pad: number) => x > rc.x0 - pad && x < rc.x1 + pad && z > rc.z0 - pad && z < rc.z1 + pad;
  const CHUNK = 12;
  const bins = new Map<number, { near: THREE.BufferGeometry[]; far: THREE.BufferGeometry[]; shadow: THREE.BufferGeometry[] }>();
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
      if (z > -2 && ((z < 18 && x > -4 && x < 14) || (z < 20 && x > 61 && x < 85))) continue;
      // The blocks get lower toward the road, so the walk stays in view.
      const tall = Math.abs(z - 7) < 6 ? 2 : 5;
      const h = HEIGHTS[Math.floor(r() * tall)];
      const tone = TONES[Math.floor(r() * 3)];
      const yaw = (r() - 0.5) * 0.12;
      const m = new THREE.Matrix4().makeRotationY(yaw).setPosition(x, h / 2, z);
      const key = Math.floor(x / CHUNK);
      const bin = bins.get(key) ?? bins.set(key, { near: [], far: [], shadow: [] }).get(key)!;
      bin.near.push(colored(new RoundedBoxGeometry(w, h, d, 2, BEVEL).toNonIndexed(), tone).applyMatrix4(m));
      bin.far.push(colored(new THREE.BoxGeometry(w, h, d).toNonIndexed(), tone).applyMatrix4(m));
      const s = shadowGeo.clone().applyMatrix4(new THREE.Matrix4().makeScale(w + 1.6, 1, d + 1.6).setPosition(x + 0.25, 0.006, z + 0.35));
      bin.shadow.push(s);
    }
  }

  const chunks: Chunk[] = [];
  for (const [key, bin] of bins) {
    const mk = (list: THREE.BufferGeometry[], mat: THREE.Material) => {
      const mesh = new THREE.Mesh(mergeGeometries(list)!, mat);
      mesh.matrixAutoUpdate = false;
      for (const g of list) g.dispose();
      return mesh;
    };
    const shadow = mk(bin.shadow, kit.mat.shadow);
    shadow.renderOrder = 1;
    chunks.push({ near: mk(bin.near, kit.mat.city), far: mk(bin.far, kit.mat.city), shadow, x: key * CHUNK + CHUNK / 2 });
  }
  return chunks;
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

const rbox = (w: number, h: number, d: number) => new RoundedBoxGeometry(w, h, d, 3, BEVEL);

/**
 * The clinic: a low, wide courtyard block. Its south wall has one tall arched door and one window,
 * and behind the wall a shallow porch where the nurse stands.
 */
export function buildClinic(kit: Kit) {
  const { x0, x1, z0, z1, h } = CLINIC;
  const cx = (x0 + x1) / 2;
  const doorX = PLACES.clinicDoor[0] - cx;
  const winX = PLACES.window[0] - cx;
  const front = wall(x1 - x0, h, 0.45, [arch(doorX, 1.7, 2.5), rectHole(winX, 1.55, 1.7, 1.1)]);
  kit.addStatic(kit.mat.building, front, new THREE.Matrix4().setPosition(cx, 0, z1 - 0.22));
  const porch = 2.2;
  kit.addStatic(kit.mat.buildingShade, rbox(x1 - x0 - 0.6, h - 0.3, z1 - z0 - porch), new THREE.Matrix4().setPosition(cx, (h - 0.3) / 2, (z0 + z1 - porch) / 2 - 0.2));
  kit.addStatic(kit.mat.building, rbox(x1 - x0, 0.3, porch + 0.4), new THREE.Matrix4().setPosition(cx, h - 0.15, z1 - porch / 2 - 0.2));
  kit.addStatic(kit.mat.building, rbox(0.45, h, porch), new THREE.Matrix4().setPosition(x0 + 0.22, h / 2, z1 - porch / 2 - 0.2));
  kit.addStatic(kit.mat.building, rbox(0.45, h, porch), new THREE.Matrix4().setPosition(x1 - 0.22, h / 2, z1 - porch / 2 - 0.2));
  kit.shadowAt(cx + 0.4, (z0 + z1) / 2 + 0.5, x1 - x0 + 3, z1 - z0 + 3);
  front.dispose();
}

/** The hospital: three stepped masses and a deep canopy over the door. No signs, no cross. */
export function buildHospital(kit: Kit) {
  HOSPITAL.forEach((b, i) => {
    const g = rbox(b.x1 - b.x0, b.h, b.z1 - b.z0);
    kit.addStatic(i === 1 ? kit.mat.buildingShade : kit.mat.building, g, new THREE.Matrix4().setPosition((b.x0 + b.x1) / 2, b.h / 2, (b.z0 + b.z1) / 2));
    g.dispose();
  });
  const [b] = HOSPITAL;
  kit.shadowAt((b.x0 + b.x1) / 2 + 0.5, (b.z0 + b.z1) / 2 - 1, b.x1 - b.x0 + 4, b.z1 - b.z0 + 8);
  const D = PLACES.hospitalDoor;
  // The doorway: a dark recess in the wall.
  kit.addStatic(kit.mat.recess, new THREE.BoxGeometry(1.7, 2.5, 0.06), new THREE.Matrix4().setPosition(D[0] + 1.6, 1.25, b.z1 + 0.01));
  // The canopy: one deep slab cantilevered from the wall over the door.
  kit.addStatic(kit.mat.building, rbox(5.4, 0.32, 3.4), new THREE.Matrix4().setPosition(D[0] + 0.7, 2.9, b.z1 + 1.65));
  kit.shadowAt(D[0] + 0.7, b.z1 + 1.9, 6.4, 4.4);
}

/** The paving: a flat strip along the road, a shade lighter than the ground. */
export function buildPaving(kit: Kit, road: THREE.CatmullRomCurve3): THREE.Mesh {
  const n = 320;
  const pos = new Float32Array((n + 1) * 6);
  const p = new THREE.Vector3();
  const t = new THREE.Vector3();
  const width = 1.7;
  for (let i = 0; i <= n; i++) {
    road.getPointAt(i / n, p);
    road.getTangentAt(i / n, t);
    const sx = -t.z * (width / 2);
    const sz = t.x * (width / 2);
    pos.set([p.x - sx, 0.012, p.z - sz, p.x + sx, 0.012, p.z + sz], i * 6);
  }
  const index: number[] = [];
  for (let i = 0; i < n; i++) index.push(i * 2, i * 2 + 1, i * 2 + 2, i * 2 + 1, i * 2 + 3, i * 2 + 2);
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.BufferAttribute(pos, 3));
  g.setIndex(index);
  const mesh = new THREE.Mesh(g, kit.mat.paving);
  mesh.matrixAutoUpdate = false;
  return mesh;
}

export const vec = (a: readonly number[]) => new THREE.Vector3(a[0], a[1], a[2]);
