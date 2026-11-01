import * as THREE from "three";
import { fatima, makeThread, nurse, others, passersby, receipt, receiptU, slabAndReaders, standardStone, thread, tipU, type Actor, type Cast } from "./actors";
import { createKit, faceTexture } from "./kit";
import { COLOR } from "./palette";
import { cameraAt, type Subjects } from "./rig";
import { CUES, FACE_SEEDS, walkAt, type AnchorId, type CastKey, type FaceKey, type StoryFigures } from "./script";
import { PLACES, buildCity, buildClinic, buildHospital, buildPaving, makeRoad } from "./world";

export interface StageOptions {
  canvas: HTMLCanvasElement;
  faces: Record<FaceKey, CanvasImageSource>;
  unknown: CanvasImageSource;
  figures: StoryFigures;
  mobile: boolean;
  /** Called once if the first 60 frames run slower than 33 ms at the median. */
  onSlow: () => void;
  /** Where each anchored line of copy should sit, as fractions of the viewport. Called on every resize. */
  onAnchors: (anchors: Partial<Record<AnchorId, { x: number; y: number }>>) => void;
}

export interface StageStats {
  renderer: string;
  calls: number;
  triangles: number;
  dpr: number;
  /** CPU time of each render() call, ms. */
  renderMs: number[];
  /** Frames drawn since load; the idle check reads it. */
  frames: number;
}

export interface Stage {
  setProgress(p: number): void;
  setPerson(person: CastKey): void;
  resize(): void;
  dispose(): void;
  stats(): StageStats;
}

const median = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)] ?? 0;

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
  const fog = new THREE.Fog(COLOR.ground, 16, 46);
  scene.fog = fog;
  const camera = new THREE.PerspectiveCamera(30, 1, 0.3, 400);

  const kit = createKit(o.mobile);
  const faces = {} as Record<FaceKey, THREE.Texture>;
  for (const k of Object.keys(FACE_SEEDS) as FaceKey[]) faces[k] = faceTexture(o.faces[k]);
  const unknown = faceTexture(o.unknown);

  const road = makeRoad();
  const cast: Cast = {
    kit,
    mobile: o.mobile,
    faces,
    unknown,
    figures: o.figures,
    road,
    thread: makeThread(road),
    pick: { person: "fatima", at: -Infinity },
  };

  const floor = new THREE.Mesh(new THREE.PlaneGeometry(700, 700).rotateX(-Math.PI / 2), kit.mat.floor);
  floor.position.set(39, 0, 0);
  floor.matrixAutoUpdate = false;
  floor.updateMatrix();
  scene.add(floor, buildPaving(kit, road));

  const chunks = buildCity(kit, road, o.mobile);
  for (const c of chunks) scene.add(c.near, c.far, c.shadow);
  buildClinic(kit);
  buildHospital(kit);
  standardStone(cast);

  const her = fatima(cast);
  const line = thread(cast, her.at);
  const card = receipt(cast);
  const actors: Actor[] = [her, line, card, slabAndReaders(cast), nurse(cast), others(cast), passersby(cast)];
  for (const a of actors) scene.add(a.group);
  for (const mesh of kit.mergeStatics()) scene.add(mesh);

  const subjects: Subjects = { fatima: new THREE.Vector3(), tip: new THREE.Vector3(), receipt: new THREE.Vector3() };
  function locate(p: number) {
    road.getPointAt(walkAt(p), subjects.fatima).setY(0.5);
    cast.thread.getPointAt(Math.max(0.001, tipU(p, line.ring1)), subjects.tip);
    cast.thread.getPointAt(Math.max(0.002, receiptU(p) * 0.985), subjects.receipt).y += 0.75;
  }

  let target = 0;
  let shown = -1;
  const look = new THREE.Vector3();
  let pull = 1;

  function pose(p: number) {
    locate(p);
    const view = cameraAt(p, o.mobile, subjects, camera.position, look);
    if (pull !== 1) camera.position.sub(look).multiplyScalar(pull).add(look);
    camera.lookAt(look);
    if (camera.fov !== view.fov) {
      camera.fov = view.fov;
      camera.updateProjectionMatrix();
    }
    return view;
  }

  let raf = 0;
  let last = 0;
  let warm = 60;
  let frames = 0;
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
    frames++;
    const gap = last ? now - last : 0;
    const dt = gap ? Math.min(0.1, gap / 1000) : 1 / 60;
    const continuous = gap > 0 && gap < 40;
    last = now;

    // Frame-rate independent damping (half-life about 90 ms), so trackpad noise never shows.
    shown = shown < 0 ? target : shown + (target - shown) * (1 - Math.exp(-dt * 7.7));
    if (Math.abs(target - shown) < 2e-5) shown = target;
    const view = pose(shown);
    fog.near = view.fogNear * pull;
    fog.far = view.fogFar * pull;

    // Chunks past the fog are hidden; far ones draw plain boxes.
    const wide = fog.far > 100;
    for (const c of chunks) {
      const d = Math.abs(c.x - camera.position.x);
      const on = d < fog.far + 12;
      const close = on && !wide && d < 30;
      c.near.visible = close;
      c.far.visible = on && !close;
      c.shadow.visible = on && !wide;
    }

    let busy = shown !== target;
    for (const a of actors) if (a.update(shown, now, camera)) busy = true;

    const t0 = performance.now();
    renderer.render(scene, camera);
    renderMs.push(performance.now() - t0);
    if (renderMs.length > 600) renderMs.shift();

    if (warm > 0) {
      if (gap) warmGaps.push(gap);
      if (--warm === 0 && median(warmGaps) > 33) o.onSlow();
      busy = true;
    } else if (continuous) {
      adapt(gap, now);
    }
    if (busy) invalidate();
    else last = 0;
  }

  const ANCHOR: Record<AnchorId, (s: Subjects) => THREE.Vector3> = {
    "fatima-door": (s) => s.fatima.clone().add(new THREE.Vector3(-0.75, 0.3, 0)),
    "clinic-door": () => new THREE.Vector3(PLACES.clinicDoor[0] + 1.1, 2.4, PLACES.clinicDoor[2]),
    walker: (s) => s.fatima.clone().add(new THREE.Vector3(0.6, 0.8, 0)),
    slab: () => new THREE.Vector3(PLACES.slab[0] + 0.75, PLACES.slab[1] + 0.5, PLACES.slab[2]),
    report: () => new THREE.Vector3(PLACES.slab[0] + 0.7, PLACES.slab[1] + 1.15, PLACES.slab[2]),
    nurse: () => new THREE.Vector3(PLACES.window[0], 3.4, PLACES.window[2]),
    gap: () => new THREE.Vector3(PLACES.gap[0], 0, PLACES.gap[2]),
    tip: (s) => s.tip.clone().add(new THREE.Vector3(0, 0.5, 0)),
    receipt: (s) => s.receipt.clone().add(new THREE.Vector3(0, 0.95, 0)),
  };

  /** Projects each anchored cue's point once, with the camera where it holds during that cue. */
  function project() {
    const out: Partial<Record<AnchorId, { x: number; y: number }>> = {};
    for (const c of CUES) {
      if (!("anchor" in c.place)) continue;
      const p = c.from + (c.to - c.from) * 0.5;
      pose(p);
      camera.updateMatrixWorld();
      const v = ANCHOR[c.place.anchor](subjects).project(camera);
      out[c.place.anchor] = { x: (v.x + 1) / 2, y: (1 - v.y) / 2 };
    }
    o.onAnchors(out);
  }

  function resize() {
    const { clientWidth: w, clientHeight: h } = o.canvas;
    if (!w || !h) return;
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    pull = !o.mobile && camera.aspect < 1.3 ? 1.3 : 1;
    // On a phone the subject sits a third of the way down, above the text band.
    if (o.mobile) camera.setViewOffset(w, h, 0, h * 0.16, w, h);
    else camera.clearViewOffset();
    camera.updateProjectionMatrix();
    project();
    invalidate();
  }

  resize();
  pose(0);
  renderer.compile(scene, camera);
  invalidate();

  return {
    setProgress(p) {
      target = p;
      invalidate();
    },
    setPerson(person) {
      cast.pick = { person, at: performance.now() };
      invalidate();
    },
    resize,
    stats: () => ({
      renderer: rendererName,
      calls: renderer.info.render.calls,
      triangles: renderer.info.render.triangles,
      dpr,
      renderMs: [...renderMs],
      frames,
    }),
    dispose() {
      cancelAnimationFrame(raf);
      const seen = new Set<unknown>();
      scene.traverse((obj) => {
        if (!(obj instanceof THREE.Mesh)) return;
        for (const r of [obj.geometry, obj.material]) {
          if (seen.has(r)) continue;
          seen.add(r);
          r.dispose();
        }
      });
      for (const t of [...Object.values(faces), unknown]) t.dispose();
      kit.dispose();
      renderer.dispose();
    },
  };
}
