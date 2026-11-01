import * as THREE from "three";
import { CAST, PLACES, local, type CastKey, type FaceKey, type StoryFigures } from "./beats";
import type { Kit } from "./kit";
import { COLOR } from "./palette";

/** One piece of the room. `update` returns true while it still needs frames without any scroll change. */
export interface Part {
  group: THREE.Object3D;
  /** Scroll range (0..1) in which the part can be on screen; outside it the group is hidden. */
  window: readonly [number, number];
  update(p: number, now: number): boolean;
}

export interface World {
  kit: Kit;
  mobile: boolean;
  faces: Record<FaceKey, THREE.Texture>;
  unknown: THREE.Texture;
  figures: StoryFigures;
  /** The stage resets `far` every frame; a part may pull it in to let the room fade. */
  fog: THREE.Fog;
  /** The chip the visitor picked, and when; a change replays the join over 1.2 s. */
  pick: { person: CastKey; at: number };
}

export const clamp01 = (x: number) => Math.min(1, Math.max(0, x));
export const span = (s: number, a: number, b: number) => clamp01((s - a) / (b - a));
export const ease = (t: number) => t * t * (3 - 2 * t);
/** The one overshoot in the room: the "?" turning over. */
const easeBack = (t: number) => 1 + 2.2 * (t - 1) ** 3 + 1.2 * (t - 1) ** 2;
export const v3 = (a: readonly number[]) => new THREE.Vector3(a[0], a[1], a[2]);
export const m4 = () => new THREE.Matrix4();

const HERO = 1.4;
const PLINTH_H = 0.42;
const HERO_Y = PLINTH_H + 0.5 * HERO;

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
    p.toArray(pos, i * 6);
    pos[i * 6 + 0] -= (side.x * width) / 2;
    pos[i * 6 + 1] -= (side.y * width) / 2;
    pos[i * 6 + 2] -= (side.z * width) / 2;
    pos[i * 6 + 3] = p.x + (side.x * width) / 2;
    pos[i * 6 + 4] = p.y + (side.y * width) / 2;
    pos[i * 6 + 5] = p.z + (side.z * width) / 2;
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
    /** Show the first `u` (0..1) of the strip. */
    grow(u: number) {
      g.setDrawRange(0, Math.round(clamp01(u) * segments) * 6);
    },
  };
}

/** Arc-length position (0..1) on the curve nearest to a point. */
function uNear(curve: THREE.Curve<THREE.Vector3>, at: THREE.Vector3): number {
  let best = 0;
  let bestD = Infinity;
  const p = new THREE.Vector3();
  for (let i = 0; i <= 400; i++) {
    const d = curve.getPointAt(i / 400, p).distanceToSquared(at);
    if (d < bestD) [best, bestD] = [i / 400, d];
  }
  return best;
}

/**
 * The two places and the thread between them: the clinic (a low plinth before a wall) and, across the
 * room, the hospital (a plinth under an arch). Beat 1 rolls Fatima from one to the other; beat 3 joins them.
 */
export function places(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const C = v3(PLACES.clinic);
  const H = v3(PLACES.hospital);

  const plinth = new THREE.CylinderGeometry(1.05, 1.15, PLINTH_H, w.mobile ? 32 : 56);
  for (const at of [C, H]) {
    kit.addStatic(kit.mat.stone, plinth, m4().setPosition(at.x, PLINTH_H / 2, at.z));
    kit.shadowAt(at.x, at.z, 3.4);
  }
  kit.addStatic(kit.mat.raised, new THREE.BoxGeometry(3.2, 1.5, 0.22), m4().setPosition(C.x, 0.75, C.z - 1.6));
  const pillar = new THREE.CylinderGeometry(0.2, 0.22, 1.7, 20);
  kit.addStatic(kit.mat.raised, pillar, m4().setPosition(H.x - 1.75, 0.85, H.z - 1.4));
  kit.addStatic(kit.mat.raised, pillar, m4().setPosition(H.x + 1.75, 0.85, H.z - 1.4));
  kit.addStatic(kit.mat.raised, new THREE.TorusGeometry(1.75, 0.2, 14, 56, Math.PI), m4().setPosition(H.x, 1.7, H.z - 1.4));
  kit.shadowAt(H.x - 1.75, H.z - 1.4, 1.1);
  kit.shadowAt(H.x + 1.75, H.z - 1.4, 1.1);
  plinth.dispose();
  pillar.dispose();

  const clinic = kit.pebble(w.faces.fatima, w.unknown);
  clinic.group.scale.setScalar(HERO);
  clinic.group.position.set(C.x, HERO_Y, C.z);
  const hospital = kit.pebble(w.faces.fatima, w.unknown);
  hospital.group.scale.setScalar(HERO);
  group.add(clinic.group, hospital.group);

  const travelShadow = new THREE.Mesh(kit.geo.shadow, kit.mat.shadow);
  travelShadow.scale.setScalar(1.6);
  group.add(travelShadow);

  const ringGeo = new THREE.TorusGeometry(0.8, 0.065, 12, w.mobile ? 48 : 72);
  const rings = [PLACES.ring1, PLACES.ring2].map((at) => {
    const material = kit.tint(COLOR.stone);
    const mesh = new THREE.Mesh(ringGeo, material);
    mesh.position.set(at[0], HERO_Y, at[2]);
    mesh.rotation.y = Math.PI / 2 - 0.5;
    kit.shadowAt(at[0], at[2], 1.2);
    group.add(mesh);
    return material;
  });

  const curve = new THREE.CatmullRomCurve3(
    [
      [C.x + 0.75, HERO_Y, 0.05],
      [24.7, HERO_Y + 0.12, 0.3],
      [PLACES.ring1[0], HERO_Y, 0],
      [28, HERO_Y - 0.12, -0.25],
      [PLACES.ring2[0], HERO_Y, 0],
      [31.3, HERO_Y + 0.12, 0.28],
      [H.x - 0.78, HERO_Y, 0.02],
    ].map(v3),
    false,
    "centripetal",
  );
  const thread = ribbon(curve, w.mobile ? 140 : 260, 0.13, Math.PI * 1.2);
  const threadMesh = new THREE.Mesh(thread.geometry, kit.mat.teal);
  group.add(threadMesh);
  const u1 = uNear(curve, v3([PLACES.ring1[0], HERO_Y, 0]));
  const u2 = uNear(curve, v3([PLACES.ring2[0], HERO_Y, 0]));

  const fray = new THREE.Group();
  const strand = ribbon(new THREE.LineCurve3(v3([0, 0, 0]), v3([0.9, 0, 0])), 6, 0.05, 0.8).geometry;
  for (const [ry, rz] of [
    [0.35, 0.3],
    [-0.3, -0.05],
    [0.1, -0.35],
  ]) {
    const m = new THREE.Mesh(strand, kit.mat.held);
    m.rotation.set(0, ry, rz);
    fray.add(m);
  }
  fray.position.copy(curve.getPointAt(u2 - 0.015));
  group.add(fray);

  const reviewers = [0, 1].map(() => {
    const m = new THREE.Mesh(kit.geo.pebble, kit.mat.unknown);
    m.scale.setScalar(0.8);
    group.add(m);
    return m;
  });

  // The nurse still calling: a ring that leaves the clinic every two seconds while her plinth stands alone.
  const callMat = new THREE.MeshBasicMaterial({ color: COLOR.teal, transparent: true, opacity: 0, depthWrite: false });
  const call = new THREE.Mesh(new THREE.RingGeometry(1.2, 1.28, 64).rotateX(-Math.PI / 2), callMat);
  call.position.set(C.x, 0.02, C.z);
  group.add(call);

  const bead = new THREE.Mesh(new THREE.SphereGeometry(0.12, 16, 12), kit.mat.teal);
  group.add(bead);
  const tmp = new THREE.Vector3();
  const lit = new THREE.Color(COLOR.tealBright);
  const held = new THREE.Color(COLOR.held);
  const stone = new THREE.Color(COLOR.stone);

  return {
    group,
    window: [0.09, 0.56],
    update(p, now) {
      const who = CAST[w.pick.person];
      if (clinic.frontMat.map !== w.faces[w.pick.person]) {
        clinic.frontMat.map = w.faces[w.pick.person];
        hospital.frontMat.map = w.faces[w.pick.person];
      }

      // Beat 1: she leaves the clinic and arrives at the hospital as a "?".
      const roll = ease(span(local(p, "unknown"), 0.05, 0.4));
      const x = THREE.MathUtils.lerp(C.x + 1.5, H.x, roll);
      const hop = Math.sin(roll * Math.PI) * 0.25;
      hospital.group.position.set(x, HERO_Y + hop, H.z + Math.sin(roll * Math.PI) * 1.6);
      travelShadow.position.set(x, 0.006, hospital.group.position.z);
      travelShadow.visible = roll > 0 && roll < 1;
      const asked = span(roll, 0.7, 0.95);
      const calling = roll >= 1 && p < 0.19;
      call.visible = calling;
      if (calling) {
        const phase = (now % 2000) / 2000;
        call.scale.setScalar(1 + phase * 0.9);
        callMat.opacity = (1 - phase) * 0.55;
      }

      // Beat 3: the thread tries the name, then the birth date; replayed on a chip change.
      const replay = clamp01((now - w.pick.at) / 1200);
      const s = local(p, "join") * ease(replay);
      const reach1 = span(s, 0.12, 0.3);
      const reach2 = span(s, 0.5, 0.68);
      const end = who.joined ? 1 : u2 - 0.02;
      thread.grow(reach2 > 0 ? THREE.MathUtils.lerp(u1 + 0.03, end, ease(reach2)) : reach1 * (u1 + 0.03));
      threadMesh.visible = s > 0.12;
      rings[0].color.copy(s > 0.28 ? lit : stone);
      rings[1].color.copy(s > 0.48 ? (who.joined ? lit : held) : stone);

      const turn = who.joined ? easeBack(span(s, 0.68, 0.8)) : 0;
      hospital.group.rotation.y = Math.PI * ease(asked) * (1 - turn);

      const pulse = span(s, 0.8, 0.92);
      bead.visible = who.joined && pulse > 0 && pulse < 1;
      if (bead.visible) bead.position.copy(curve.getPointAt(1 - pulse, tmp));

      const frayed = who.joined ? 0 : ease(span(s, 0.52, 0.64));
      fray.visible = frayed > 0;
      fray.scale.setScalar(Math.max(frayed, 0.001));
      const come = who.joined ? 0 : ease(span(s, 0.56, 0.76));
      reviewers.forEach((r, i) => {
        r.visible = come > 0;
        r.position.set(H.x - 0.9 + i * 0.95, 0.5 * 0.8 + 0.02, THREE.MathUtils.lerp(-4, 1.9, come));
      });

      return replay < 1 || calling;
    },
  };
}
