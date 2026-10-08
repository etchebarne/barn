// The openbot server the app connects to: an address the user types, checked before it's saved.

/** Turns what the user typed into a server origin ("100.1.2.3:8080" → "http://100.1.2.3:8080"). */
export function normalizeServer(input: string): string {
  let text = input.trim()
  if (text === "") throw new Error("Enter your openbot server's address.")
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(text)) text = "http://" + text
  let url: URL
  try {
    url = new URL(text)
  } catch {
    throw new Error("That doesn't look like an address, e.g. http://100.64.0.1:8080")
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("The address must start with http:// or https://")
  }
  return url.origin
}

/** Checks that an openbot server answers at origin. */
export async function checkServer(origin: string, timeoutMs = 8000): Promise<void> {
  let res: Response
  try {
    res = await fetch(origin + "/api/auth/status", { signal: AbortSignal.timeout(timeoutMs) })
  } catch {
    throw new Error(
      `Couldn't reach ${origin}. Check the address, and that this computer can reach it (for example over Tailscale).`,
    )
  }
  const body: unknown = await res.json().catch(() => null)
  if (!res.ok || typeof body !== "object" || body === null || !("setupRequired" in body)) {
    throw new Error(`${origin} answered, but it isn't an openbot server.`)
  }
}

/** Whether url belongs to origin. */
export function sameOrigin(url: string, origin: string): boolean {
  try {
    return new URL(url).origin === origin
  } catch {
    return false
  }
}

function versionParts(v: string): number[] {
  return v
    .replace(/^v/, "")
    .split(/[.-]/)
    .map((p) => Number.parseInt(p, 10) || 0)
}

/** Compares release versions like "0.4.7" (a "v" prefix is ignored); positive when a > b. */
export function compareVersions(a: string, b: string): number {
  const pa = versionParts(a)
  const pb = versionParts(b)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0)
    if (d !== 0) return d
  }
  return 0
}
