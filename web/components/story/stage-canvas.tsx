"use client";

import type { MotionValue } from "motion/react";
import { useEffect, useRef } from "react";
import type { AnchorId, CastKey, StoryFigures } from "./script";
import { createStage, type Stage, type StageStats } from "./stage";

export type Anchors = Partial<Record<AnchorId, { x: number; y: number }>>;

declare global {
  interface Window {
    /** Read by the perf probe and the tests. */
    __wasl3d?: () => StageStats;
  }
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
  const stageRef = useRef<Stage | null>(null);
  const personRef = useRef(person);
  const slowRef = useRef(onSlow);
  const anchorsRef = useRef(onAnchors);
  useEffect(() => {
    slowRef.current = onSlow;
    anchorsRef.current = onAnchors;
  }, [onSlow, onAnchors]);

  useEffect(() => {
    const stage = createStage({
      canvas: canvasRef.current!,
      figures,
      mobile,
      onSlow: () => slowRef.current(),
      onAnchors: (a) => anchorsRef.current(a),
    });
    stageRef.current = stage;
    if (personRef.current !== "fatima") stage.setPerson(personRef.current);
    stage.setProgress(progress.get());
    window.__wasl3d = stage.stats;

    const unsub = progress.on("change", (p) => stage.setProgress(p));
    const onResize = () => stage.resize();
    window.addEventListener("resize", onResize);
    return () => {
      unsub();
      window.removeEventListener("resize", onResize);
      stage.dispose();
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

  // A fresh canvas per layout: antialiasing is fixed when a canvas's context is made, and phones turn it off.
  return <canvas key={mobile ? "phone" : "desktop"} ref={canvasRef} aria-hidden className="block size-full" />;
}
