import * as THREE from "three";
import type { Kit } from "./kit";
import { COLOR } from "./palette";
import { CAST, JOIN, RIDE, RUN, clamp01, ease, span, walkAt, type CastKey, type FaceKey, type StoryFigures } from "./script";
import { PLACES, rand, uNear, vec } from "./world";

/** What moves in the world. Every pose is a function of `p`; only the chip replay reads the clock. */
export interface Actor {
  group: THREE.Object3D;
  /** Returns true while it still needs frames with no scroll change (the chip replay). */
  update(p: number, now: number, camera: THREE.Camera): boolean;
}

export interface Cast {
  kit: Kit;
  mobile: boolean;
  faces: Record<FaceKey, THREE.Texture>;
  unknown: THREE.Texture;
  figures: StoryFigures;
  road: THREE.CatmullRomCurve3;
  thread: THREE.CatmullRomCurve3;
  /** The chip the visitor picked, and when; a change replays the join over 1.2 s. */
  pick: { person: CastKey; at: number };
}

const STRIDE = 0.55;

/** A contact shadow that belongs to an actor, so it hides with it. */
function shade(kit: Kit, x: number, z: number, w: number, d = w): THREE.Mesh {
  const m = new THREE.Mesh(kit.geo.shadow, kit.mat.shadow);
  m.position.set(x, 0.008, z);
  m.scale.set(w, 1, d);
  m.renderOrder = 1;
  return m;
}
const HOP = 0.18;
/** The one overshoot in the story: a pebble turning over. */
const easeBack = (t: number) => 1 + 2.2 * (t - 1) ** 3 + 1.2 * (t - 1) ** 2;

/** Where the thread tip is (0..1 along the thread) for a scroll position. */
export function tipU(p: number, ring1: number): number {
  return ease(span(p, RUN.from, RUN.to)) * (ring1 - 0.004);
}

/** Where the receipt is along the thread (1 at the hospital, 0 at the nurse's window). */
export function receiptU(p: number): number {
  return 1 - ease(span(p, RIDE.from, RIDE.to));
}

/** The thread: out of the nurse's window, down to the road, and along it to the hospital door. */
export function makeThread(road: THREE.CatmullRomCurve3): THREE.CatmullRomCurve3 {
  const W = PLACES.window;
  const pts = [new THREE.Vector3(W[0], W[1] - 0.1, W[2] - 0.5), new THREE.Vector3(W[0] - 0.6, 1.0, W[2] + 1.3)];
  const n = 60;
  const p = new THREE.Vector3();
  for (let i = 1; i <= n; i++) {
    road.getPointAt(0.03 + (0.97 * i) / n, p);
    pts.push(new THREE.Vector3(p.x, 0.5, p.z + (i === n ? 0.32 : 0)));
  }
  const c = new THREE.CatmullRomCurve3(pts, false, "centripetal");
  c.arcLengthDivisions = 800;
  return c;
}

/** A flat strip along a curve with a slow twist. Growth is a draw range, so nothing is rebuilt per frame. */
function ribbon(curve: THREE.Curve<THREE.Vector3>, segments: number, width: number, twist: number) {
  const frames = curve.computeFrenetFrames(segments, false);
  const pos = new Float32Array((segments + 1) * 6);
  const nor = new Float32Array((segments + 1) * 6);
  const p = new THREE.Vector3();
  const side = new THREE.Vector3();
  const up = new THREE.Vector3();
  for (let i = 0; i <= segments; i++) {
    const t = i / segments;
    curve.getPointAt(t, p);
    const a = t * twist;
    side.copy(frames.binormals[i]).multiplyScalar(Math.cos(a)).addScaledVector(frames.normals[i], Math.sin(a));
    up.crossVectors(frames.tangents[i], side).normalize();
    pos.set([p.x - (side.x * width) / 2, p.y - (side.y * width) / 2, p.z - (side.z * width) / 2], i * 6);
    pos.set([p.x + (side.x * width) / 2, p.y + (side.y * width) / 2, p.z + (side.z * width) / 2], i * 6 + 3);
    up.toArray(nor, i * 6);
    up.toArray(nor, i * 6 + 3);
  }
  const index: number[] = [];
  for (let i = 0; i < segments; i++) {
    const a = i * 2;
    index.push(a, a + 1, a + 2, a + 1, a + 3, a + 2);
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.BufferAttribute(pos, 3));
  g.setAttribute("normal", new THREE.BufferAttribute(nor, 3));
  g.setIndex(index);
  return {
    geometry: g,
    /** Show the strip from `a` to `b` (0..1). */
    show(a: number, b: number) {
      const i0 = Math.round(clamp01(a) * segments);
      const i1 = Math.round(clamp01(b) * segments);
      g.setDrawRange(i0 * 6, Math.max(0, i1 - i0) * 6);
    },
  };
}

/**
 * A pebble person walking: hop height from the stride phase, a squash at each landing, a sway that
 * alternates feet, and a contact shadow that shrinks at the top of a hop. Pure arithmetic per frame.
 */
function gait(distance: number) {
  const phase = distance / STRIDE;
  const f = phase - Math.floor(phase);
  const hop = HOP * Math.abs(Math.sin(Math.PI * f));
  const land = f < 0.15 ? Math.sin((f / 0.15) * Math.PI) : 0;
  const foot = Math.floor(phase) % 2 === 0 ? 1 : -1;
  return { hop, squash: land, sway: foot * Math.sin(Math.PI * f), f };
}

const camDir = new THREE.Vector3();
const tmp = new THREE.Vector3();
const tan = new THREE.Vector3();

/** Yaw that turns a pebble's face (+z) toward the camera, leaning a little toward where it travels. */
function faceYaw(at: THREE.Vector3, camera: THREE.Camera, travel?: THREE.Vector3): number {
  camDir.copy(camera.position).sub(at).setY(0).normalize();
  if (travel) camDir.addScaledVector(travel, 0.35).normalize();
  return Math.atan2(camDir.x, camDir.z);
}

/** Fatima: walks out of her clinic, across the city, and into the hospital, where she becomes "?". */
export function fatima(c: Cast): Actor & { at: THREE.Vector3 } {
  const { kit } = c;
  const group = new THREE.Group();
  const body = new THREE.Group();
  const peb = kit.pebble(c.faces.fatima, c.unknown);
  peb.group.position.y = 0.5;
  body.add(peb.group);
  const footGeo = new THREE.SphereGeometry(0.11, 12, 8).scale(1, 0.55, 1.4);
  const feet = [-1, 1].map((s) => {
    const m = new THREE.Mesh(footGeo, kit.mat.stone);
    m.position.set(s * 0.17, 0.05, 0);
    body.add(m);
    return m;
  });
  const shadow = new THREE.Mesh(kit.geo.shadow, kit.mat.shadow);
  shadow.renderOrder = 1;
  group.add(body, shadow);

  const length = c.road.getLength();
  const at = new THREE.Vector3();
  const end = c.road.getPointAt(1);
  // The thread reaches her at the end of the join, so she stands where the thread ends.
  end.z += 0.0;

  return {
    group,
    at,
    update(p, now, camera) {
      const s = walkAt(p);
      c.road.getPointAt(s, at);
      c.road.getTangentAt(s, tan);
      const g = gait(s * length);
      const walking = s > 0 && s < 1;
      const hop = walking ? g.hop : 0;
      body.position.set(at.x, hop, at.z);
      const sq = walking ? g.squash : 0;
      body.scale.set(1 + sq * 0.05, 1 - sq * 0.1, 1 + sq * 0.05);
      shadow.position.set(at.x, 0.008, at.z);
      shadow.scale.setScalar(1.15 * (1 - (hop / HOP) * 0.2));
      for (const [i, m] of feet.entries()) {
        const swing = walking ? g.sway * (i ? 1 : -1) * 0.16 : 0;
        m.position.z = swing;
        m.visible = !c.mobile && p < 0.42;
      }

      // At the hospital door she turns over: the "?" is what the hospital's record shows.
      const who = CAST[c.pick.person].outcome;
      const replay = clamp01((now - c.pick.at) / 1200);
      const j = span(p, JOIN.from, JOIN.to) * ease(replay);
      const back = who === "review" ? 0 : easeBack(span(j, 0.7, 0.86));
      const over = easeBack(span(p, 0.343, 0.362)) * (1 - back);
      const wantFront = who === "apart" && back > 0 ? c.faces.stranger : c.faces[c.pick.person];
      if (peb.frontMat.map !== wantFront) peb.frontMat.map = wantFront;

      const yaw = faceYaw(at, camera, walking ? tan : undefined);
      body.rotation.set(walking ? 0.1 : 0, yaw + Math.PI * over, walking ? g.sway * 0.05 : 0, "YXZ");
      return replay < 1;
    },
  };
}

/** The teal thread, the two rings at the hospital door, and what the join does for each chip. */
export function thread(c: Cast, fatimaAt: THREE.Vector3): Actor & { tip: THREE.Vector3; ring1: number } {
  const { kit } = c;
  const group = new THREE.Group();
  const strip = ribbon(c.thread, c.mobile ? 600 : 1200, 0.13, Math.PI * 9);
  const mesh = new THREE.Mesh(strip.geometry, kit.mat.teal);
  mesh.frustumCulled = false;
  group.add(mesh);

  const end = c.thread.getPointAt(1);
  const u1 = uNear(c.thread, end.x - 1.3, end.z + 2.35);
  const u2 = uNear(c.thread, end.x - 0.45, end.z + 1.25);
  const ringGeo = new THREE.TorusGeometry(0.62, 0.06, 12, c.mobile ? 40 : 64);
  const ringMats = [0, 1].map(() => kit.tint(COLOR.stone));
  const gates = [u1, u2].map((u, i) => {
    const m = new THREE.Mesh(ringGeo, ringMats[i]);
    c.thread.getPointAt(u, m.position);
    c.thread.getTangentAt(u, tan);
    m.rotation.y = Math.atan2(tan.x, tan.z);
    m.position.y = 0.62;
    group.add(m);
    return m;
  });

  const fray = new THREE.Group();
  const strand = ribbon(new THREE.LineCurve3(new THREE.Vector3(), new THREE.Vector3(0.7, 0, 0)), 6, 0.05, 0.8);
  strand.show(0, 1);
  for (const [ry, rz] of [
    [0.4, 0.3],
    [-0.35, -0.05],
    [0.1, -0.4],
  ]) {
    const m = new THREE.Mesh(strand.geometry, kit.mat.held);
    m.rotation.set(0, ry, rz);
    fray.add(m);
  }
  c.thread.getPointAt(u2 - 0.004, fray.position);
  group.add(fray);

  const reviewers = [0, 1].map(() => {
    const m = new THREE.Mesh(kit.geo.pebble, kit.mat.unknown);
    m.scale.setScalar(0.8);
    group.add(m);
    return m;
  });

  const bead = new THREE.Mesh(new THREE.SphereGeometry(0.11, 16, 12), kit.mat.teal);
  const head = new THREE.Mesh(new THREE.SphereGeometry(0.17, 20, 14), kit.mat.teal);
  group.add(bead, head);
  const tip = new THREE.Vector3();
  const lit = new THREE.Color(COLOR.tealBright);
  const held = new THREE.Color(COLOR.held);
  const ink = new THREE.Color(COLOR.unknown);
  const stone = new THREE.Color(COLOR.stone);

  return {
    group,
    tip,
    ring1: u1,
    update(p, now) {
      const who = CAST[c.pick.person].outcome;
      const replay = clamp01((now - c.pick.at) / 1200);
      const j = span(p, JOIN.from, JOIN.to) * ease(replay);
      const run = tipU(p, u1);
      const quiver = j > 0.05 && j < 0.32 ? Math.sin(j * 120) * 0.0015 : 0;
      const reach2 = ease(span(j, 0.3, 0.5));
      const reachEnd = who === "joined" ? ease(span(j, 0.52, 0.7)) : 0;
      let u = run;
      if (j > 0) u = u1 - 0.004 + quiver + (u2 - u1) * reach2 * (who === "review" ? 0.92 : who === "apart" ? 0.6 : 1) + (1 - u2) * reachEnd;
      // Once the receipt leaves, the whole thread stays: it is the road home.
      const visible = p > RUN.from;
      mesh.visible = visible;
      strip.show(0, u);
      c.thread.getPointAt(Math.max(u, 0.001), tip);
      head.position.copy(tip);
      head.visible = visible && j < 0.7 && p < 0.75;
      for (const g of gates) g.visible = p > RUN.from && p < 0.748;
      ringMats[0].color.copy(j > 0.28 ? lit : stone);
      ringMats[1].color.copy(j > 0.48 ? (who === "joined" ? lit : who === "review" ? held : ink) : stone);
      const frayed = who === "review" ? ease(span(j, 0.5, 0.64)) : 0;
      fray.visible = frayed > 0;
      fray.scale.setScalar(Math.max(frayed, 0.001));
      const come = who === "review" ? ease(span(j, 0.56, 0.8)) : 0;
      reviewers.forEach((r, i) => {
        r.visible = come > 0;
        r.position.set(fatimaAt.x - 1.3 + i * 2.6, 0.4, fatimaAt.z + THREE.MathUtils.lerp(-1.2, 0.4, come));
        r.rotation.y = i ? -0.5 : 0.5;
      });
      const pulse = span(j, 0.86, 0.98);
      bead.visible = who === "joined" && pulse > 0 && pulse < 1;
      if (bead.visible) c.thread.getPointAt(Math.max(0.6, 1 - pulse), bead.position);
      return replay < 1;
    },
  };
}

/** The record slab on the hospital wall, which later becomes the report the two readers check. */
export function slabAndReaders(c: Cast): Actor {
  const { kit } = c;
  const group = new THREE.Group();
  const S = vec(PLACES.slab);
  const slab = new THREE.Mesh(new THREE.BoxGeometry(1.3, 1.7, 0.08), kit.mat.surface);
  slab.position.copy(S);
  group.add(slab);
  const lineGeo = new THREE.BoxGeometry(0.9, 0.07, 0.02);
  const lineMat = kit.tint(COLOR.stone);
  for (let i = 0; i < 5; i++) {
    const l = new THREE.Mesh(lineGeo, i === 2 ? lineMat : kit.mat.stone);
    l.position.set(S.x - (i === 0 ? 0.2 : 0), S.y + 0.55 - i * 0.26, S.z + 0.05);
    if (i === 0) l.scale.x = 0.5;
    group.add(l);
  }
  const lineY = S.y + 0.55 - 2 * 0.26;

  const rulesMat = Object.assign(kit.tint(COLOR.stone), { flatShading: true });
  const aiMat = kit.tint(COLOR.stone);
  const rules = new THREE.Mesh(new THREE.TorusGeometry(0.48, 0.09, 4, 6), rulesMat);
  const ai = new THREE.Mesh(new THREE.TorusGeometry(0.48, 0.08, 16, c.mobile ? 40 : 72), aiMat);
  rules.position.set(S.x - 2.1, lineY, S.z + 0.9);
  rules.rotation.y = 0.9;
  ai.position.set(S.x + 2.1, lineY, S.z + 0.9);
  ai.rotation.y = -0.9;
  const bandGeo = new THREE.BoxGeometry(1, 0.04, 0.04).translate(0.5, 0, 0);
  const bandL = new THREE.Mesh(bandGeo, kit.mat.teal);
  const bandR = new THREE.Mesh(bandGeo, kit.mat.teal);
  bandL.position.copy(rules.position);
  bandR.position.copy(ai.position);
  const toLine = (from: THREE.Vector3) => Math.atan2(-(S.z + 0.06 - from.z), S.x - from.x);
  bandL.rotation.y = toLine(rules.position);
  bandR.rotation.y = toLine(ai.position);
  const reach = rules.position.distanceTo(new THREE.Vector3(S.x - 0.45, lineY, S.z + 0.06));
  const shades = [shade(kit, rules.position.x, rules.position.z, 1.4, 0.7), shade(kit, ai.position.x, ai.position.z, 1.4, 0.7)];
  group.add(rules, ai, bandL, bandR, ...shades);

  const lit = new THREE.Color(COLOR.tealBright);
  const stone = new THREE.Color(COLOR.stone);
  const teal = new THREE.Color(COLOR.teal);
  return {
    group,
    update(p) {
      const show = p > 0.74;
      rules.visible = ai.visible = shades[0].visible = shades[1].visible = show;
      const r = ease(span(p, 0.758, 0.772));
      bandL.visible = bandR.visible = show && r > 0;
      bandL.scale.x = bandR.scale.x = Math.max(0.001, r * reach);
      const met = r >= 1;
      lineMat.color.copy(met && p > 0.74 ? teal : stone);
      rulesMat.color.copy(show && r > 0 ? lit : stone);
      aiMat.color.copy(show && r > 0 ? lit : stone);
      return false;
    },
  };
}

/** The receipt: the thread's knot pressed into a seal, a card that rides the thread home. */
export function receipt(c: Cast): Actor & { at: THREE.Vector3 } {
  const { kit } = c;
  const card = new THREE.Group();
  card.add(new THREE.Mesh(new THREE.BoxGeometry(0.72, 0.94, 0.04), kit.mat.surface));
  const lineGeo = new THREE.BoxGeometry(0.5, 0.045, 0.02);
  for (let i = 0; i < 4; i++) {
    const l = new THREE.Mesh(lineGeo, kit.mat.stone);
    l.position.set(i === 0 ? -0.1 : 0, 0.3 - i * 0.13, 0.03);
    if (i === 0) l.scale.x = 0.55;
    card.add(l);
  }
  const seal = new THREE.Mesh(new THREE.CylinderGeometry(0.11, 0.11, 0.03, 32).rotateX(Math.PI / 2), kit.mat.teal);
  seal.position.set(0, -0.3, 0.035);
  card.add(seal);
  const at = new THREE.Vector3();
  return {
    group: card,
    at,
    update(p, _now, camera) {
      const born = ease(span(p, 0.806, 0.826));
      card.visible = born > 0 && p < 0.95;
      const u = receiptU(p);
      c.thread.getPointAt(Math.max(0.002, u * 0.985), at);
      at.y += 0.75 + Math.sin(u * 60) * 0.05;
      card.position.copy(at);
      card.scale.setScalar(Math.max(0.001, born));
      card.rotation.set(0, faceYaw(at, camera), Math.sin(u * 30) * 0.06);
      return false;
    },
  };
}

/** The nurse at the clinic window, calling: rings that widen and fade before they cross the sand. */
export function nurse(c: Cast): Actor {
  const { kit } = c;
  const group = new THREE.Group();
  const peb = kit.pebble(c.faces.nurse, c.unknown);
  const N = vec(PLACES.nurse);
  peb.group.position.set(N.x, 0.5, N.z);
  group.add(peb.group);
  const ringGeo = new THREE.RingGeometry(0.94, 1, 96).rotateX(-Math.PI / 2);
  const rings = [0, 1, 2].map(() => {
    const m = new THREE.Mesh(ringGeo, new THREE.MeshBasicMaterial({ color: COLOR.teal, transparent: true, opacity: 0, depthWrite: false }));
    m.position.set(N.x, 0.04, N.z + 1.4);
    m.renderOrder = 2;
    group.add(m);
    return m;
  });
  return {
    group,
    update(p) {
      const calling = p > 0.36 && p < RIDE.to;
      rings.forEach((m, i) => {
        m.visible = calling;
        if (!calling) return;
        const phase = (p * 70 + i / 3) % 1;
        m.scale.setScalar(1 + phase * 24);
        (m.material as THREE.MeshBasicMaterial).opacity = (1 - phase) ** 2 * 0.5;
      });
      return false;
    },
  };
}

/**
 * The other people the wide shots show: five at the hospital, each with a line back to a clinic.
 * In the gap only the ID-number matches have a (grey) line; at the end all are teal but one, held
 * amber with two people beside it.
 */
export function others(c: Cast): Actor {
  const { kit } = c;
  const group = new THREE.Group();
  const found = 5 - c.figures.missedOfFive;
  const spots = [0, 1, 2, 3, 4].map((i) => new THREE.Vector3(76.6 + i * 1.5, 0, 3.4 + (i % 2) * 0.9));
  const bodies = spots.map(() => kit.tint(COLOR.surface));
  const missing = spots.map((_, i) => i >= found);
  spots.forEach((s, i) => {
    const m = new THREE.Mesh(kit.geo.pebble, bodies[i]);
    m.position.set(s.x, 0.5, s.z);
    m.rotation.y = -0.2;
    group.add(m, shade(kit, s.x, s.z, 1.2));
  });
  const HELD = 3;
  const lines = spots.map((s, i) => {
    const a = new THREE.Vector3(2 + i * 1.8, 3.3, -1 - i * 0.6);
    const b = new THREE.Vector3(s.x, 0.95, s.z);
    const mid = a.clone().lerp(b, 0.5).setY(13 + i * 1.2);
    const tube = new THREE.TubeGeometry(new THREE.QuadraticBezierCurve3(a, mid, b), 64, 0.09, 5, false);
    const m = new THREE.Mesh(tube, kit.mat.grey);
    group.add(m);
    return m;
  });
  const helpers = [0, 1].map((k) => {
    const m = new THREE.Mesh(kit.geo.pebble, kit.mat.unknown);
    m.scale.setScalar(0.75);
    m.position.set(spots[HELD].x - 0.7 + k * 1.4, 0.38, spots[HELD].z + 1.1);
    group.add(m);
    return m;
  });
  const surface = new THREE.Color(COLOR.surface);
  const unknown = new THREE.Color(COLOR.unknown);
  return {
    group,
    update(p) {
      const end = p > 0.9;
      group.visible = p > 0.415;
      lines.forEach((m, i) => {
        m.visible = end || (p > 0.415 && p < 0.56 && !missing[i]);
        m.material = end ? (i === HELD ? kit.mat.held : kit.mat.teal) : kit.mat.grey;
      });
      bodies.forEach((b, i) => b.color.copy(!end && missing[i] ? unknown : surface));
      helpers.forEach((h) => (h.visible = end));
      return false;
    },
  };
}

/** Other people crossing the streets, blank-faced, moving only as the visitor scrolls. */
export function passersby(c: Cast): Actor {
  const { kit } = c;
  const r = rand(5);
  const lanes = [
    [12.2, -6, 12.2, 16],
    [19.5, 15, 19.5, -4],
    [27, -8, 27.6, 14],
    [33.6, 16, 33, -3],
    [24, 11.5, 36, 11.8],
    [60.5, -4, 60.5, 15],
    [64.5, 16, 65, 9],
    [14, 2.6, 30, 3],
    [86, 12, 86, -6],
    [-6, 12, -6, -2],
  ].slice(0, c.mobile ? 6 : 10);
  const n = lanes.length;
  const bodies = new THREE.InstancedMesh(kit.geo.pebbleLow, kit.mat.raised, n);
  const shadows = new THREE.InstancedMesh(kit.geo.shadow, kit.mat.shadow, n);
  shadows.renderOrder = 1;
  bodies.frustumCulled = shadows.frustumCulled = false;
  const speed = lanes.map(() => 3 + r() * 3);
  const offset = lanes.map(() => r());
  const d = new THREE.Object3D();
  const a = new THREE.Vector3();
  const b = new THREE.Vector3();
  return {
    group: new THREE.Group().add(bodies, shadows),
    update(p) {
      for (let i = 0; i < n; i++) {
        const [x0, z0, x1, z1] = lanes[i];
        a.set(x0, 0, z0);
        b.set(x1, 0, z1);
        const len = a.distanceTo(b);
        const raw = (p * speed[i] + offset[i]) % 2;
        const t = raw < 1 ? raw : 2 - raw;
        const g = gait(raw * len);
        tmp.copy(a).lerp(b, t);
        d.position.set(tmp.x, 0.5 + g.hop, tmp.z);
        d.rotation.set(0, Math.atan2(b.x - a.x, b.z - a.z) + (raw < 1 ? 0 : Math.PI) + Math.PI / 2, g.sway * 0.05);
        d.scale.set(0.9, 0.9 - g.squash * 0.08, 0.9);
        d.updateMatrix();
        bodies.setMatrixAt(i, d.matrix);
        d.position.set(tmp.x, 0.008, tmp.z);
        d.rotation.set(0, 0, 0);
        d.scale.setScalar(1.05);
        d.updateMatrix();
        shadows.setMatrixAt(i, d.matrix);
      }
      bodies.instanceMatrix.needsUpdate = true;
      shadows.instanceMatrix.needsUpdate = true;
      return false;
    },
  };
}

/** The low stone by the road where the 31-day standard runs out. */
export function standardStone(c: Cast): void {
  const { kit, road, figures } = c;
  const u = figures.standardDays / figures.waitDays;
  const p = road.getPointAt(u);
  const t = road.getTangentAt(u);
  const x = p.x - t.z * 1.35;
  const z = p.z + t.x * 1.35;
  const g = new THREE.CylinderGeometry(0.32, 0.36, 0.7, 24);
  kit.addStatic(kit.mat.stone, g, new THREE.Matrix4().setPosition(x, 0.35, z));
  const cap = new THREE.CylinderGeometry(0.02, 0.02, 0.5, 8);
  kit.addStatic(kit.mat.ink, cap, new THREE.Matrix4().setPosition(x, 0.95, z));
  kit.shadowAt(x, z, 1.1);
  g.dispose();
  cap.dispose();
}
