import { afterEach, describe, expect, it, vi } from "vitest"

import {
  acceptFiles,
  attachmentSummary,
  downloadUrl,
  formatBytes,
  MAX_ATTACHMENT_BYTES,
  uploadAttachment,
  type Attachment,
} from "./attachments"

function file(name: string, size = 10, type = "text/plain") {
  const f = new File(["x"], name, { type })
  Object.defineProperty(f, "size", { value: size })
  return f
}

const image = (id: string): Attachment => ({
  id,
  name: `${id}.png`,
  mime: "image/png",
  size: 100,
  url: `/api/attachments/${id}`,
  width: 800,
  height: 600,
})

afterEach(() => vi.unstubAllGlobals())

describe("acceptFiles", () => {
  it("rejects files over 25 MB with a clear message", () => {
    const big = file("video.mov", MAX_ATTACHMENT_BYTES + 1)
    const ok = file("notes.txt")
    expect(acceptFiles(0, [big, ok])).toEqual({ accepted: [ok], error: "video.mov is over 25 MB." })
  })

  it("accepts at most 10 per message", () => {
    const files = Array.from({ length: 4 }, (_, i) => file(`f${i}.txt`))
    const result = acceptFiles(8, files)
    expect(result.accepted).toHaveLength(2)
    expect(result.error).toBe("You can attach up to 10 files per message.")
    expect(acceptFiles(0, files)).toEqual({ accepted: files, error: null })
  })
})

describe("helpers", () => {
  it("formats sizes", () => {
    expect(formatBytes(512)).toBe("512 B")
    expect(formatBytes(1536)).toBe("1.5 KB")
    expect(formatBytes(3.2 * 1024 * 1024)).toBe("3.2 MB")
    expect(formatBytes(25 * 1024 * 1024)).toBe("25 MB")
  })

  it("builds download links", () => {
    expect(downloadUrl({ url: "/api/attachments/a1" })).toBe("/api/attachments/a1?download=1")
  })

  it("summarizes attachments for previews", () => {
    expect(attachmentSummary([image("a")])).toBe("🖼 Image")
    expect(attachmentSummary([image("a"), image("b")])).toBe("🖼 2 images")
    expect(
      attachmentSummary([
        image("a"),
        { ...image("r"), name: "report.pdf", mime: "application/pdf" },
      ]),
    ).toBe("📎 report.pdf +1")
    expect(attachmentSummary([])).toBeNull()
  })
})

describe("uploadAttachment", () => {
  it("posts the file as multipart with the CSRF header", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response(JSON.stringify(image("a1")), { status: 201 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await uploadAttachment("chat 1", file("shot.png", 10, "image/png"))
    expect(result.id).toBe("a1")
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe("/api/chats/chat%201/attachments")
    expect(init?.method).toBe("POST")
    expect(init?.headers).toEqual({ "X-Barn-CSRF": "1" })
    const body = init?.body
    expect(body).toBeInstanceOf(FormData)
    expect(body instanceof FormData ? body.get("file") : null).toBeInstanceOf(File)
  })

  it("turns 413 and 400 into readable errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValue(new Response("", { status: 413 })),
    )
    await expect(uploadAttachment("c", file("big.bin"))).rejects.toThrow(
      "Files can be up to 25 MB.",
    )
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(
          new Response(JSON.stringify({ message: "Unsupported file" }), { status: 400 }),
        ),
    )
    await expect(uploadAttachment("c", file("x.exe"))).rejects.toThrow("Unsupported file")
  })
})
