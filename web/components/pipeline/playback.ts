"use client";

import { useEffect, useReducer, useSyncExternalStore } from "react";

/** A replay a screen can play: where it rests, and how one step moves it. Pure, so tests drive it without a browser. */
export type Track<S> = { rest: S; advance: (s: S) => S };

export type Playback<S> = { frame: S; paused: boolean };

export type PlaybackAction<S> =
  | { type: "tick" }
  | { type: "toggle" }
  | { type: "pause" }
  | { type: "step" }
  | { type: "seek"; frame: S };

/** Ticks move only while playing; Step pauses and moves one; Seek pauses on a chosen frame. */
export function playback<S>(track: Track<S>, state: Playback<S>, action: PlaybackAction<S>): Playback<S> {
  switch (action.type) {
    case "tick":
      return state.paused ? state : { ...state, frame: track.advance(state.frame) };
    case "pause":
      return state.paused ? state : { ...state, paused: true };
    case "toggle":
      return { ...state, paused: !state.paused };
    case "step":
      return { frame: track.advance(state.frame), paused: true };
    case "seek":
      return { frame: action.frame, paused: true };
  }
}

const REDUCE = "(prefers-reduced-motion: reduce)";

/** Whether the viewer asked for reduced motion. False on the server and for the first client render, so hydration matches. */
export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    (notify) => {
      const m = window.matchMedia(REDUCE);
      m.addEventListener("change", notify);
      return () => m.removeEventListener("change", notify);
    },
    () => window.matchMedia(REDUCE).matches,
    () => false,
  );
}

/**
 * The one timer for a replay screen. It starts on the rest frame, so the first paint is a complete still.
 * With reduced motion it starts paused on the rest frame (the settled state): nothing plays until the viewer presses Play or Step.
 * `hold` stops the timer without touching `paused`, so whatever held it (an open profile) resumes only a playing replay.
 */
export function usePlayback<S>(track: Track<S>, intervalMs: number, hold = false) {
  const [state, dispatch] = useReducer(
    (s: Playback<S>, a: PlaybackAction<S>) => playback(track, s, a),
    { frame: track.rest, paused: false },
  );

  const reduce = usePrefersReducedMotion();
  useEffect(() => {
    if (reduce) dispatch({ type: "pause" });
  }, [reduce]);

  useEffect(() => {
    if (state.paused || hold) return;
    const id = window.setInterval(() => dispatch({ type: "tick" }), intervalMs);
    return () => window.clearInterval(id);
  }, [state.paused, hold, intervalMs]);

  return {
    ...state,
    reduceMotion: reduce,
    toggle: () => dispatch({ type: "toggle" }),
    step: () => dispatch({ type: "step" }),
    seek: (frame: S) => dispatch({ type: "seek", frame }),
  };
}
