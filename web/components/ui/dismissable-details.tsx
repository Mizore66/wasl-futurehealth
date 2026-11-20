"use client";

import { useEffect, useRef, type ComponentProps } from "react";

/** A `<details>` that folds itself on a press or focus move outside it, or Escape, like a menu. */
export function DismissableDetails(props: ComponentProps<"details">) {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const outside = (t: EventTarget | null) => t instanceof Node && !el.contains(t);
    // A click, not a press: a phone scroll that starts outside must not fold it mid-gesture.
    const onClick = (e: MouseEvent) => {
      if (el.open && outside(e.target)) el.open = false;
    };
    const onFocus = (e: FocusEvent) => {
      if (el.open && outside(e.relatedTarget)) el.open = false;
    };
    // On the document: Safari leaves focus on the body after a mouse open.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || !el.open || e.isComposing || e.defaultPrevented) return;
      el.open = false;
      el.querySelector("summary")?.focus();
    };
    document.addEventListener("click", onClick);
    el.addEventListener("focusout", onFocus);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("click", onClick);
      el.removeEventListener("focusout", onFocus);
      document.removeEventListener("keydown", onKey);
    };
  }, []);
  return <details ref={ref} {...props} />;
}
