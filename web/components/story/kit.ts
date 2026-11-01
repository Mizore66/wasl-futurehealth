import * as THREE from "three";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import { COLOR } from "./palette";

function canvas(size: number): [HTMLCanvasElement, CanvasRenderingContext2D] {
  const c = document.createElement("canvas");
  c.width = c.height = size;
  return [c, c.getContext("2d")!];
}

function srgb(c: HTMLCanvasElement): THREE.CanvasTexture {
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

/** Warm key from the upper left, cool fill from the lower right, a soft rim. Tinted per material by `color`. */
function matcapTexture(): THREE.CanvasTexture {
  const [c, g] = canvas(256);
  g.fillStyle = "#77706a";
  g.fillRect(0, 0, 256, 256);
  const key = g.createRadialGradient(92, 80, 4, 118, 112, 150);
  key.addColorStop(0, "#ffffff");
  key.addColorStop(0.45, "#f1ebe2");
  key.addColorStop(0.8, "#c9c0b3");
  key.addColorStop(1, "#9c9388");
  g.fillStyle = key;
  g.beginPath();
  g.arc(128, 128, 128, 0, Math.PI * 2);
  g.fill();
  const fill = g.createRadialGradient(196, 204, 0, 196, 204, 110);
  fill.addColorStop(0, "rgba(206,224,228,0.45)");
  fill.addColorStop(1, "rgba(206,224,228,0)");
  g.fillStyle = fill;
  g.fillRect(0, 0, 256, 256);
  const rim = g.createRadialGradient(128, 128, 100, 128, 128, 128);
  rim.addColorStop(0, "rgba(255,252,246,0)");
  rim.addColorStop(0.85, "rgba(255,252,246,0.35)");
  rim.addColorStop(1, "rgba(255,252,246,0)");
  g.fillStyle = rim;
  g.fillRect(0, 0, 256, 256);
  return srgb(c);
}

function shadowTexture(): THREE.CanvasTexture {
  const [c, g] = canvas(64);
  const r = g.createRadialGradient(32, 32, 0, 32, 32, 32);
  r.addColorStop(0, "rgba(40,30,20,0.34)");
  r.addColorStop(0.5, "rgba(40,30,20,0.16)");
  r.addColorStop(1, "rgba(40,30,20,0)");
  g.fillStyle = r;
  g.fillRect(0, 0, 64, 64);
  return srgb(c);
}

/** A face decal: the avatar's SVG raster, cut to a circle by the decal geometry. */
export function faceTexture(img: CanvasImageSource): THREE.CanvasTexture {
  const [c, g] = canvas(256);
  g.drawImage(img, 0, 0, 256, 256);
  const t = srgb(c);
  t.anisotropy = 4;
  return t;
}

/** One teal dot per case Wasl marked done; a wrong one would be red. Counts come from results.json. */
export function dotWallTexture(done: number, wrong: number, size: number): THREE.CanvasTexture {
  const [c, g] = canvas(size);
  g.fillStyle = COLOR.surface;
  g.fillRect(0, 0, size, size);
  const side = Math.ceil(Math.sqrt(done));
  const cell = size / (side + 2);
  for (let i = 0; i < done; i++) {
    g.fillStyle = i < wrong ? "#b3261e" : COLOR.teal;
    g.beginPath();
    g.arc(cell * (1.5 + (i % side)), cell * (1.5 + Math.floor(i / side)), cell * 0.32, 0, Math.PI * 2);
    g.fill();
  }
  return srgb(c);
}

/** A rounded coin: lathe profile with a fully rounded rim, axis along z so its faces look at ±z. */
function pebbleGeometry(radius: number, half: number, seg: number, arc: number): THREE.BufferGeometry {
  const pts: THREE.Vector2[] = [new THREE.Vector2(0, -half)];
  for (let i = 0; i <= arc; i++) {
    const a = -Math.PI / 2 + (Math.PI * i) / arc;
    pts.push(new THREE.Vector2(radius - half + Math.cos(a) * half, Math.sin(a) * half));
  }
  pts.push(new THREE.Vector2(0, half));
  return new THREE.LatheGeometry(pts, seg).rotateX(Math.PI / 2);
}

export type Kit = ReturnType<typeof createKit>;

export function createKit(mobile: boolean) {
  const matcap = matcapTexture();
  const shadowMap = shadowTexture();
  const tint = (color: string) => new THREE.MeshMatcapMaterial({ matcap, color: new THREE.Color(color) });

  const PEBBLE_R = 0.5;
  const PEBBLE_HALF = 0.17;
  const geo = {
    pebble: pebbleGeometry(PEBBLE_R, PEBBLE_HALF, mobile ? 20 : 36, mobile ? 6 : 10),
    pebbleLow: pebbleGeometry(PEBBLE_R, PEBBLE_HALF, mobile ? 14 : 24, mobile ? 4 : 6),
    decal: new THREE.CircleGeometry(PEBBLE_R - PEBBLE_HALF * 0.55, mobile ? 24 : 40),
    shadow: new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2),
  };

  const mat = {
    surface: tint(COLOR.surface),
    stone: tint(COLOR.stone),
    raised: tint(COLOR.raised),
    unknown: tint(COLOR.unknown),
    ink: tint(COLOR.ink),
    teal: Object.assign(tint(COLOR.teal), { side: THREE.DoubleSide }),
    held: Object.assign(tint(COLOR.held), { side: THREE.DoubleSide }),
    shadow: new THREE.MeshBasicMaterial({ map: shadowMap, transparent: true, depthWrite: false, fog: true }),
    floor: new THREE.MeshBasicMaterial({ color: COLOR.raised }),
  };

  /** Static geometry gathered per material and merged once: one draw call per material for all the props. */
  const statics = new Map<THREE.Material, THREE.BufferGeometry[]>();
  function addStatic(material: THREE.Material, geometry: THREE.BufferGeometry, at: THREE.Matrix4) {
    const g = geometry.index ? geometry.toNonIndexed() : geometry.clone();
    if (material !== mat.shadow) g.deleteAttribute("uv");
    g.applyMatrix4(at);
    (statics.get(material) ?? statics.set(material, []).get(material)!).push(g);
  }
  function shadowAt(x: number, z: number, size: number) {
    const m = new THREE.Matrix4().makeScale(size, 1, size).setPosition(x, 0.005, z);
    addStatic(mat.shadow, geo.shadow, m);
  }
  function mergeStatics(): THREE.Mesh[] {
    const meshes: THREE.Mesh[] = [];
    for (const [material, list] of statics) {
      const mesh = new THREE.Mesh(mergeGeometries(list)!, material);
      mesh.matrixAutoUpdate = false;
      if (material === mat.shadow) mesh.renderOrder = 1;
      meshes.push(mesh);
    }
    statics.clear();
    return meshes;
  }

  /** A pebble with a face on the front and the "?" on the back. Turning it over reveals the person. */
  function pebble(front: THREE.Texture, back: THREE.Texture, body: THREE.Material = mat.surface) {
    const group = new THREE.Group();
    group.add(new THREE.Mesh(geo.pebble, body));
    const frontMat = new THREE.MeshBasicMaterial({ map: front });
    const backMat = new THREE.MeshBasicMaterial({ map: back });
    const f = new THREE.Mesh(geo.decal, frontMat);
    f.position.z = PEBBLE_HALF + 0.004;
    const b = new THREE.Mesh(geo.decal, backMat);
    b.position.z = -PEBBLE_HALF - 0.004;
    b.rotation.y = Math.PI;
    group.add(f, b);
    return { group, frontMat, backMat };
  }

  function dispose() {
    matcap.dispose();
    shadowMap.dispose();
    for (const g of Object.values(geo)) g.dispose();
    for (const m of Object.values(mat)) m.dispose();
  }

  return { geo, mat, tint, pebble, addStatic, shadowAt, mergeStatics, dispose, PEBBLE_R, PEBBLE_HALF };
}
