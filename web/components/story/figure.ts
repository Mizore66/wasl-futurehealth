import * as THREE from "three";
import type { FigureGeo } from "./figure-geometry";
import type { Kit } from "./kit";
import type { FigureLook } from "./look";
import { COLOR } from "./palette";
import { clamp01, ease, span } from "./script";

/** How much bigger a figure stands than its modelling units. */
export const FIGURE_SCALE = 1.15;

export interface Figure {
  group: THREE.Group;
  /** 0 is a grey "?" figure, 1 is the person. Continuous, so a scroll position can hold it anywhere. */
  setKnown(amount: number): void;
  /** The chips swap who she is. */
  setLook(look: FigureLook): void;
}

/** Hair settles from this high above the head while the colour reaches it. */
const SETTLE = 0.34;

/** Dark print on light skin. On the deepest tones a dark print vanishes at road size, so it goes a soft warm light. */
function inkFor(skin: string): string {
  return new THREE.Color(skin).getHSL({ h: 0, s: 0, l: 0 }).l < 0.3 ? "#dcbfa3" : "#1b2224";
}

export function createFigure(kit: Kit, look: FigureLook, geo: FigureGeo = kit.figure): Figure {
  const group = new THREE.Group();
  const model = new THREE.Group();
  model.scale.setScalar(FIGURE_SCALE);
  group.add(model);

  const top = geo.height + 0.2;
  const flood = { level: { value: top }, edge: { value: 0 } };
  const bodyMat = kit.floodMaterial(flood);
  const headMat = kit.floodMaterial(flood);
  const hairMat = new THREE.MeshMatcapMaterial({ matcap: kit.mat.crowd.matcap, vertexColors: true, side: THREE.DoubleSide });
  const faceMat = new THREE.MeshBasicMaterial({ transparent: true, depthWrite: false });
  const askMat = new THREE.MeshBasicMaterial({ map: kit.questionMap(), color: COLOR.unknownInk, transparent: true, depthWrite: false, opacity: 0 });

  const body = new THREE.Mesh(geo.body, bodyMat);
  const head = new THREE.Mesh(geo.head, headMat);
  const hair = new THREE.Mesh(geo.hair.short ?? undefined, hairMat);
  const face = new THREE.Mesh(geo.face, faceMat);
  const ask = new THREE.Mesh(geo.face, askMat);
  hair.name = "hair";
  face.name = "face";
  ask.name = "ask";
  body.name = "body";
  head.name = "head";
  ask.visible = false;
  model.add(body, head, hair, face, ask);

  let shown = -1;
  let hasHair = true;

  function setLook(l: FigureLook) {
    bodyMat.color.set(l.cloth);
    headMat.color.set(l.skin);
    hairMat.color.set(l.topColor);
    faceMat.color.set(inkFor(l.skin));
    faceMat.map = kit.faceMap(l.eyes, l.mouth);
    faceMat.needsUpdate = true;
    const piece = geo.hair[l.top];
    hasHair = piece !== null;
    if (piece) hair.geometry = piece;
    const k = shown;
    shown = -1;
    setKnown(k < 0 ? 1 : k);
  }

  function setKnown(amount: number) {
    const k = clamp01(amount);
    if (k === shown) return;
    shown = k;
    flood.level.value = k === 0 ? -0.1 : k === 1 ? top : k * top;
    flood.edge.value = k > 0 && k < 1 ? Math.sin(Math.PI * k) : 0;

    const settle = ease(span(k, 0.66, 0.96));
    hair.visible = hasHair && settle > 0;
    hair.position.y = (1 - settle) * SETTLE;
    hair.scale.setScalar(0.86 + 0.14 * settle);

    const seen = ease(span(k, 0.62, 0.82));
    const asked = 1 - ease(span(k, 0.38, 0.62));
    face.visible = seen > 0;
    faceMat.opacity = seen;
    ask.visible = asked > 0;
    askMat.opacity = asked;
  }

  setLook(look);
  return { group, setKnown, setLook };
}
