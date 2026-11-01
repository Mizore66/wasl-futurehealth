import * as THREE from "three";
import { PLACES, local } from "./beats";
import { dotWallTexture } from "./kit";
import { COLOR } from "./palette";
import { ease, m4, span, v3, type Part, type World } from "./parts";

/** Deterministic scatter, so the room is the same on every visit. */
function rand(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s / 2 ** 32;
  };
}

/**
 * The crowd: pairs of pebbles, one person's two records each. At the start about a third are joined;
 * at the close most are, and the few held open are amber with two people beside them.
 */
export function crowd(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const pairs = w.mobile ? 24 : 60;
  const r = rand(7);
  const spots: { x: number; z: number; yaw: number }[] = [];
  while (spots.length < pairs) {
    const x = (r() - 0.5) * 28;
    const z = -22 + r() * 21;
    if (Math.abs(x - 0.6) < 3 && z > -4) continue;
    if (spots.some((s) => Math.hypot(s.x - x, s.z - z) < 2.4)) continue;
    spots.push({ x, z, yaw: (r() - 0.5) * 0.8 });
  }
  const joinedAtStart = (i: number) => i % 3 === 0;
  const heldAtEnd = (i: number) => i % 9 === 4;

  // Two records per pair, then two reviewers per pair that only held pairs show at the close.
  const pebbles = new THREE.InstancedMesh(kit.geo.pebbleLow, kit.mat.surface, pairs * 4);
  const threads = new THREE.InstancedMesh(new THREE.BoxGeometry(1, 0.07, 0.07), kit.mat.surface, pairs);
  const dummy = new THREE.Object3D();
  const surface = new THREE.Color(COLOR.surface);
  const unknown = new THREE.Color(COLOR.unknown);
  const teal = new THREE.Color(COLOR.teal);
  const held = new THREE.Color(COLOR.held);
  const zero = new THREE.Matrix4().makeScale(0, 0, 0);

  function place(i: number, side: number, s: { x: number; z: number; yaw: number }, scale = 1, forward = 0) {
    dummy.position.set(s.x + Math.cos(s.yaw) * side * 0.62, 0.5 * scale + 0.01, s.z - Math.sin(s.yaw) * side * 0.62 + forward);
    dummy.rotation.set(0, s.yaw, 0);
    dummy.scale.setScalar(scale);
    dummy.updateMatrix();
    pebbles.setMatrixAt(i, dummy.matrix);
  }

  function setEnd(end: boolean) {
    spots.forEach((s, i) => {
      const joined = end ? !heldAtEnd(i) : joinedAtStart(i);
      const holding = end && heldAtEnd(i);
      place(i * 2, -1, s);
      place(i * 2 + 1, 1, s);
      pebbles.setColorAt(i * 2, surface);
      pebbles.setColorAt(i * 2 + 1, joined ? surface : unknown);
      pebbles.setColorAt(pairs * 2 + i * 2, unknown);
      pebbles.setColorAt(pairs * 2 + i * 2 + 1, unknown);
      if (holding) {
        place(pairs * 2 + i * 2, -1.1, s, 0.55, 1);
        place(pairs * 2 + i * 2 + 1, 1.1, s, 0.55, 1);
      } else {
        pebbles.setMatrixAt(pairs * 2 + i * 2, zero);
        pebbles.setMatrixAt(pairs * 2 + i * 2 + 1, zero);
      }
      threads.setColorAt(i, joined ? teal : held);
      if (joined || holding) {
        dummy.position.set(s.x, 0.5, s.z);
        dummy.rotation.set(0, s.yaw, 0);
        dummy.scale.set(0.62, 1, 1);
        dummy.updateMatrix();
        threads.setMatrixAt(i, dummy.matrix);
      } else threads.setMatrixAt(i, zero);
    });
    pebbles.instanceMatrix.needsUpdate = true;
    pebbles.instanceColor!.needsUpdate = true;
    threads.instanceMatrix.needsUpdate = true;
    threads.instanceColor!.needsUpdate = true;
    shownEnd = end;
  }
  let shownEnd = false;
  setEnd(false);
  for (const s of spots) kit.shadowAt(s.x, s.z, 2.2);
  group.add(pebbles, threads);

  // Fatima and her unlinked twin: the pair the camera pushes toward.
  const her = kit.pebble(w.faces.fatima, w.unknown);
  her.group.position.set(0, 0.51, 0);
  const twin = kit.pebble(w.faces.fatima, w.unknown);
  twin.group.position.set(1.3, 0.51, 0);
  const tie = new THREE.Mesh(new THREE.BoxGeometry(0.64, 0.07, 0.07), kit.mat.teal);
  tie.position.set(0.65, 0.51, 0);
  kit.shadowAt(0, 0, 1.6);
  kit.shadowAt(1.3, 0, 1.6);
  group.add(her.group, twin.group, tie);

  return {
    group,
    window: [0, 1],
    update(p) {
      group.visible = p < 0.12 || p > 0.86;
      if (!group.visible) return false;
      const end = p > 0.86;
      if (end !== shownEnd) setEnd(end);
      twin.group.rotation.y = end ? 0 : Math.PI;
      tie.visible = end;
      if (end) w.fog.far = THREE.MathUtils.lerp(40, 75, ease(local(p, "start")));
      else if (p < 0.1) w.fog.far = THREE.MathUtils.lerp(75, 26, ease(local(p, "lost")));
      return false;
    },
  };
}

/** Beat 1, second half: five people on a ledge; an ID-only check finds too few of them. */
export function ledge(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const L = v3(PLACES.ledge);
  kit.addStatic(kit.mat.stone, new THREE.BoxGeometry(10.4, 0.5, 1.6), m4().setPosition(L.x, 0.25, L.z));
  kit.shadowAt(L.x - 3, L.z, 4);
  kit.shadowAt(L.x + 3, L.z, 4);
  const found = 5 - w.figures.missedOfFive;
  const threads: THREE.Mesh[] = [];
  const threadGeo = new THREE.BoxGeometry(0.62, 0.06, 0.06);
  const grey = kit.tint("#9aa3a2");
  for (let i = 0; i < 5; i++) {
    const x = L.x + (i - 2) * 1.9;
    const a = new THREE.Mesh(kit.geo.pebble, kit.mat.surface);
    a.scale.setScalar(0.75);
    a.position.set(x - 0.5, 0.5 + 0.375, L.z);
    const b = i < found ? new THREE.Mesh(kit.geo.pebble, kit.mat.surface) : kit.pebble(w.unknown, w.unknown, kit.mat.unknown).group;
    b.scale.setScalar(0.75);
    b.position.set(x + 0.5, 0.5 + 0.375, L.z);
    group.add(a, b);
    if (i < found) {
      const t = new THREE.Mesh(threadGeo, grey);
      t.position.set(x, 0.875, L.z);
      threads.push(t);
      group.add(t);
    }
  }
  const disc = new THREE.Mesh(new THREE.CylinderGeometry(0.34, 0.34, 0.08, 32), grey);
  group.add(disc);
  return {
    group,
    window: [0.16, 0.27],
    update(p) {
      const s = ease(span(local(p, "unknown"), 0.55, 0.95));
      const dx = L.x - 5.4 + 10.8 * s;
      disc.position.set(dx, 0.54, L.z + 0.6);
      for (const t of threads) t.visible = t.position.x < dx;
      return false;
    },
  };
}

/** Beat 2: the wait, drawn to scale. A notch at the standard; the pebble rolls past it to the end. */
export function beam(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const B = v3(PLACES.beam);
  const len = 14;
  const x0 = B.x - len / 2;
  kit.addStatic(kit.mat.stone, new THREE.BoxGeometry(len, 0.36, 0.9), m4().setPosition(B.x, 0.18, B.z));
  const notch = x0 + (len * w.figures.standardDays) / w.figures.waitDays;
  kit.addStatic(kit.mat.ink, new THREE.CylinderGeometry(0.04, 0.04, 1.6, 12), m4().setPosition(notch, 0.8, B.z + 0.47));
  kit.shadowAt(B.x - 4, B.z, 3);
  kit.shadowAt(B.x + 4, B.z, 3);
  const her = kit.pebble(w.faces.fatima, w.unknown);
  group.add(her.group);
  return {
    group,
    window: [0.22, 0.35],
    update(p) {
      const s = ease(span(local(p, "wait"), 0.05, 0.85));
      const x = THREE.MathUtils.lerp(x0 + 0.5, x0 + len - 0.5, s);
      her.group.position.set(x, 0.36 + 0.5, B.z);
      her.group.rotation.z = -s * Math.PI * 8;
      return false;
    },
  };
}

/** Beat 4: a report between two readers, rules (faceted) and AI (smooth); then the wall of closed cases. */
export function readers(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const R = v3(PLACES.readers);
  const Y = 1.45;
  const rulesMat = Object.assign(kit.tint(COLOR.stone), { flatShading: true });
  const aiMat = kit.tint(COLOR.stone);
  const rules = new THREE.Mesh(new THREE.TorusGeometry(1.05, 0.16, 4, 6), rulesMat);
  rules.position.set(R.x - 2, Y, R.z);
  rules.rotation.y = 0.35;
  const ai = new THREE.Mesh(new THREE.TorusGeometry(1.05, 0.14, 16, w.mobile ? 48 : 96), aiMat);
  ai.position.set(R.x + 2, Y, R.z);
  ai.rotation.y = -0.35;
  kit.shadowAt(R.x - 2, R.z, 2.2);
  kit.shadowAt(R.x + 2, R.z, 2.2);

  const slabGeo = new THREE.BoxGeometry(1, 2, 0.1);
  const slab = new THREE.Mesh(slabGeo, kit.mat.raised);
  const half = new THREE.Mesh(slabGeo, kit.mat.raised);
  half.scale.y = 0.5;
  const lineMat = kit.tint(COLOR.stone);
  const line = new THREE.Mesh(new THREE.BoxGeometry(0.8, 0.07, 0.02), lineMat);
  line.position.set(0, 0.25, 0.06);
  slab.add(line);
  const bandGeo = new THREE.BoxGeometry(1, 0.05, 0.02).translate(0.5, 0, 0);
  const bandL = new THREE.Mesh(bandGeo, kit.mat.teal);
  const bandR = new THREE.Mesh(bandGeo, kit.mat.teal);
  bandL.position.set(R.x - 2 + 0.2, Y + 0.25, R.z + 0.08);
  bandR.position.set(R.x + 2 - 0.2, Y + 0.25, R.z + 0.08);
  bandR.rotation.y = Math.PI;
  const people = [0, 1].map((i) => {
    const m = new THREE.Mesh(kit.geo.pebble, kit.mat.unknown);
    m.scale.setScalar(0.7);
    m.position.set(R.x + 3.6 + i * 0.85, 0.36, R.z + 1.6);
    return m;
  });
  group.add(rules, ai, slab, half, bandL, bandR, ...people);

  const W = v3(PLACES.wall);
  const wallTex = dotWallTexture(w.figures.closuresDone, w.figures.closuresWrong, w.mobile ? 1024 : 2048);
  const wall = new THREE.Mesh(new THREE.PlaneGeometry(4.6, 4.6), new THREE.MeshBasicMaterial({ map: wallTex }));
  wall.position.copy(W);
  kit.addStatic(kit.mat.surface, new THREE.BoxGeometry(4.9, 4.9, 0.12), m4().setPosition(W.x, W.y, W.z - 0.08));
  kit.addStatic(kit.mat.stone, new THREE.BoxGeometry(1.2, W.y - 2.4, 0.5), m4().setPosition(W.x, (W.y - 2.4) / 2, W.z - 0.2));
  group.add(wall);

  const lit = new THREE.Color(COLOR.tealBright);
  const stone = new THREE.Color(COLOR.stone);
  const teal = new THREE.Color(COLOR.teal);
  return {
    group,
    window: [0.46, 0.68],
    update(p) {
      const s = local(p, "proof");
      const into = ease(span(s, 0, 0.14));
      const out = ease(span(s, 0.4, 0.5));
      slab.position.set(R.x, Y, THREE.MathUtils.lerp(-4, 0, into) - out * 5);
      const reach = ease(span(s, 0.14, 0.32));
      lineMat.color.copy(reach >= 1 ? teal : stone);
      rulesMat.color.copy(s > 0.14 && s < 0.5 ? lit : stone);
      aiMat.color.copy(s > 0.14 && s < 0.82 ? lit : stone);

      // The unfinished check: only the AI's band reaches it, so the two never meet.
      const comes = ease(span(s, 0.48, 0.58));
      const smooth = ease(span(s, 0.58, 0.68));
      const tips = ease(span(s, 0.68, 0.8));
      half.visible = s > 0.46;
      half.position.set(R.x + tips * 1.9, Y - 0.5 - tips * 0.45, THREE.MathUtils.lerp(-4, 0, comes) + tips * 0.9);
      half.rotation.z = -tips * 1.1;
      bandL.visible = s > 0.14 && s < 0.5;
      bandL.scale.x = Math.max(0.001, reach * 1.6);
      bandR.visible = (s > 0.14 && s < 0.5) || (smooth > 0 && s < 0.8);
      bandR.scale.x = Math.max(0.001, s < 0.5 ? reach * 1.6 : smooth * 0.9);
      const arrive = ease(span(s, 0.72, 0.86));
      people.forEach((m, i) => {
        m.visible = arrive > 0;
        m.position.z = R.z + THREE.MathUtils.lerp(-3, 1.6, arrive) + i * 0.1;
      });
      return false;
    },
  };
}

/** Beat 5: the thread closes into a seal at the foot of a receipt that stands up square to the camera. */
export function receipt(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const P = v3(PLACES.receipt);
  const card = new THREE.Group();
  card.add(new THREE.Mesh(new THREE.BoxGeometry(2.3, 3.1, 0.06), kit.mat.surface));
  const lineGeo = new THREE.BoxGeometry(1.6, 0.09, 0.02);
  for (let i = 0; i < 5; i++) {
    const l = new THREE.Mesh(lineGeo, kit.mat.stone);
    l.position.set(i === 0 ? -0.35 : 0, 1.05 - i * 0.36, 0.04);
    if (i === 0) l.scale.x = 0.55;
    card.add(l);
  }
  const seal = new THREE.Mesh(new THREE.CylinderGeometry(0.3, 0.3, 0.06, 40).rotateX(Math.PI / 2), kit.mat.teal);
  seal.position.set(0, -1.1, 0.05);
  card.add(seal);
  const loop = new THREE.Mesh(new THREE.TorusGeometry(1.7, 0.05, 10, 96), kit.mat.teal);
  group.add(card, loop);
  kit.shadowAt(P.x, P.z + 0.3, 3.4);
  return {
    group,
    window: [0.6, 0.8],
    update(p) {
      const s = local(p, "receipt");
      const stand = ease(span(s, 0, 0.45));
      card.position.set(P.x, THREE.MathUtils.lerp(0.1, P.y, stand), P.z);
      card.rotation.x = -(1 - stand) * (Math.PI / 2 - 0.15);
      const tie = ease(span(s, 0.2, 0.6));
      loop.visible = tie < 1;
      loop.scale.setScalar(Math.max(0.001, 1 - tie * 0.82));
      loop.position.set(P.x, THREE.MathUtils.lerp(P.y, P.y - 1.1, tie), P.z + 0.06);
      loop.rotation.set(0.5 * (1 - tie), 0.7 * (1 - tie), 0);
      seal.scale.setScalar(Math.max(0.001, ease(span(s, 0.5, 0.65))));
      return false;
    },
  };
}

/** Beat 6: who notices. Four lit in turn around the seal, each tied to it by a thread. */
export function noticers(w: World): Part {
  const { kit } = w;
  const group = new THREE.Group();
  const O = v3(PLACES.who);
  kit.addStatic(kit.mat.teal, new THREE.CylinderGeometry(0.42, 0.42, 0.06, 40), m4().setPosition(O.x, 0.03, O.z));
  const R = 2.7;
  const at = [-Math.PI / 2, Math.PI * 0.95, 0.15, Math.PI / 2].map((a) => new THREE.Vector3(O.x + Math.cos(a) * R, 0, O.z + Math.sin(a) * R));
  const bodies = at.map(() => kit.tint(COLOR.stone));

  const person = kit.pebble(w.faces.fatima, w.unknown, bodies[0]);
  const nurse = kit.pebble(w.faces.nurse, w.unknown, bodies[1]);
  const auditor = kit.pebble(w.faces.auditor, w.unknown, bodies[3]);
  for (const [g, p] of [
    [person.group, at[0]],
    [nurse.group, at[1]],
    [auditor.group, at[3]],
  ] as const) {
    g.scale.setScalar(1.2);
    g.position.set(p.x, 0.6, p.z);
    g.rotation.x = -0.35;
  }
  const tileGeo = new THREE.BoxGeometry(0.5, 0.06, 0.34);
  const topTile = kit.tint(COLOR.stone);
  const tiles = [0, 1, 2].map((i) => {
    const m = new THREE.Mesh(tileGeo, i === 0 ? topTile : kit.mat.raised);
    m.position.set(at[1].x + 0.95, 0.04, at[1].z - 0.5 + i * 0.42);
    return m;
  });
  const lens = new THREE.Mesh(new THREE.TorusGeometry(0.32, 0.05, 10, 40).rotateX(Math.PI / 2), kit.mat.ink);

  const n = w.mobile ? 16 : 40;
  const cluster = new THREE.InstancedMesh(kit.geo.pebbleLow, bodies[2], n);
  const d = new THREE.Object3D();
  for (let i = 0; i < n; i++) {
    const a = i * 2.39996;
    const rr = 0.18 * Math.sqrt(i + 0.5) * (w.mobile ? 1.5 : 1);
    d.position.set(at[2].x + Math.cos(a) * rr, 0.18, at[2].z + Math.sin(a) * rr);
    d.scale.setScalar(0.36);
    d.updateMatrix();
    cluster.setMatrixAt(i, d.matrix);
  }
  for (const p of at) kit.shadowAt(p.x, p.z, 2);

  const threadGeo = new THREE.BoxGeometry(1, 0.05, 0.05).translate(0.5, 0, 0);
  const threads = at.map((p) => {
    const t = new THREE.Mesh(threadGeo, kit.mat.teal);
    t.position.set(O.x, 0.08, O.z);
    t.rotation.y = -Math.atan2(p.z - O.z, p.x - O.x);
    return t;
  });
  group.add(person.group, nurse.group, auditor.group, lens, cluster, ...tiles, ...threads);

  const lit = new THREE.Color(COLOR.surface);
  const dim = new THREE.Color(COLOR.stone);
  const glow = new THREE.Color(COLOR.tealBright);
  return {
    group,
    window: [0.72, 0.94],
    update(p) {
      const s = local(p, "who");
      at.forEach((_, i) => {
        const on = ease(span(s, 0.08 + i * 0.2, 0.22 + i * 0.2));
        bodies[i].color.copy(dim).lerp(lit, on);
        threads[i].scale.x = Math.max(0.001, on * (R - 0.75));
      });
      topTile.color.copy(s > 0.28 ? glow : dim);
      const sweep = span(s, 0.68, 0.9) * Math.PI * 2;
      lens.visible = s > 0.66;
      lens.position.set(O.x + Math.sin(sweep) * 0.9, 0.9, O.z + 0.9 + Math.cos(sweep) * 0.5);
      return false;
    },
  };
}
