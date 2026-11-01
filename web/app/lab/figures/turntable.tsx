"use client";

import { useEffect, useRef } from "react";
import * as THREE from "three";
import { createFigure, type Figure } from "@/components/story/figure";
import { figureGeo, type FigureGeo } from "@/components/story/figure-geometry";
import { createKit, type Kit } from "@/components/story/kit";
import { lookOf, type FigureLook } from "@/components/story/look";
import { COLOR } from "@/components/story/palette";
import { CAST } from "@/components/story/script";
import { SILHOUETTES } from "./silhouettes";

declare global {
  interface Window {
    /** Read by the e2e test: what a real figure shows for a seed and a known amount. */
    __figureProbe?: (key: string, sex: "female" | "male" | undefined, known: number) => FigureProbe;
  }
}

export interface FigureProbe {
  look: FigureLook;
  body: string;
  head: string;
  hairVisible: boolean;
  faceVisible: boolean;
  askVisible: boolean;
}

const RANDOM: { key: string; sex?: "female" | "male" }[] = [
  { key: "lab-1", sex: "female" },
  { key: "lab-2", sex: "male" },
  { key: "lab-3" },
  { key: "lab-4", sex: "female" },
  { key: "lab-5", sex: "male" },
  { key: "lab-8", sex: "female" },
];

const WIDTH = 1180;
const CLOSE_H = 300;
const ROAD_H = 90;

function row(kit: Kit, geo: FigureGeo, scene: THREE.Scene): Figure[] {
  const figures: Figure[] = [];
  const people = [CAST.fatima.seed, CAST.khalid.seed, null, ...RANDOM];
  people.forEach((seed, i) => {
    const f = createFigure(kit, lookOf(seed ?? CAST.fatima.seed), geo);
    if (!seed) f.setKnown(0);
    f.group.position.x = (i - (people.length - 1) / 2) * 1.45;
    const shadow = new THREE.Mesh(kit.geo.shadow, kit.mat.shadow);
    shadow.position.set(f.group.position.x, 0.006, 0);
    shadow.scale.setScalar(1.5);
    shadow.renderOrder = 1;
    scene.add(f.group, shadow);
    figures.push(f);
  });
  return figures;
}

function Silhouette({ index }: { index: number }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const s = SILHOUETTES[index];
  useEffect(() => {
    const canvas = ref.current!;
    const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
    renderer.setPixelRatio(1);
    renderer.setSize(WIDTH, CLOSE_H + ROAD_H, false);
    renderer.setScissorTest(true);
    const kit = createKit(false);
    const geo = figureGeo(s.shape, 26);
    const scene = new THREE.Scene();
    scene.background = new THREE.Color(COLOR.floor);
    const road = new THREE.Mesh(new THREE.PlaneGeometry(40, 1.8).rotateX(-Math.PI / 2), new THREE.MeshBasicMaterial({ color: COLOR.road }));
    scene.add(road);
    const figures = row(kit, geo, scene);

    const close = new THREE.PerspectiveCamera(24, WIDTH / CLOSE_H, 0.1, 100);
    close.position.set(0, 1.9, 8.6);
    close.lookAt(0, 0.72, 0);
    const far = new THREE.PerspectiveCamera(18, WIDTH / ROAD_H, 0.1, 200);
    far.position.set(0, 2.1, 9.9);
    far.lookAt(0, 0.62, 0);

    const probe = createFigure(kit, lookOf(CAST.fatima.seed), geo);
    if (index === 0) {
      window.__figureProbe = (key, sex, known) => {
        const look = lookOf({ key, sex });
        probe.setLook(look);
        probe.setKnown(known);
        const part = <T extends THREE.Object3D>(name: string) => probe.group.getObjectByName(name) as T;
        const color = (name: string) => `#${(part<THREE.Mesh>(name).material as THREE.MeshMatcapMaterial).color.getHexString()}`;
        return {
          look,
          body: color("body"),
          head: color("head"),
          hairVisible: part("hair").visible,
          faceVisible: part("face").visible,
          askVisible: part("ask").visible,
        };
      };
    }

    const still = new URLSearchParams(location.search).has("still");
    let raf = 0;
    const draw = (now: number) => {
      raf = requestAnimationFrame(draw);
      figures.forEach((f, i) => (f.group.rotation.y = still ? 0.25 : Math.sin(now / 1400 + i * 0.5) * 0.9));
      renderer.setViewport(0, ROAD_H, WIDTH, CLOSE_H);
      renderer.setScissor(0, ROAD_H, WIDTH, CLOSE_H);
      renderer.render(scene, close);
      renderer.setViewport(0, 0, WIDTH, ROAD_H);
      renderer.setScissor(0, 0, WIDTH, ROAD_H);
      renderer.render(scene, far);
    };
    raf = requestAnimationFrame(draw);
    return () => {
      cancelAnimationFrame(raf);
      if (index === 0) delete window.__figureProbe;
      kit.dispose();
      renderer.dispose();
    };
  }, [index, s]);
  return (
    <section className="mt-6">
      <h2 className="m-0 text-[18px] font-semibold">
        {s.id} · {s.name}
      </h2>
      <canvas ref={ref} width={WIDTH} height={CLOSE_H + ROAD_H} style={{ width: WIDTH, height: CLOSE_H + ROAD_H }} className="mt-2 block max-w-full" />
    </section>
  );
}

export function Turntable() {
  return SILHOUETTES.map((_, i) => <Silhouette key={i} index={i} />);
}
