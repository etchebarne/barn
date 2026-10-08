import assert from "node:assert/strict"
import { test } from "node:test"

import { compareVersions, normalizeServer, sameOrigin } from "./server"

void test("normalizeServer", () => {
  assert.equal(normalizeServer(" 100.64.0.1:8080 "), "http://100.64.0.1:8080")
  assert.equal(normalizeServer("https://bot.example.com/chats/1"), "https://bot.example.com")
  assert.equal(normalizeServer("HTTP://Bot.Example.com:80/"), "http://bot.example.com")
  assert.throws(() => normalizeServer(""), /Enter/)
  assert.throws(() => normalizeServer("ftp://x"), /http/)
  assert.throws(() => normalizeServer("http://"), /address/)
})

void test("sameOrigin", () => {
  assert.ok(sameOrigin("http://100.64.0.1:8080/chats/x", "http://100.64.0.1:8080"))
  assert.ok(!sameOrigin("http://100.64.0.1:9090/", "http://100.64.0.1:8080"))
  assert.ok(!sameOrigin("https://evil.example", "http://100.64.0.1:8080"))
  assert.ok(!sameOrigin("not a url", "http://100.64.0.1:8080"))
})

void test("compareVersions", () => {
  assert.ok(compareVersions("v0.4.10", "0.4.9") > 0)
  assert.ok(compareVersions("0.4.7", "v0.4.7") === 0)
  assert.ok(compareVersions("0.5.0", "0.4.99") > 0)
  assert.ok(compareVersions("0.4.7", "0.4.8") < 0)
})
