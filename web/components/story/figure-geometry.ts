import * as THREE from "three";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import type { Top } from "@/components/avatar/traits";

/**
 * Figure shapes, in figure-local units with y up and the base on y = 0. The flood that colours a
 * figure reads local y, so every part is built in place rather than offset by its mesh.
 */

/** A head: lathed around y, front facing +z. `radiusAt` also shapes the printed face patch. */
export interface HeadSpec {
  r: number;
  /** The radius of the body under it, which covering cloth has to clear. */
  shoulder: number;
  y0: number;
  y1: number;
  radiusAt(y: number): number;
}

export interface Silhouette {
  /** Geometry for the body at a radial segment count. */
  body(seg: number): THREE.BufferGeometry;
  head: HeadSpec;
}

export interface FigureGeo {
  body: THREE.BufferGeometry;
  head: THREE.BufferGeometry;
  face: THREE.BufferGeometry;
  hair: Record<Top, THREE.BufferGeometry | null>;
  bodyLow: THREE.BufferGeometry;
  headLow: THREE.BufferGeometry;
  height: number;
}

type Pt = readonly [r: number, y: number];

export function lathe(points: readonly Pt[], seg: number, phiStart = 0, phiLength = Math.PI * 2): THREE.BufferGeometry {
  return new THREE.LatheGeometry(
    points.map(([r, y]) => new THREE.Vector2(r, y)),
    seg,
    phiStart,
    phiLength,
  );
}

/** A Lego head: a cylinder with rounded shoulders. */
export function legoHead(r: number, y0: number, y1: number, bevel: number, shoulder: number): HeadSpec {
  return {
    r,
    shoulder,
    y0,
    y1,
    radiusAt(y) {
      const edge = Math.min(y - y0, y1 - y);
      if (edge >= bevel) return r;
      const d = bevel - Math.max(0, edge);
      return r - bevel + Math.sqrt(Math.max(0, bevel * bevel - d * d));
    },
  };
}

function headPoints(h: HeadSpec, rows: number): Pt[] {
  const pts: Pt[] = [[0, h.y0]];
  for (let i = 0; i <= rows; i++) {
    const y = h.y0 + ((h.y1 - h.y0) * i) / rows;
    pts.push([h.radiusAt(y), y]);
  }
  pts.push([0, h.y1]);
  return pts;
}

/** The front half of the head, a hair proud of it, with even v so the print does not stretch. */
function facePatch(h: HeadSpec, seg: number): THREE.BufferGeometry {
  const y0 = h.y0 + (h.y1 - h.y0) * 0.075;
  const y1 = h.y1 - (h.y1 - h.y0) * 0.075;
  const pts: Pt[] = [];
  for (let i = 0; i <= 6; i++) {
    const y = y0 + ((y1 - y0) * i) / 6;
    pts.push([h.radiusAt(y) + 0.005, y]);
  }
  return lathe(pts, Math.max(8, seg >> 1), -Math.PI / 2, Math.PI);
}

function painted(g: THREE.BufferGeometry, c: THREE.Color): THREE.BufferGeometry {
  const n = g.attributes.position.count;
  const a = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) c.toArray(a, i * 3);
  g.setAttribute("color", new THREE.BufferAttribute(a, 3));
  return g;
}

const WHITE = new THREE.Color(1, 1, 1);
const AGAL = new THREE.Color(0.05, 0.06, 0.07);

function merged(parts: THREE.BufferGeometry[], color = WHITE): THREE.BufferGeometry {
  const g = mergeGeometries(parts.map((p) => (p.attributes.color ? p : painted(p, color))))!;
  for (const p of parts) p.dispose();
  return g;
}

/** A skull cap: full circle above `low`, closed underneath so no gap shows against the head. */
function dome(h: HeadSpec, low: number, th: number, seg: number): THREE.BufferGeometry {
  const R = h.r + th;
  const shoulder = h.y1 - 0.05;
  const crown = h.y1 + th * 0.9 + 0.012;
  const pts: Pt[] = [
    [h.r - 0.02, low],
    [R, low + 0.008],
    [R, Math.max(low + 0.012, shoulder)],
  ];
  for (let i = 1; i <= 5; i++) {
    const a = (i / 5) * (Math.PI / 2);
    pts.push([i === 5 ? 0 : R * Math.cos(a), shoulder + (crown - shoulder) * Math.sin(a)]);
  }
  return lathe(pts, seg);
}

/** A shell around the head and below it, open by `open` radians either side of the face. */
function shell(h: HeadSpec, low: number, top: number, thLow: number, thTop: number, open: number, seg: number): THREE.BufferGeometry {
  return lathe(
    [
      [h.r - 0.04, low],
      [h.r + thLow, low + 0.01],
      [h.r + (thLow + thTop) / 2, (low + top) / 2],
      [h.r + thTop, top],
    ],
    seg,
    open,
    Math.PI * 2 - open * 2,
  );
}

/**
 * A head covering with an oval opening for the face, like the avatar's cut-out. A lathe whose sweep
 * narrows row by row: each row of the grid starts and ends at the edge of the oval.
 */
function hoodGrid(h: HeadSpec, profile: readonly Pt[], open: number, seg: number): THREE.BufferGeometry {
  const yc = (h.y0 + h.y1) / 2 - 0.012;
  const yr = (h.y1 - h.y0) * 0.5;
  const rows = 18;
  const curve = new THREE.CatmullRomCurve3(profile.map(([r, y]) => new THREE.Vector3(r, y, 0)), false, "centripetal");
  const columns = seg + 8;
  const pos: number[] = [];
  const nor: number[] = [];
  const p = new THREE.Vector3();
  const t = new THREE.Vector3();
  for (let j = 0; j <= rows; j++) {
    curve.getPointAt(j / rows, p);
    curve.getTangentAt(j / rows, t);
    const d = (p.y - yc) / yr;
    const edge = d * d < 1 ? open * Math.sqrt(1 - d * d) : 0;
    for (let i = 0; i <= columns; i++) {
      const phi = edge + ((Math.PI * 2 - edge * 2) * i) / columns;
      const [sin, cos] = [Math.sin(phi), Math.cos(phi)];
      pos.push(Math.max(0, p.x) * sin, p.y, Math.max(0, p.x) * cos);
      nor.push(t.y * sin, -t.x, t.y * cos);
    }
  }
  const index: number[] = [];
  for (let j = 0; j < rows; j++) {
    for (let i = 0; i < columns; i++) {
      const a = j * (columns + 1) + i;
      const c = a + columns + 1;
      index.push(a, a + 1, c, a + 1, c + 1, c);
    }
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute("normal", new THREE.Float32BufferAttribute(nor, 3));
  g.setAttribute("uv", new THREE.Float32BufferAttribute(new Float32Array((pos.length / 3) * 2), 2));
  g.setIndex(index);
  return g;
}

const SIDE = 1.02;
type Hair = (h: HeadSpec, seg: number) => THREE.BufferGeometry;

function ball(x: number, y: number, z: number, r: number, seg: number): THREE.BufferGeometry {
  return new THREE.SphereGeometry(r, Math.max(6, seg >> 2), Math.max(4, seg >> 3)).translate(x, y, z);
}

const short: Hair = (h, seg) => merged([dome(h, h.y1 - 0.13, 0.022, seg), shell(h, h.y0 + 0.13, h.y1 - 0.1, 0.022, 0.022, SIDE + 0.25, seg)]);
const bob = (low: number, flare: number): Hair => (h, seg) => merged([dome(h, h.y1 - 0.13, 0.03, seg), shell(h, h.y0 + low, h.y1 - 0.1, flare, 0.03, SIDE - 0.1, seg)]);
/** Cloth from the shoulders up around the head and over the crown, `low` below the chin at `rLow`. */
const hood = (low: number, flare: number): Hair => (h, seg) => {
  const R = h.r + 0.036;
  const rLow = Math.max(R, h.shoulder) + flare;
  return merged([
    hoodGrid(
      h,
      [
        [rLow, h.y0 + low],
        [(rLow + R) / 2 + 0.02, h.y0 + low * 0.45],
        [R + 0.006, h.y0 + 0.03],
        [R, (h.y0 + h.y1) / 2],
        [R, h.y1 - 0.04],
        [R * 0.74, h.y1 + 0.04],
        [0.001, h.y1 + 0.068],
      ],
      1.28,
      seg,
    ),
  ]);
};

/**
 * One piece per `Top` the traits system can assign, or none for a bare head. The silhouette family
 * is what tells people apart at road size, so families differ in outline before they differ in detail.
 */
const HAIR: Record<Top, Hair | null> = {
  bald: null,
  short,
  crop: (h, seg) => merged([dome(h, h.y1 - 0.085, 0.012, seg), shell(h, h.y1 - 0.2, h.y1 - 0.06, 0.012, 0.012, SIDE + 0.45, seg)]),
  curly: (h, seg) => {
    const parts = [dome(h, h.y1 - 0.1, 0.018, seg)];
    const ring = (n: number, y: number, R: number, r: number, from = 0, sweep = Math.PI * 2) => {
      for (let i = 0; i < n; i++) {
        const a = from + (sweep * i) / n;
        parts.push(ball(Math.sin(a) * R, y, Math.cos(a) * R, r, seg));
      }
    };
    ring(11, h.y1 - 0.03, h.r * 0.92, 0.075);
    ring(6, h.y1 + 0.05, h.r * 0.48, 0.078);
    ring(6, h.y1 - 0.15, h.r + 0.01, 0.07, SIDE + 0.2, Math.PI * 2 - (SIDE + 0.2) * 2);
    return merged(parts);
  },
  wavy: bob(0, 0.05),
  long: bob(-0.3, 0.075),
  bun: (h, seg) => merged([short(h, seg), ball(0, h.y1 + 0.075, -0.05, 0.1, seg)]),
  hijab: hood(-0.11, 0.022),
  shayla: hood(-0.3, 0.04),
  ghutra: (h, seg) =>
    merged([
      hood(-0.2, 0.05)(h, seg),
      painted(new THREE.TorusGeometry(h.r + 0.042, 0.024, 6, seg).rotateX(Math.PI / 2).translate(0, h.y1 - 0.03, 0), AGAL),
    ]),
  kufi: (h, seg) =>
    painted(
      lathe(
        [
          [h.r - 0.02, h.y1 - 0.11],
          [h.r + 0.016, h.y1 - 0.105],
          [h.r + 0.012, h.y1 + 0.04],
          [h.r - 0.03, h.y1 + 0.068],
          [0, h.y1 + 0.072],
        ],
        seg,
      ),
      WHITE,
    ),
};

/**
 * A figure: a straight cylinder for the body, edges barely rounded, and a Lego head of its own width
 * on top. `r` and `height` are the body's; everything that sits on it follows from those.
 */
export function cylinderFigure(r: number, height: number, headR: number, headH: number): Silhouette {
  const e = 0.024;
  const lip = [0.3, 0.71, 0.93];
  const edge = (y: number, dir: number): Pt[] => lip.map((t) => [r - e * (1 - Math.sin((t * Math.PI) / 2)), y + dir * e * (1 - Math.cos((t * Math.PI) / 2))]);
  return {
    head: legoHead(headR, height - 0.008, height - 0.008 + headH, 0.05, r),
    body: (seg) => lathe([[0, 0], [r - e, 0], ...edge(0, 1), ...edge(height, -1).reverse(), [r - e, height], [0, height]], seg),
  };
}

export const CYLINDER = cylinderFigure(0.235, 0.62, 0.205, 0.39);

export function figureGeo(s: Silhouette, seg: number): FigureGeo {
  const hair = Object.fromEntries((Object.keys(HAIR) as Top[]).map((t) => [t, HAIR[t]?.(s.head, seg) ?? null])) as FigureGeo["hair"];
  return {
    body: s.body(seg),
    head: lathe(headPoints(s.head, 10), seg),
    face: facePatch(s.head, seg),
    hair,
    bodyLow: s.body(Math.max(8, seg >> 2)),
    headLow: lathe(headPoints(s.head, 4), Math.max(8, seg >> 2)),
    height: s.head.y1,
  };
}
