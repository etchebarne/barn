import { seeded } from "./seed"

/**
 * Each agent's avatar palette: "Dusk" (deep base → mid → violet → pink glow → peach highlight)
 * rotated around the hue wheel by the agent's id, so agents share one look but are easy to
 * tell apart. Offsets and lightness mirror the original Dusk palette:
 * #0f0c29 #302b63 #6f5bd6 #e96fa8 #ffb48f.
 */
export function duskPalette(seed: string): string[] {
  // Base hues from teal through blue and violet to magenta: dark tones there stay rich instead
  // of turning brown or olive. The highlights (+86°, +133°) still reach warm colors.
  const h = 165 + seeded(seed).int(0, 175)
  const at = (offset: number) => (h + offset + 360) % 360
  return [
    `hsl(${at(0)} 55% 14%)`,
    `hsl(${at(-1)} 40% 28%)`,
    `hsl(${at(4)} 60% 60%)`,
    `hsl(${at(86)} 74% 67%)`,
    `hsl(${at(133)} 100% 78%)`,
  ]
}
