import * as THREE from "three";
import { RoundedBoxGeometry } from "three/addons/geometries/RoundedBoxGeometry.js";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import type { Cast } from "./actors";
import { COLOR } from "./palette";
import { JOIN_BARS, RESOLVE, ease, span, type BarState, type Outcome } from "./script";
import { rand } from "./world";

/**
 * The identity moment at the hospital door: scan, compare, lock, resolve. Every part is a function of
 * the join's progress `j` and the chip picked, so it reverses with the scroll and replays with a chip.
 * The ground is pale paper, where additive light washes out to white, so the coloured parts blend
 * normally; only the flash at the centre of the burst is additive, over a dark pool on the ground.
 */

const BAR_COLOR: Record<BarState, THREE.Color> = {
  similar: new THREE.Color(COLOR.held),
  agrees: new THREE.Color(COLOR.thread),
  differs: new THREE.Color(COLOR.differs),
  missing: new THREE.Color(COLOR.stone),
};

const CARD = { w: 0.86, h: 0.7, bar: 0.6, thick: 0.1 };
const BAR_Y = [0.18, 0, -0.18];

function frameGeometry(w: number, h: number, t: number): THREE.BufferGeometry {
  const sides = [
    new THREE.BoxGeometry(w, t, 0.01).translate(0, (h - t) / 2, 0),
    new THREE.BoxGeometry(w, t, 0.01).translate(0, -(h - t) / 2, 0),
    new THREE.BoxGeometry(t, h, 0.01).translate(-(w - t) / 2, 0, 0),
    new THREE.BoxGeometry(t, h, 0.01).translate((w - t) / 2, 0, 0),
  ];
  const g = mergeGeometries(sides)!;
  for (const s of sides) s.dispose();
  return g;
}

/** Where the two records float, facing the shot that watches the join. */
const WATCHED_FROM = new THREE.Vector3(73.2, 2.65, 10.5);
const CARDS: Record<"desktop" | "phone", readonly (readonly [number, number, number])[]> = {
  desktop: [
    [69.95, 1.5, 5.5],
    [73.72, 1.84, 3.1],
  ],
  phone: [
    [70.1, 1.6, 5.5],
    [72.72, 2.12, 3.1],
  ],
};

function ringTexture(): THREE.CanvasTexture {
  const c = document.createElement("canvas");
  c.width = c.height = 128;
  const g = c.getContext("2d")!;
  const r = g.createRadialGradient(64, 64, 0, 64, 64, 64);
  r.addColorStop(0, "rgba(255,255,255,0.1)");
  r.addColorStop(0.72, "rgba(255,255,255,0.28)");
  r.addColorStop(0.88, "rgba(255,255,255,1)");
  r.addColorStop(1, "rgba(255,255,255,0)");
  g.fillStyle = r;
  g.fillRect(0, 0, 128, 128);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

export function resolveEffect(c: Cast, ring1: number) {
  const { kit } = c;
  const group = new THREE.Group();
  group.visible = false;
  const ringMap = ringTexture();
  const soft = (map: THREE.Texture, color: string, blending: THREE.Blending = THREE.NormalBlending) =>
    new THREE.MeshBasicMaterial({ map, color, transparent: true, depthWrite: false, blending, opacity: 0 });
  const flat = new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2);
  const upright = new THREE.PlaneGeometry(1, 1);

  const pool = new THREE.Mesh(flat, soft(kit.glowMap, "#04302e"));
  pool.scale.setScalar(5.4);
  pool.renderOrder = 1;
  const disc = new THREE.Mesh(flat, soft(ringMap, COLOR.thread));
  disc.renderOrder = 3;
  const shock = new THREE.Mesh(flat, soft(ringMap, COLOR.thread));
  shock.renderOrder = 2;
  const flash = new THREE.Mesh(upright, soft(kit.glowMap, "#ffffff", THREE.AdditiveBlending));
  flash.renderOrder = 4;
  const lockGlow = new THREE.Mesh(upright, soft(kit.glowMap, COLOR.thread));
  lockGlow.scale.setScalar(1.05);
  lockGlow.renderOrder = 4;
  group.add(pool, disc, shock, flash, lockGlow);

  const boardGeo = new RoundedBoxGeometry(CARD.w, CARD.h, 0.05, 2, 0.022);
  const boardMat = kit.tint(COLOR.surface);
  const printGeo = mergeGeometries([
    new THREE.BoxGeometry(CARD.bar, CARD.thick, 0.01).translate(0, BAR_Y[0], 0),
    new THREE.BoxGeometry(CARD.bar, CARD.thick, 0.01).translate(0, BAR_Y[1], 0),
    frameGeometry(CARD.bar, CARD.thick, 0.018).translate(0, BAR_Y[2], 0),
  ])!;
  const printMat = new THREE.MeshBasicMaterial({ color: COLOR.stone });
  const fillGeo = new THREE.BoxGeometry(1, CARD.thick, 0.012).translate(0.5, 0, 0);
  const fillMat = new THREE.MeshBasicMaterial();
  const scanGeo = new THREE.PlaneGeometry(CARD.w + 0.1, 0.09);
  const scanMat = new THREE.MeshBasicMaterial({ color: COLOR.thread, transparent: true, depthWrite: false, opacity: 0.95 });

  const d = new THREE.Object3D();
  const places = CARDS[c.mobile ? "phone" : "desktop"];
  const cards = places.map(([x, y, z]) => {
    const card = new THREE.Group();
    card.position.set(x, y, z);
    card.lookAt(WATCHED_FROM);
    const board = new THREE.Mesh(boardGeo, boardMat);
    const print = new THREE.Mesh(printGeo, printMat);
    print.position.z = 0.03;
    const fills = new THREE.InstancedMesh(fillGeo, fillMat, 2);
    fills.position.z = 0.033;
    fills.frustumCulled = false;
    fills.setColorAt(0, BAR_COLOR.similar);
    fills.setColorAt(1, BAR_COLOR.agrees);
    const scan = new THREE.Mesh(scanGeo, scanMat);
    scan.position.z = 0.05;
    scan.renderOrder = 3;
    const shade = new THREE.Mesh(kit.geo.shadow, kit.mat.shadow);
    shade.position.set(x, 0.02, z);
    shade.scale.set(1.3, 1, 0.8);
    shade.renderOrder = 1;
    card.add(board, print, fills, scan);
    group.add(card, shade);
    return { card, fills, scan, shade };
  });
  const flushed = { who: "" as Outcome | "" };

  const SPARKS = c.mobile ? 70 : 160;
  const r = rand(23);
  const velocity = new Float32Array(SPARKS * 3);
  const colors = new Float32Array(SPARKS * 3);
  const paint = [new THREE.Color(COLOR.thread), new THREE.Color(COLOR.thread), new THREE.Color(COLOR.held), new THREE.Color("#ffffff")];
  for (let i = 0; i < SPARKS; i++) {
    const a = r() * Math.PI * 2;
    const out = 0.6 + r() * 1.9;
    velocity.set([Math.cos(a) * out, 1.0 + r() * 2.1, Math.sin(a) * out], i * 3);
    paint[Math.floor(r() * paint.length)].toArray(colors, i * 3);
  }
  const sparkGeo = new THREE.BufferGeometry();
  const sparkPos = new THREE.BufferAttribute(new Float32Array(SPARKS * 3), 3);
  sparkGeo.setAttribute("position", sparkPos);
  sparkGeo.setAttribute("color", new THREE.BufferAttribute(colors, 3));
  const sparkMat = new THREE.PointsMaterial({ size: 0.2, map: kit.glowMap, vertexColors: true, transparent: true, depthWrite: false });
  const sparks = new THREE.Points(sparkGeo, sparkMat);
  sparks.frustumCulled = false;
  sparks.renderOrder = 5;
  group.add(sparks);

  const FIGURE_H = 1.25;
  const at = new THREE.Vector3();

  return {
    group,
    /** How hard ring `i` is firing: only a locked match sends the glow through the rings. */
    fire(j: number, who: Outcome, i: number) {
      if (who !== "joined") return 0;
      const t = span(j, RESOLVE.lock[0], RESOLVE.lock[1]);
      return Math.sin(Math.PI * span(t, i * 0.32, i * 0.32 + 0.34));
    },
    update(p: number, j: number, who: Outcome, figure: THREE.Vector3) {
      const on = j > 0 && p < 0.748;
      group.visible = on;
      if (!on) return;

      pool.position.set(figure.x, 0.022, figure.z);
      (pool.material as THREE.MeshBasicMaterial).opacity = 0.62 * ease(span(j, 0, 0.12)) * (1 - ease(span(j, 0.86, 1)));

      const scan = span(j, RESOLVE.scan[0], RESOLVE.scan[1]);
      const scanning = scan > 0 && scan < 1;
      disc.visible = scanning;
      if (scanning) {
        disc.position.set(figure.x, 0.03 + scan * FIGURE_H, figure.z);
        disc.scale.setScalar(1.25);
        (disc.material as THREE.MeshBasicMaterial).opacity = Math.sin(Math.PI * scan) ** 0.4;
      }

      const born = ease(span(j, 0, 0.09));
      const flush = who !== flushed.who;
      if (flush) flushed.who = who;
      const bars = JOIN_BARS[who];
      if (flush) {
        for (const k of cards) {
          k.fills.setColorAt(0, BAR_COLOR[bars[0]]);
          k.fills.setColorAt(1, BAR_COLOR[bars[1]]);
          k.fills.instanceColor!.needsUpdate = true;
        }
      }
      const name = ease(span(j, RESOLVE.nameBar[0], RESOLVE.nameBar[1]));
      const birth = ease(span(j, RESOLVE.birthBar[0], RESOLVE.birthBar[1]));
      cards.forEach((k, i) => {
        k.card.visible = k.shade.visible = born > 0;
        k.card.scale.setScalar(Math.max(0.001, born));
        k.scan.visible = scanning;
        k.scan.position.y = CARD.h * (scan - 0.5);
        [name, birth].forEach((t, b) => {
          d.position.set(-CARD.bar / 2, BAR_Y[b], 0);
          d.scale.set(Math.max(0.001, t * CARD.bar), t > 0 ? 1 + 0.5 * Math.sin(Math.PI * t) : 0.001, 1);
          d.updateMatrix();
          k.fills.setMatrixAt(b, d.matrix);
        });
        k.fills.instanceMatrix.needsUpdate = true;
        k.card.position.y = places[i][1] + 0.03 * birth * (who === "apart" ? Math.sin(j * 90) * (1 - span(j, 0.5, 0.62)) : 0);
      });
      const reddening = who === "apart" ? Math.max(0.16, Math.sin(Math.PI * span(j, RESOLVE.birthBar[0], 0.68)) * 0.85) * birth : 0;
      boardMat.color.set(COLOR.surface).lerp(BAR_COLOR.differs, reddening);

      const lock = span(j, RESOLVE.lock[0], RESOLVE.lock[1]);
      const locking = who === "joined" && lock > 0 && lock < 1;
      lockGlow.visible = locking;
      if (locking) {
        c.thread.getPointAt(ring1 + (1 - ring1) * ease(lock), at);
        lockGlow.position.copy(at);
        lockGlow.lookAt(WATCHED_FROM);
        (lockGlow.material as THREE.MeshBasicMaterial).opacity = Math.sin(Math.PI * lock) ** 0.3;
      }

      const burst = who === "joined" ? span(j, RESOLVE.burst[0], RESOLVE.burst[1]) : 0;
      const bursting = burst > 0 && burst < 1;
      sparks.visible = shock.visible = flash.visible = bursting;
      if (bursting) {
        const out = 1 - (1 - burst) * (1 - burst);
        for (let i = 0; i < SPARKS; i++) {
          sparkPos.setXYZ(
            i,
            figure.x + velocity[i * 3] * out,
            Math.max(0.03, 0.55 + velocity[i * 3 + 1] * out - 1.5 * burst * burst),
            figure.z + velocity[i * 3 + 2] * out,
          );
        }
        sparkPos.needsUpdate = true;
        sparkMat.opacity = 1 - burst ** 3;
        const wave = span(burst, 0, 0.72);
        shock.position.set(figure.x, 0.028, figure.z);
        shock.scale.setScalar(0.8 + (1 - (1 - wave) ** 2) * 7.4);
        (shock.material as THREE.MeshBasicMaterial).opacity = 1 - wave;
        flash.position.set(figure.x, 0.7, figure.z);
        flash.lookAt(WATCHED_FROM);
        flash.scale.setScalar(1.2 + burst * 2.6);
        (flash.material as THREE.MeshBasicMaterial).opacity = (1 - span(burst, 0, 0.3)) * 0.9;
      }
    },
  };
}
