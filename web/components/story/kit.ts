import * as THREE from "three";
import { mergeGeometries } from "three/addons/utils/BufferGeometryUtils.js";
import { CYLINDER, figureGeo, type FigureGeo } from "./figure-geometry";
import { COLOR } from "./palette";

function canvas(width: number, height = width): [HTMLCanvasElement, CanvasRenderingContext2D] {
  const c = document.createElement("canvas");
  c.width = width;
  c.height = height;
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
  g.fillStyle = "#544d47";
  g.fillRect(0, 0, 256, 256);
  const key = g.createRadialGradient(92, 76, 4, 118, 112, 150);
  key.addColorStop(0, "#ffffff");
  key.addColorStop(0.42, "#f2ebe0");
  key.addColorStop(0.76, "#b9ae9f");
  key.addColorStop(1, "#6f665c");
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
  r.addColorStop(0, "rgba(38,26,16,0.58)");
  r.addColorStop(0.5, "rgba(38,26,16,0.3)");
  r.addColorStop(1, "rgba(38,26,16,0)");
  g.fillStyle = r;
  g.fillRect(0, 0, 64, 64);
  return srgb(c);
}

/** A soft round spot, white at the heart and clear at the edge: every glow in the story tints this. */
function glowTexture(): THREE.CanvasTexture {
  const [c, g] = canvas(128);
  const r = g.createRadialGradient(64, 64, 0, 64, 64, 64);
  r.addColorStop(0, "rgba(255,255,255,1)");
  r.addColorStop(0.28, "rgba(255,255,255,0.72)");
  r.addColorStop(0.6, "rgba(255,255,255,0.18)");
  r.addColorStop(1, "rgba(255,255,255,0)");
  g.fillStyle = r;
  g.fillRect(0, 0, 128, 128);
  return srgb(c);
}

type Draw = (g: CanvasRenderingContext2D) => void;

function dot(g: CanvasRenderingContext2D, x: number, y: number, r: number) {
  g.beginPath();
  g.arc(x, y, r, 0, Math.PI * 2);
  g.fill();
}

/** The avatar's eyes, in its 64-unit space, thickened to survive at road size. */
const EYES: readonly Draw[] = [
  (g) => {
    dot(g, 27.4, 31, 2.3);
    dot(g, 36.6, 31, 2.3);
  },
  (g) => g.stroke(new Path2D("M25 31.6Q27.4 29 29.8 31.6M34.2 31.6Q36.6 29 39 31.6")),
  (g) => g.stroke(new Path2D("M25.4 31H29.4M34.6 31H38.6")),
  (g) => {
    g.stroke(new Path2D("M30.7 31H33.3M30.7 31A3.3 3.3 0 1 0 24.1 31A3.3 3.3 0 1 0 30.7 31M39.9 31A3.3 3.3 0 1 0 33.3 31A3.3 3.3 0 1 0 39.9 31"));
    dot(g, 27.4, 31, 1.1);
    dot(g, 36.6, 31, 1.1);
  },
];
/** Small and closed, every one, so nobody on the road looks startled. */
const MOUTHS: readonly Draw[] = [
  (g) => g.stroke(new Path2D("M29.8 38.6Q32 40.2 34.2 38.6")),
  (g) => g.stroke(new Path2D("M30.2 39.2H33.8")),
  (g) => g.stroke(new Path2D("M30 39.4Q32.4 39.9 34 38.6")),
];

/** The part of the avatar's 64-unit space the face patch on a head shows. */
const PATCH = { x: 13, y: 25, w: 38, h: 20 };

function printTexture(draw: Draw): THREE.CanvasTexture {
  const [c, g] = canvas(256, 128);
  g.fillStyle = g.strokeStyle = "#fff";
  draw(g);
  const t = srgb(c);
  t.anisotropy = 4;
  return t;
}

/**
 * What setKnown animates. Colour has risen to figure-local height `level`; above it a part is the
 * unknown grey, with a bright line at the front while `edge` is above zero.
 */
export interface Flood {
  level: { value: number };
  edge: { value: number };
}

export type Kit = ReturnType<typeof createKit>;

export function createKit(mobile: boolean) {
  const matcap = matcapTexture();
  const shadowMap = shadowTexture();
  const glowMap = glowTexture();
  const tint = (color: string) => new THREE.MeshMatcapMaterial({ matcap, color: new THREE.Color(color) });

  const figure: FigureGeo = figureGeo(CYLINDER, mobile ? 14 : 26);
  const geo = {
    shadow: new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2),
  };

  const prints = new Map<string, THREE.CanvasTexture>();
  const printed = (key: string, draw: Draw) => prints.get(key) ?? prints.set(key, printTexture(draw)).get(key)!;
  /** One texture per eyes and mouth pair, in white; a figure's face material tints it. */
  const faceMap = (eyes: number, mouth: number) =>
    printed(`${eyes}-${mouth}`, (g) => {
      g.setTransform(256 / PATCH.w, 0, 0, 128 / PATCH.h, (-PATCH.x * 256) / PATCH.w, (-PATCH.y * 128) / PATCH.h);
      g.lineWidth = 1.9;
      g.lineCap = "round";
      EYES[eyes](g);
      g.lineWidth = 1.5;
      MOUTHS[mouth](g);
    });
  const questionMap = () =>
    printed("?", (g) => {
      g.font = "800 118px system-ui, sans-serif";
      g.textAlign = "center";
      g.textBaseline = "middle";
      g.fillText("?", 128, 70);
    });

  function floodMaterial(flood: Flood) {
    const m = new THREE.MeshMatcapMaterial({ matcap });
    m.onBeforeCompile = (s) => {
      s.uniforms.uLevel = flood.level;
      s.uniforms.uEdge = flood.edge;
      s.uniforms.uGrey = { value: new THREE.Color(COLOR.unknown) };
      s.uniforms.uEdgeColor = { value: new THREE.Color(COLOR.thread) };
      s.vertexShader = s.vertexShader
        .replace("#include <common>", "#include <common>\nvarying float vFigureY;")
        .replace("#include <begin_vertex>", "#include <begin_vertex>\nvFigureY = position.y;");
      s.fragmentShader = s.fragmentShader
        .replace("#include <common>", "#include <common>\nvarying float vFigureY;\nuniform float uLevel;\nuniform float uEdge;\nuniform vec3 uGrey;\nuniform vec3 uEdgeColor;")
        .replace("#include <color_fragment>", "#include <color_fragment>\ndiffuseColor.rgb = mix(diffuseColor.rgb, uGrey, smoothstep(uLevel - 0.012, uLevel + 0.012, vFigureY));")
        .replace(
          "#include <opaque_fragment>",
          "outgoingLight = mix(outgoingLight, uEdgeColor * 1.25, uEdge * (1.0 - smoothstep(0.0, 0.075, abs(vFigureY - uLevel))));\n#include <opaque_fragment>",
        );
    };
    m.customProgramCacheKey = () => "figure-flood";
    return m;
  }

  const mat = {
    surface: tint(COLOR.surface),
    stone: tint(COLOR.stone),
    ink: tint(COLOR.ink),
    thread: new THREE.MeshBasicMaterial({ color: COLOR.thread, side: THREE.DoubleSide }),
    held: new THREE.MeshBasicMaterial({ color: COLOR.held, side: THREE.DoubleSide }),
    /** White, so instance colours show as they are. */
    crowd: new THREE.MeshMatcapMaterial({ matcap }),
    shadow: new THREE.MeshBasicMaterial({ map: shadowMap, transparent: true, depthWrite: false, fog: true }),
    floor: new THREE.MeshBasicMaterial({ color: COLOR.floor }),
    road: new THREE.MeshBasicMaterial({ vertexColors: true }),
    city: new THREE.MeshMatcapMaterial({ matcap, vertexColors: true }),
    /** Same look as the city; separate buckets so each building merges and culls on its own. */
    clinic: new THREE.MeshMatcapMaterial({ matcap, vertexColors: true }),
    hospital: new THREE.MeshMatcapMaterial({ matcap, vertexColors: true }),
    grey: tint(COLOR.grey),
  };

  /** Static geometry gathered per material and merged once: one draw call per material for all the props. */
  const statics = new Map<THREE.Material, THREE.BufferGeometry[]>();
  function addStatic(material: THREE.Material, geometry: THREE.BufferGeometry, at: THREE.Matrix4) {
    const g = geometry.index ? geometry.toNonIndexed() : geometry.clone();
    if (material !== mat.shadow) g.deleteAttribute("uv");
    g.applyMatrix4(at);
    (statics.get(material) ?? statics.set(material, []).get(material)!).push(g);
  }
  function shadowAt(x: number, z: number, w: number, d = w) {
    const m = new THREE.Matrix4().makeScale(w, 1, d).setPosition(x, 0.018, z);
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

  function dispose() {
    matcap.dispose();
    shadowMap.dispose();
    glowMap.dispose();
    for (const t of prints.values()) t.dispose();
    for (const g of [...Object.values(geo), figure.body, figure.head, figure.face, figure.bodyLow, figure.headLow, ...Object.values(figure.hair)]) g?.dispose();
    for (const m of Object.values(mat)) m.dispose();
  }

  return { geo, mat, tint, glowMap, figure, faceMap, questionMap, floodMaterial, addStatic, shadowAt, mergeStatics, dispose };
}
