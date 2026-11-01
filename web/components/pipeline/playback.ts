"use client";

import { useEffect, useReducer } from "react";

/** A replay a screen can play: where it rests, and how one step moves it. Pure, so tests drive it without a browser. */
export type Track<S> = { rest: S; advance: (s: S) => S };

export type Playback<S> = { frame: S; paused: boolean };

export type PlaybackAction<S> =
  | { type: "tick" }
  | { type: "toggle" }
  | { type: "step" }
  | { type: "seek"; frame: S };

/** Ticks move only while playing; Step pauses and moves one; Seek pauses on a chosen frame. */
export function playback<S>(track: Track<S>, state: Playback<S>, action: PlaybackAction<S>): Playback<S> {
  switch (action.type) {
    case "tick":
      return state.paused ? state : { ...state, frame: track.advance(state.frame) };
    case "toggle":
      return { ...state, paused: !state.paused };
    case "step":
      return { frame: track.advance(state.frame), paused: true };
    case "seek":
      return { frame: action.frame, paused: true };
  }
}

/** The one timer for a replay screen. It starts on the rest frame, so the first paint is a complete still. */
export function usePlayback<S>(track: Track<S>, intervalMs: number) {
  const [state, dispatch] = useReducer(
    (s: Playback<S>, a: PlaybackAction<S>) => playback(track, s, a),
    { frame: track.rest, paused: false },
  );

  useEffect(() => {
    if (state.paused) return;
    const id = window.setInterval(() => dispatch({ type: "tick" }), intervalMs);
    return () => window.clearInterval(id);
  }, [state.paused, intervalMs]);

  return {
    ...state,
    toggle: () => dispatch({ type: "toggle" }),
    step: () => dispatch({ type: "step" }),
    seek: (frame: S) => dispatch({ type: "seek", frame }),
  };
}
