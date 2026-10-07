/**
 * A random UUID (v4). `crypto.randomUUID` only exists in secure contexts (HTTPS or localhost),
 * and barn is often opened over plain http on a Tailscale address, so fall back to
 * `getRandomValues`, which works everywhere.
 */
export function randomId(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID()
  const b = crypto.getRandomValues(new Uint8Array(16))
  b[6] = (b[6]! & 0x0f) | 0x40
  b[8] = (b[8]! & 0x3f) | 0x80
  const hex = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("")
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
