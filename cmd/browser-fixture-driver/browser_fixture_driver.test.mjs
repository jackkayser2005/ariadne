import test from "node:test";
import assert from "node:assert/strict";
import {DevTools} from "./browser_fixture_driver.mjs";

test("stalled browser command times out and releases its pending entry", async () => {
  const client = new DevTools("ws://127.0.0.1/", Date.now() + 20);
  client.socket = {send() {}, readyState: 3};
  await assert.rejects(client.command("Page.enable"), /timed out/);
  assert.equal(client.pending.size, 0);
});
