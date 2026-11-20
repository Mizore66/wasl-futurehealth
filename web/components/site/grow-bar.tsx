"use client";

import { motion } from "motion/react";
import { cn } from "@/lib/utils";

/** A bar that grows from the left once it scrolls into view. Reduced motion is handled by the root MotionConfig. */
export function GrowBar({ pct, className, delay = 0 }: { pct: number; className?: string; delay?: number }) {
  return (
    <motion.div
      className={cn("h-full origin-left rounded-xs", className)}
      style={{ width: `${Math.max(0, Math.min(100, pct))}%` }}
      initial={{ scaleX: 0 }}
      whileInView={{ scaleX: 1 }}
      viewport={{ once: true, margin: "0px 0px -10% 0px" }}
      transition={{ duration: 0.9, delay, ease: [0.22, 1, 0.36, 1] }}
    />
  );
}
