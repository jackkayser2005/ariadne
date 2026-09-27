import test from "node:test";
import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
import { parseProfile } from "./profile.mjs";

async function serve(handler) {
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  return server;
}

function address(server) {
  return "http://127.0.0.1:" + server.address().port;
}

async function waitUntil(check, message) {
  for (let attempt = 0; attempt < 60; attempt++) {
    if (await check()) return;
    await delay(100);
  }
  throw new Error(message);
}

function startGuide(binary, site, output) {
  const child = spawn(binary, ["investigate", "--no-open", "--output", output, site], {
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  return new Promise((resolve, reject) => {
    let stdout = "", stderr = "", settled = false;
    const timer = setTimeout(() => finish(new Error("Guided interface did not start")), 30000);
    function finish(error, url) {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (error) {
        child.kill();
        reject(error);
      } else {
        resolve({ child, url });
      }
    }
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      const match = stdout.match(/Ariadne local interface:\s+(http:\/\/127\.0\.0\.1:\d+\/#[A-Z2-7]+)/);
      if (match) finish(null, match[1]);
    });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("error", finish);
    child.on("exit", (code) => finish(new Error("Guided interface exited " + code + ": " + stderr.slice(0, 500))));
  });
}

async function fillRecordedEmail(page, destination) {
  const fill = page.getByRole("button", { name: "Fill test email" });
  for (let attempt = 0; attempt < 25; attempt++) {
    await fill.click();
    await waitUntil(() => fill.isEnabled(), "The fill action did not finish");
    if (await page.locator("#message").isVisible()) {
      await delay(100);
      continue;
    }
    await page.locator("#cards article").filter({ hasText: destination }).waitFor({ timeout: 10000 });
    return;
  }
  throw new Error("The synthetic input never reached the local fixture");
}

async function waitForMarkerObservation(page, kind, site) {
  await waitUntil(async () => {
    const rows = await page.locator("#timeline li").allTextContents();
    return rows.some((row) => row.startsWith(kind + " · page · " + site) && row.includes("Email"));
  }, "The rendered journey did not show its " + kind + " marker observation");
}

test("rendered guide records, compares, and exports a bounded local journey", { timeout: 150000 }, async (t) => {
  assert.equal(process.platform, "win32", "The guided acceptance runner uses visible Windows Chrome/Edge");
  const binary = process.env.ARIADNE_BINARY;
  assert.ok(binary, "Set ARIADNE_BINARY to a built Ariadne executable");
  const root = await mkdtemp(join(tmpdir(), "ariadne-guided-"));
  let collector, siteServer, guide, browser;
  t.after(async () => {
    if (browser) await browser.close();
    if (guide?.child.exitCode === null) {
      try {
        const token = new URL(guide.url).hash.slice(1);
        await fetch(new URL("/api/cancel", guide.url), {
          method: "POST",
          headers: {
            Authorization: "Bearer " + token,
            Origin: new URL(guide.url).origin,
            "Content-Type": "application/json",
          },
          body: "{}",
          signal: AbortSignal.timeout(3000),
        });
      } catch {
        // The server may already be gone; process termination still follows.
      }
      guide.child.kill();
      await Promise.race([new Promise((resolve) => guide.child.once("exit", resolve)), delay(3000)]);
    }
    if (siteServer) await new Promise((resolve) => siteServer.close(resolve));
    if (collector) await new Promise((resolve) => collector.close(resolve));
    await rm(root, { recursive: true, force: true });
  });

  let received = 0;
  collector = await serve((_req, response) => {
    received++;
    response.writeHead(200, { "Access-Control-Allow-Origin": "*", "Content-Type": "text/plain" });
    response.end("ok");
  });
  const destination = address(collector);
  siteServer = await serve((_req, response) => {
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    response.end('<!doctype html><title>Guided local fixture</title><label>Test email <input type="email" autofocus></label><p id="task">Ready</p><script>document.querySelector("input").addEventListener("input",function(event){localStorage.setItem("fixture",event.target.value);fetch("' + destination + '/collect?v="+encodeURIComponent(event.target.value)).then(function(){document.querySelector("#task").textContent="Worked"}).catch(function(){document.querySelector("#task").textContent="Blocked"})})</script>');
  });
  const site = address(siteServer);
  guide = await startGuide(binary, site, join(root, "runs"));
  browser = await chromium.launch({
    channel: process.env.ARIADNE_COMPANION_CHANNEL || "chromium",
    headless: true,
  });
  const page = await browser.newPage({ acceptDownloads: true });
  await page.goto(guide.url);
  await page.getByRole("status").first().getByText("Ready to investigate.").waitFor();

  const addressField = page.getByRole("textbox", { name: "Website address" });
  await addressField.focus();
  await page.keyboard.press("Tab");
  assert.equal(await page.evaluate(() => document.activeElement?.id), "start");
  await addressField.fill("ftp://invalid.example");
  await page.getByRole("button", { name: "Start investigation" }).click();
  await page.locator("#message").waitFor({ state: "visible" });
  await addressField.fill(site);
  await page.getByRole("button", { name: "Start investigation" }).click();
  await page.getByRole("button", { name: "Stop and review" }).waitFor();
  const marker = await page.locator("#marker-m1").inputValue();
  await fillRecordedEmail(page, destination);
  await waitUntil(() => received === 1, "The baseline request did not reach the local collector");
  await waitForMarkerObservation(page, "input", site);
  await waitForMarkerObservation(page, "storage write", site);
  await waitUntil(async () => (await page.locator("#timeline").textContent()).includes("response"), "The baseline response was not observed");
  await page.getByRole("button", { name: "Stop and review" }).click();
  await page.getByRole("button", { name: "Export verified evidence" }).waitFor();

  await page.getByRole("checkbox", { name: destination }).check();
  await page.getByRole("button", { name: "Start with these changes" }).click();
  await page.getByRole("button", { name: "Stop and review" }).waitFor();
  assert.equal(await page.locator("#marker-m1").inputValue(), marker, "The trial must reuse its baseline test input");
  await fillRecordedEmail(page, destination);
  await page.locator("#cards article").filter({ hasText: "blocked" }).waitFor();
  await delay(400);
  assert.equal(received, 1, "The blocked trial must not reach the collector");
  await page.getByRole("button", { name: "Stop and review" }).click();
  await page.getByRole("heading", { name: "What changed?" }).waitFor();
  assert.match(await page.locator("#comparison-summary").textContent(), /remains unknown/i);
  assert.deepEqual(await page.locator("#comparison-counts tr").nth(2).locator("td").allTextContents(), ["0", "1"]);

  await page.getByRole("combobox", { name: "Baseline task" }).selectOption("works");
  await page.getByRole("combobox", { name: "Task with privacy changes" }).selectOption("broken");
  await page.getByRole("button", { name: "Use these reports" }).click();
  assert.match(await page.locator("#functionality-summary").textContent(), /Automatic functionality verification: unknown/);

  const [portableDownload] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Export verified evidence" }).click(),
  ]);
  const portablePath = join(root, "portable.json");
  await portableDownload.saveAs(portablePath);
  const portableText = await readFile(portablePath, "utf8");
  assert.ok(!portableText.includes(site) && !portableText.includes(destination) && !portableText.includes(marker), "Portable evidence must exclude private context");
  const portable = JSON.parse(portableText);
  assert.equal(portable.receipt.kind, "browser-journey");
  const inspected = spawnSync(binary, ["inspect", "--json", portablePath], { encoding: "utf8", timeout: 30000, windowsHide: true });
  assert.equal(inspected.status, 0, inspected.stderr);

  const [profileDownload] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Save private protection profile" }).click(),
  ]);
  const profilePath = join(root, "profile.json");
  await profileDownload.saveAs(profilePath);
  const profile = await parseProfile(await readFile(profilePath, "utf8"));
  assert.deepEqual(profile.controls.block_origins, [destination]);
  assert.equal(profile.test.reduction, "unknown");
  assert.equal(profile.test.functionality_state, "claimed");
  assert.equal(profile.test.automatic_functionality, "unknown");
  assert.equal((await readdir(join(root, "runs"))).length, 2, "Baseline and trial must publish separately");
});
