/** Deterministic randomness from a string seed (an agent id), for identicons. */

function hash(seed: string): number {
  // FNV-1a, 32-bit.
  let h = 0x811c9dc5
  for (let i = 0; i < seed.length; i++) {
    h ^= seed.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h
}

export type Random = {
  /** A float in [0, 1). */
  next: () => number
  /** An integer in [min, max]. */
  int: (min: number, max: number) => number
}

/** mulberry32, seeded by the string's hash: the same seed always yields the same sequence. */
export function seeded(seed: string): Random {
  let a = hash(seed)
  const next = () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  return {
    next,
    int: (min, max) => min + Math.floor(next() * (max - min + 1)),
  }
}
