"use client";

import { motion } from "motion/react";

/**
 * A restrained scroll entrance: the block rises 24px once as it enters view. Transform only, so
 * text is never faded below contrast and a page without script still reads. Reduced motion is
 * handled by the root MotionConfig.
 */
export function Reveal({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <motion.div
      className={className}
      initial={{ y: 24 }}
      whileInView={{ y: 0 }}
      viewport={{ once: true, margin: "0px 0px -10% 0px" }}
      transition={{ duration: 0.5, ease: [0.22, 1, 0.36, 1] }}
    >
      {children}
    </motion.div>
  );
}
