import * as THREE from "three";
import { CAMERA_KEYS, FACE_SEEDS, type CastKey, type FaceKey, type StoryFigures } from "./beats";
import { createKit, faceTexture } from "./kit";
import { COLOR } from "./palette";
import type { Part, World } from "./parts";

export interface StageOptions {
  canvas: HTMLCanvasElement;
  faces: Record<FaceKey, CanvasImageSource>;
  unknown: CanvasImageSource;
  figures: StoryFigures;
  mobile: boolean;
  /** Scene builders; each adds its props and returns the parts that move. */
  build: (w: World) => Part[];
  /** Called once if the first 60 frames run slower than 33 ms at the median. */
  onSlow: () => void;
}

export interface StageStats {
  renderer: string;
  calls: number;
  triangles: number;
  dpr: number;
  /** CPU time of each render() call, ms. */
  renderMs: number[];
}

export interface Stage {
  setProgress(p: number): void;
  setPerson(person: CastKey): void;
  resize(): void;
  dispose(): void;
  stats(): StageStats;
}

const smoother = (t: number) => t * t * t * (t * (t * 6 - 15) + 10);
const median = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)] ?? 0;

const keyTmp = new THREE.Vector3();

/** The camera pose for a scroll position: eased between the keys in beats.ts. */
function cameraAt(p: number, pos: THREE.Vector3, look: THREE.Vector3) {
  let i = 0;
  while (i < CAMERA_KEYS.length - 2 && p > CAMERA_KEYS[i + 1].p) i++;
  const a = CAMERA_KEYS[i];
  const b = CAMERA_KEYS[i + 1];
  const t = smoother(Math.min(1, Math.max(0, (p - a.p) / (b.p - a.p || 1))));
  pos.set(a.pos[0], a.pos[1], a.pos[2]).lerp(keyTmp.set(b.pos[0], b.pos[1], b.pos[2]), t);
  look.set(a.look[0], a.look[1], a.look[2]).lerp(keyTmp.set(b.look[0], b.look[1], b.look[2]), t);
}

export function createStage(o: StageOptions): Stage {
  const renderer = new THREE.WebGLRenderer({ canvas: o.canvas, antialias: !o.mobile, powerPreference: "high-performance" });
  const gl = renderer.getContext();
  const dbg = gl.getExtension("WEBGL_debug_renderer_info");
  const rendererName = String(dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER));
  const dprCap = Math.min(window.devicePixelRatio || 1, 1.5);
  let dpr = dprCap;
  renderer.setPixelRatio(dpr);
  renderer.setClearColor(COLOR.ground);

  const scene = new THREE.Scene();
  const FOG_NEAR = 16;
  const FOG_FAR = 64;
  const fog = new THREE.Fog(COLOR.ground, FOG_NEAR, FOG_FAR);
  scene.fog = fog;
  const camera = new THREE.PerspectiveCamera(o.mobile ? 38 : 30, 1, 0.5, 120);

  const kit = createKit(o.mobile);
  const faces = {} as Record<FaceKey, THREE.Texture>;
  for (const k of Object.keys(FACE_SEEDS) as FaceKey[]) faces[k] = faceTexture(o.faces[k]);
  const unknown = faceTexture(o.unknown);
  const world: World = { kit, mobile: o.mobile, faces, unknown, figures: o.figures, fog, pick: { person: "fatima", at: -Infinity } };

  const floor = new THREE.Mesh(new THREE.PlaneGeometry(400, 400).rotateX(-Math.PI / 2), kit.mat.floor);
  floor.position.set(36, 0, 0);
  floor.matrixAutoUpdate = false;
  floor.updateMatrix();
  scene.add(floor);

  const parts = o.build(world);
  for (const part of parts) scene.add(part.group);
  for (const mesh of kit.mergeStatics()) scene.add(mesh);

  let target = 0;
  let shown = -1;
  const camLook = new THREE.Vector3();
  /** Narrow screens see less of the room at the same distance, so the camera steps back along its view line. */
  let pull = 1;

  let raf = 0;
  let last = 0;
  let warm = 60;
  const warmGaps: number[] = [];
  const renderMs: number[] = [];
  const gaps: number[] = [];
  let lastDprChange = 0;
  let fastSince = 0;

  function invalidate() {
    if (!raf) raf = requestAnimationFrame(frame);
  }

  function adapt(gap: number, now: number) {
    gaps.push(gap);
    if (gaps.length > 30) gaps.shift();
    if (gaps.length < 30 || now - lastDprChange < 1000) return;
    const m = median(gaps);
    if (m > 14 && dpr > 1) {
      dpr = Math.max(1, dpr - 0.25);
    } else if (m < 9 && dpr < dprCap) {
      fastSince ||= now;
      if (now - fastSince < 2000) return;
      dpr = Math.min(dprCap, dpr + 0.25);
    } else {
      fastSince = 0;
      return;
    }
    fastSince = 0;
    lastDprChange = now;
    renderer.setPixelRatio(dpr);
    gaps.length = 0;
  }

  function frame(now: number) {
    raf = 0;
    const gap = last ? now - last : 0;
    const dt = gap ? Math.min(0.1, gap / 1000) : 1 / 60;
    const continuous = gap > 0 && gap < 40;
    last = now;

    shown = shown < 0 ? target : shown + (target - shown) * (1 - Math.exp(-dt * 7));
    if (Math.abs(target - shown) < 1e-5) shown = target;
    cameraAt(shown, camera.position, camLook);
    if (pull !== 1) camera.position.sub(camLook).multiplyScalar(pull).add(camLook);
    camera.lookAt(camLook);

    fog.far = FOG_FAR;
    let busy = shown !== target;
    for (const part of parts) {
      const on = shown >= part.window[0] && shown <= part.window[1];
      part.group.visible = on;
      if (on && part.update(shown, now)) busy = true;
    }
    fog.near = FOG_NEAR * pull;
    fog.far *= pull;

    const t0 = performance.now();
    renderer.render(scene, camera);
    renderMs.push(performance.now() - t0);
    if (renderMs.length > 600) renderMs.shift();

    if (warm > 0) {
      // The warm-up asks for every frame, so each gap counts, however long.
      if (gap) warmGaps.push(gap);
      if (--warm === 0 && median(warmGaps) > 33) o.onSlow();
      busy = true;
    } else if (continuous) {
      adapt(gap, now);
    }
    if (busy) invalidate();
    else last = 0;
  }

  function resize() {
    const { clientWidth: w, clientHeight: h } = o.canvas;
    if (!w || !h) return;
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    pull = camera.aspect < 0.8 ? 1.9 : camera.aspect < 1.3 ? 1.3 : 1;
    // The copy owns the left third on desktop and the lower part on phones; push the subject away from it.
    if (o.mobile) camera.setViewOffset(w, h, 0, h * 0.27, w, h);
    else camera.setViewOffset(w, h, -w * 0.15, 0, w, h);
    camera.updateProjectionMatrix();
    invalidate();
  }

  resize();
  renderer.compile(scene, camera);
  invalidate();

  return {
    setProgress(p) {
      target = p;
      invalidate();
    },
    setPerson(person) {
      world.pick = { person, at: performance.now() };
      invalidate();
    },
    resize,
    stats: () => ({ renderer: rendererName, calls: renderer.info.render.calls, triangles: renderer.info.render.triangles, dpr, renderMs: [...renderMs] }),
    dispose() {
      cancelAnimationFrame(raf);
      scene.traverse((obj) => {
        if (!(obj instanceof THREE.Mesh)) return;
        obj.geometry.dispose();
        (obj.material as THREE.Material).dispose();
      });
      for (const t of [...Object.values(faces), unknown]) t.dispose();
      kit.dispose();
      renderer.dispose();
    },
  };
}
