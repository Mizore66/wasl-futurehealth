"use client";

import type { MotionValue } from "motion/react";
import { useEffect, useRef } from "react";
import { PortraitSvg, UnknownSvg } from "@/components/avatar/avatar";
import { portrait } from "@/components/avatar/traits";
import { FACE_SEEDS, type AnchorId, type CastKey, type FaceKey, type StoryFigures } from "./script";
import { createStage, type Stage, type StageStats } from "./stage";

export type Anchors = Partial<Record<AnchorId, { x: number; y: number }>>;

declare global {
  interface Window {
    /** Read by the perf probe and the tests. */
    __wasl3d?: () => StageStats;
  }
}

/** The app's own avatar SVG, rasterised so the scene shows the same face every other screen shows. */
async function raster(svg: SVGSVGElement): Promise<HTMLImageElement> {
  const copy = svg.cloneNode(true) as SVGSVGElement;
  copy.setAttribute("width", "256");
  copy.setAttribute("height", "256");
  copy.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  const img = new Image();
  img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(new XMLSerializer().serializeToString(copy))}`;
  await img.decode();
  return img;
}

export default function StageCanvas({
  progress,
  person,
  figures,
  mobile,
  onSlow,
  onAnchors,
}: {
  progress: MotionValue<number>;
  figures: StoryFigures;
  person: CastKey;
  mobile: boolean;
  onSlow: () => void;
  onAnchors: (a: Anchors) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const facesRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<Stage | null>(null);
  const personRef = useRef(person);
  const slowRef = useRef(onSlow);
  const anchorsRef = useRef(onAnchors);
  useEffect(() => {
    slowRef.current = onSlow;
    anchorsRef.current = onAnchors;
  }, [onSlow, onAnchors]);

  useEffect(() => {
    let stage: Stage | null = null;
    let cancelled = false;
    const box = facesRef.current!;
    const svg = (sel: string) => box.querySelector<SVGSVGElement>(`${sel} svg`)!;

    (async () => {
      const keys = Object.keys(FACE_SEEDS) as FaceKey[];
      const imgs = await Promise.all([...keys.map((k) => raster(svg(`[data-face="${k}"]`))), raster(svg('[data-face="unknown"]'))]);
      if (cancelled) return;
      stage = createStage({
        canvas: canvasRef.current!,
        faces: Object.fromEntries(keys.map((k, i) => [k, imgs[i]])) as Record<FaceKey, HTMLImageElement>,
        unknown: imgs[keys.length],
        figures,
        mobile,
        onSlow: () => slowRef.current(),
        onAnchors: (a) => anchorsRef.current(a),
      });
      stageRef.current = stage;
      if (personRef.current !== "fatima") stage.setPerson(personRef.current);
      stage.setProgress(progress.get());
      window.__wasl3d = stage.stats;
    })();

    const unsub = progress.on("change", (p) => stage?.setProgress(p));
    const onResize = () => stage?.resize();
    window.addEventListener("resize", onResize);
    return () => {
      cancelled = true;
      unsub();
      window.removeEventListener("resize", onResize);
      stage?.dispose();
      stageRef.current = null;
      delete window.__wasl3d;
    };
    // figures come from the server page and never change while it is open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [progress, mobile]);

  useEffect(() => {
    if (personRef.current === person) return;
    personRef.current = person;
    stageRef.current?.setPerson(person);
  }, [person]);

  return (
    <>
      <canvas ref={canvasRef} aria-hidden className="block size-full" />
      <div ref={facesRef} hidden>
        {(Object.keys(FACE_SEEDS) as FaceKey[]).map((k) => (
          <span key={k} data-face={k}>
            <PortraitSvg f={portrait(FACE_SEEDS[k])} />
          </span>
        ))}
        <span data-face="unknown">
          <UnknownSvg />
        </span>
      </div>
    </>
  );
}
