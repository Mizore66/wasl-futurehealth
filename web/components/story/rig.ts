import * as THREE from "three";
import { smoother } from "./script";
import { PLACES } from "./world";

/**
 * The camera reads one table of keys. Each key names a target (a subject that moves with `p`, or a
 * fixed point), an offset from it and a look offset, in world units. Between two keys the camera
 * eases (smootherstep); two keys with the same pose make a hold. Phones get their own pose where the
 * portrait frame needs one.
 */

export type Target = "fatima" | "tip" | "receipt" | readonly [number, number, number];
type V3 = readonly [number, number, number];
interface Pose {
  target: Target;
  off: V3;
  look: V3;
  fov: number;
  /** Linear fog from the camera, near and far. */
  fog: readonly [number, number];
}
export interface CameraKey extends Pose {
  p: number;
  phone?: Partial<Pose>;
}

const DOOR_B: V3 = [72, 1.05, 2.9];
const SLAB: V3 = [PLACES.slab[0], PLACES.slab[1], PLACES.slab[2]];
const WINDOW: V3 = [PLACES.window[0], 1.2, 1.5];
const MID: V3 = [39, 0, 0];

const near: Pose["fog"] = [16, 46];
const city: Pose["fog"] = [22, 60];
const far: Pose["fog"] = [120, 230];

const hold = (p: number, k: Omit<CameraKey, "p">): CameraKey => ({ p, ...k });

const CLOSE: Omit<CameraKey, "p"> = {
  target: "fatima",
  off: [1.8, 4.4, 11.6],
  look: [-2.6, 1.5, -2.5],
  fov: 30,
  fog: near,
  phone: { off: [1.5, 4.5, 13], look: [0, 0.6, 0], fov: 36 },
};
const DOORWAY: Omit<CameraKey, "p"> = {
  target: "fatima",
  off: [6, 5.5, 14],
  look: [1.8, 1.2, -2.5],
  fov: 30,
  fog: near,
  phone: { off: [5, 6.5, 17], look: [1, 1, -2], fov: 40 },
};
const TRACK: Omit<CameraKey, "p"> = {
  target: "fatima",
  off: [-1.0, 3.4, 9.5],
  look: [3.4, 0.6, -3],
  fov: 30,
  fog: city,
  phone: { off: [-2, 4.5, 11], look: [1, 0.4, -1], fov: 40 },
};
const AT_DOOR: Omit<CameraKey, "p"> = {
  target: DOOR_B,
  off: [6, 5, 14],
  look: [0.6, 1.2, -1.5],
  fov: 30,
  fog: near,
  phone: { off: [4, 6, 15], look: [0.3, 0.6, -0.8], fov: 40 },
};
const WIDE: Omit<CameraKey, "p"> = {
  target: MID,
  off: [0, 74, 80],
  look: [0, 0, 0],
  fov: 30,
  fog: far,
  phone: { target: [38, 0, 3], off: [100, 120, 30], fov: 34 },
};
/** The wide shot tilted up, so the city sits under the closing lines instead of behind them. */
const END: Omit<CameraKey, "p"> = { ...WIDE, look: [0, 13, 0], phone: { ...WIDE.phone, off: [130, 156, 39], look: [0, -20, 0], fog: [150, 300] } };
const BEHIND_TIP: Omit<CameraKey, "p"> = {
  target: "tip",
  off: [-6.5, 3.6, 4.8],
  look: [4, 0, -0.8],
  fov: 30,
  fog: city,
  phone: { off: [-5, 3.5, 5], look: [1.2, 0, -0.3], fov: 40 },
};
const TWO_SHOT: Omit<CameraKey, "p"> = {
  target: DOOR_B,
  off: [1.2, 1.6, 7.6],
  look: [-0.6, 0.1, 1.4],
  fov: 30,
  fog: near,
  phone: { off: [1, 3.5, 14], look: [-0.8, 0.3, 1.4], fov: 40 },
};
const FACE_ON: Omit<CameraKey, "p"> = {
  target: SLAB,
  off: [0, 1.4, 10],
  look: [0, -0.2, 0],
  fov: 30,
  fog: near,
  phone: { off: [0, 2, 14], look: [0, 0, 0], fov: 40 },
};
const BEHIND_CARD: Omit<CameraKey, "p"> = {
  target: "receipt",
  off: [5.2, 3.4, 3.8],
  look: [-3, -0.6, 0],
  fov: 30,
  fog: city,
  phone: { off: [6, 5, 7], look: [-1.5, -0.6, 0], fov: 40 },
};
const HOME: Omit<CameraKey, "p"> = {
  target: WINDOW,
  off: [2.4, 2.4, 10],
  look: [1.8, 0.3, 0],
  fov: 30,
  fog: near,
  phone: { off: [1.5, 3, 13], look: [0.8, 0, 0], fov: 40 },
};

export const CAMERA_KEYS: readonly CameraKey[] = [
  hold(0, CLOSE),
  hold(0.104, { ...CLOSE, off: [1.6, 4.0, 10.6] }),
  hold(0.11, DOORWAY),
  hold(0.148, DOORWAY),
  hold(0.168, { ...TRACK, off: [-4.6, 2.4, 3.4], look: [3, 0.3, 0] }),
  hold(0.19, TRACK),
  hold(0.31, TRACK),
  hold(0.338, AT_DOOR),
  hold(0.415, AT_DOOR),
  hold(0.44, WIDE),
  hold(0.543, WIDE),
  hold(0.558, BEHIND_TIP),
  hold(0.634, BEHIND_TIP),
  hold(0.642, TWO_SHOT),
  hold(0.746, TWO_SHOT),
  hold(0.756, FACE_ON),
  hold(0.826, FACE_ON),
  hold(0.836, BEHIND_CARD),
  hold(0.866, BEHIND_CARD),
  hold(0.876, HOME),
  hold(0.908, HOME),
  hold(0.935, END),
  hold(1, END),
];

export type Subjects = Record<"fatima" | "tip" | "receipt", THREE.Vector3>;

const a = new THREE.Vector3();
const b = new THREE.Vector3();
const la = new THREE.Vector3();
const lb = new THREE.Vector3();

function poseOf(k: CameraKey, phone: boolean, subjects: Subjects, pos: THREE.Vector3, look: THREE.Vector3): { fov: number; fog: Pose["fog"] } {
  const q: Pose = phone && k.phone ? { ...k, ...k.phone } : k;
  const t = q.target;
  if (typeof t === "string") look.copy(subjects[t]);
  else look.set(t[0], t[1], t[2]);
  pos.set(look.x + q.off[0], look.y + q.off[1], look.z + q.off[2]);
  look.x += q.look[0];
  look.y += q.look[1];
  look.z += q.look[2];
  return { fov: q.fov, fog: q.fog };
}

/** The camera pose for a scroll position, given where the moving subjects are at that position. */
export function cameraAt(p: number, phone: boolean, subjects: Subjects, pos: THREE.Vector3, look: THREE.Vector3) {
  let i = 0;
  while (i < CAMERA_KEYS.length - 2 && p > CAMERA_KEYS[i + 1].p) i++;
  const ka = CAMERA_KEYS[i];
  const kb = CAMERA_KEYS[i + 1];
  const t = smoother(Math.min(1, Math.max(0, (p - ka.p) / (kb.p - ka.p || 1))));
  const A = poseOf(ka, phone, subjects, a, la);
  const B = poseOf(kb, phone, subjects, b, lb);
  pos.copy(a).lerp(b, t);
  look.copy(la).lerp(lb, t);
  const lerp = THREE.MathUtils.lerp;
  // Fog distances ease on a log scale, so a crane from the street to the wide shot opens up evenly.
  const lg = (x: number, y: number) => Math.exp(lerp(Math.log(x), Math.log(y), t));
  return { fov: lerp(A.fov, B.fov, t), fogNear: lg(A.fog[0], B.fog[0]), fogFar: lg(A.fog[1], B.fog[1]) };
}
