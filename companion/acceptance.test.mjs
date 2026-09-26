import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve, sep, basename } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const extension = fileURLToPath(new URL(".", import.meta.url));
function testedProfile(site, destination) {
  const p = {
    schema_version: 1,
    kind: "private-site-protection",
    site_origin: site,
    controls: { location: "deny", block_origins: [destination] },
    test: {
      baseline_journey_sha256: "a".repeat(64),
      treatment_journey_sha256: "b".repeat(64),
      comparison_sha256: "c".repeat(64),
      order: "baseline-treatment",
      baseline: { requests: 1, responses: 1, blocked: 0, websocket_frames: 0 },
      treatment: { requests: 1, responses: 0, blocked: 1, websocket_frames: 0 },
      reduction: "unknown",
      evidence_state: "unknown",
      functionality: { baseline: "works", treatment: "works" },
      functionality_state: "claimed",
      automatic_functionality: "unknown",
    },
  };
  p.profile_sha256 = createHash("sha256")
    .update(JSON.stringify(p))
    .digest("hex");
  return p;
}
async function serve(handler) {
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  return server;
}
const address = (server) => "http://127.0.0.1:" + server.address().port;

// Attach independently to the action's new target: applying ordinary tab
// initialization to a native popup can crash some Chromium releases.
async function popupSession(browser, id, existing) {
  let target,
    targetInfos = [];
  for (let attempt = 0; attempt < 50 && !target; attempt++) {
    ({ targetInfos } = await browser.send("Target.getTargets", {
      filter: [{}],
    }));
    target = targetInfos.find(
      (target) =>
        target.type !== "tab" &&
        !existing.has(target.targetId) &&
        target.url === "chrome-extension://" + id + "/protection.html",
    );
    if (!target) await new Promise((resolve) => setTimeout(resolve, 40));
  }
  assert.ok(
    target,
    "native companion popup did not open: " +
      JSON.stringify(targetInfos.map(({ type, url }) => ({ type, url }))),
  );
  const { sessionId } = await browser.send("Target.attachToTarget", {
    targetId: target.targetId,
    flatten: false,
  });
  let next = 0;
  return {
    targetId: target.targetId,
    call(method, params = {}) {
      const id = ++next;
      return new Promise((resolve, reject) => {
        const done = (error, value) => {
          clearTimeout(timer);
          browser.off("Target.receivedMessageFromTarget", receive);
          error ? reject(error) : resolve(value);
        };
        const receive = (event) => {
          if (event.sessionId !== sessionId) return;
          const message = JSON.parse(event.message);
          if (message.id === id)
            done(
              message.error ? new Error(message.error.message) : null,
              message.result,
            );
        };
        const timer = setTimeout(
          () => done(new Error("Popup command timed out: " + method)),
          10000,
        );
        browser.on("Target.receivedMessageFromTarget", receive);
        browser
          .send("Target.sendMessageToTarget", {
            sessionId,
            message: JSON.stringify({ id, method, params }),
          })
          .catch((error) => done(error));
      });
    },
  };
}

test(
  "real companion import, blocking, frames, location, alias, restart, pause and complete undo",
  { timeout: 180000 },
  async (t) => {
    const root = await mkdtemp(join(tmpdir(), "ariadne-companion-"));
    let received = 0,
      websockets = 0,
      submissions = 0,
      context;
    const collector = await serve((req, res) => {
      received++;
      res.setHeader("Access-Control-Allow-Origin", "*");
      res.end("received");
    });
    collector.on("upgrade", (_req, socket) => {
      websockets++;
      socket.end("HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n");
    });
    const destination = address(collector);
    const siteServer = await serve((req, res) => {
      if (req.url === "/submit") {
        submissions++;
        res.end("submitted");
        return;
      }
      if (req.url === "/worker.js") {
        res.setHeader("Content-Type", "text/javascript");
        res.end(
          `onmessage=async()=>{try{await fetch(${JSON.stringify(destination)}+'/worker');postMessage('sent')}catch{postMessage('blocked')}}`,
        );
        return;
      }
      res.setHeader("Content-Type", "text/html");
      res.setHeader("Permissions-Policy", "geolocation=*");
      res.end(
        `<!doctype html><html lang="en"><title>Companion local fixture</title><form action="/submit"><label>Alias field <input id="alias"></label><label>Password <input type="password" id="password"></label></form><button id="send" type="button">Send fixture request</button><p id="result">ready</p><script>document.querySelector('#send').onclick=async()=>{try{await fetch(${JSON.stringify(destination)}+'/collect');document.querySelector('#result').textContent='sent'}catch{document.querySelector('#result').textContent='blocked'}};</script>${req.url === "/frame" ? "" : '<iframe title="Embedded fixture" src="http://frame.localhost:' + siteServer.address().port + '/frame" allow="geolocation"></iframe>'}</html>`,
      );
    });
    const site = "http://protected.localhost:" + siteServer.address().port;
    const other = "http://other.localhost:" + siteServer.address().port;
    const profileFile = join(root, "private-profile.json");
    await writeFile(
      profileFile,
      JSON.stringify(testedProfile(site, destination)),
    );
    const launch = () =>
      chromium.launchPersistentContext(join(root, "browser"), {
        channel: process.env.ARIADNE_COMPANION_CHANNEL || "chromium",
        headless: process.env.ARIADNE_COMPANION_HEADED !== "1",
        args: [
          "--disable-extensions-except=" + extension,
          "--load-extension=" + extension,
          "--enable-unsafe-extension-debugging",
          "--host-resolver-rules=MAP *.localhost 127.0.0.1",
        ],
        ignoreDefaultArgs: ["--disable-extensions"],
      });
    try {
      context = await launch();
      let worker =
        context.serviceWorkers()[0] ||
        (await context.waitForEvent("serviceworker"));
      const id = new URL(worker.url()).host;
      let manager = await context.newPage();
      await manager.goto("chrome-extension://" + id + "/protection.html");
      const main = await context.newPage();
      await main.goto(site);
      const isolated = await context.newPage();
      await isolated.goto(other);
      const permission = (page) =>
        page.evaluate(() =>
          navigator.permissions
            .query({ name: "geolocation" })
            .then((p) => p.state),
        );
      const priorLocation = await permission(main);
      assert.notEqual(priorLocation, "denied");
      const send = async (page, want) => {
        await page
          .locator("#result")
          .evaluate((e) => (e.textContent = "ready"));
        await page
          .getByRole("button", { name: "Send fixture request" })
          .click();
        await page.waitForFunction(
          (want) => document.querySelector("#result").textContent === want,
          want,
        );
      };
      await send(main, "sent");
      assert.equal(received, 1);
      for (const text of [
        '{"schema_version":1,"schema_version":1}',
        " ".repeat(65537),
        JSON.stringify(testedProfile(site, destination)).replace(
          '"location":"deny"',
          '"location":"unchanged"',
        ),
      ]) {
        await manager
          .getByLabel("Profile file")
          .setInputFiles({
            name: "bad.json",
            mimeType: "application/json",
            buffer: Buffer.from(text),
          });
        await manager.getByRole("alert").waitFor({ state: "visible" });
        assert.equal(
          await manager
            .getByRole("button", { name: "Import paused", exact: true })
            .isVisible(),
          false,
        );
      }
      await manager.getByLabel("Profile file").setInputFiles(profileFile);
      await manager
        .getByRole("button", { name: "Import paused", exact: true })
        .waitFor();
      await manager.getByLabel("Profile file").focus();
      await manager.keyboard.press("Tab");
      assert.equal(
        await manager.evaluate(() => document.activeElement.id),
        "import",
      );
      await manager.keyboard.press("Enter");
      await manager.getByText("Paused", { exact: true }).waitFor();
      assert.equal(
        (
          await worker.evaluate(() =>
            chrome.declarativeNetRequest.getDynamicRules(),
          )
        ).length,
        0,
      );
      const control = async (label, want) => {
        await manager.getByRole("button", { name: label, exact: true }).click();
        await manager.waitForFunction(
          (want) =>
            document.querySelector("#sites strong")?.textContent === want ||
            !document.querySelector("#error").hidden,
          want,
        );
        assert.equal(
          await manager.locator("#error").isVisible(),
          false,
          await manager.locator("#error").textContent(),
        );
      };
      await control("Enable", "Enabled");
      assert.equal(await permission(main), "denied");
      assert.equal(await permission(isolated), priorLocation);
      await send(main, "blocked");
      assert.equal(received, 1);
      const frame = main.frame({ url: new RegExp("/frame$") });
      assert.ok(frame);
      assert.equal(
        await permission(frame),
        "denied",
        "embedded location remains available despite the protected top-level origin",
      );
      await frame.getByRole("button", { name: "Send fixture request" }).click();
      await frame.waitForFunction(
        () => document.querySelector("#result").textContent === "blocked",
      );
      assert.equal(received, 1);
      const workerResult = await main.evaluate(
        () =>
          new Promise((resolve) => {
            const worker = new Worker("/worker.js");
            worker.onmessage = (e) => {
              worker.terminate();
              resolve(e.data);
            };
            worker.postMessage("go");
          }),
      );
      assert.equal(workerResult, "blocked");
      assert.equal(received, 1);
      await main.evaluate(
        (url) =>
          new Promise((resolve) => {
            const socket = new WebSocket(url);
            socket.onerror = () => resolve("failed");
            socket.onopen = () => {
              socket.close();
              resolve("opened");
            };
          }),
        destination.replace(/^http/, "ws") + "/socket",
      );
      assert.equal(websockets, 0);
      await send(isolated, "sent");
      assert.equal(received, 2);
      const isolatedFrame = isolated.frame({ url: new RegExp("/frame$") });
      assert.ok(isolatedFrame);
      assert.equal(
        await permission(isolatedFrame),
        priorLocation,
        "another site's embedded location preference changed",
      );
      await send(isolatedFrame, "sent");
      assert.equal(received, 3);
      await manager
        .getByLabel("Alias for " + site, { exact: true })
        .fill("chosen-alias@example.invalid");
      await manager
        .getByRole("button", { name: "Save alias", exact: true })
        .click();
      await manager
        .getByRole("status")
        .filter({ hasText: "Alias saved locally." })
        .waitFor();
      const browserCDP = await context.browser().newBrowserCDPSession();
      const openAction = async (page) => {
        await page.bringToFront();
        const targets = await browserCDP.send("Target.getTargets", {
          filter: [{ type: "tab" }, { exclude: true }],
        });
        const target = targets.targetInfos.find(
          (target) => target.url === page.url(),
        );
        assert.ok(target, "fixture tab is unavailable");
        const existing = new Set(
          (
            await browserCDP.send("Target.getTargets", { filter: [{}] })
          ).targetInfos.map((target) => target.targetId),
        );
        await browserCDP.send("Extensions.triggerAction", {
          id,
          targetId: target.targetId,
        });
        return popupSession(browserCDP, id, existing);
      };
      const fillThroughPopup = async (popup) => {
        const result = await popup.call("Runtime.evaluate", {
          expression: `new Promise((resolve,reject)=>{
      const deadline=setTimeout(()=>{clearInterval(poll);reject(new Error('Alias control did not become available'))},10000);
      let clicked=false;
      const poll=setInterval(()=>{
        const button=document.getElementById('fill');
        if(!clicked&&button&&!document.getElementById('fill-panel').hidden){clicked=true;button.click()}
        const error=document.getElementById('error');
        if(error&&!error.hidden){clearTimeout(deadline);clearInterval(poll);resolve({error:error.textContent})}
        if(document.getElementById('message')?.textContent.includes('Alias filled.')){clearTimeout(deadline);clearInterval(poll);resolve({filled:true})}
      },20);
    })`,
          awaitPromise: true,
          returnByValue: true,
        });
        assert.equal(
          result.exceptionDetails,
          undefined,
          JSON.stringify(result.exceptionDetails),
        );
        return result.result.value;
      };
      const closeAction = (popup) =>
        browserCDP.send("Target.closeTarget", { targetId: popup.targetId });
      for (const select of [
        () => main.locator("#password").focus(),
        async () => {
          await main.locator("#alias").evaluate((el) => (el.readOnly = true));
          await main.locator("#alias").focus();
        },
        () => frame.locator("#alias").focus(),
      ]) {
        await select();
        const popup = await openAction(main);
        const result = await fillThroughPopup(popup);
        assert.match(result.error, /Select an editable text field/);
        await closeAction(popup);
        await main.locator("#alias").evaluate((el) => (el.readOnly = false));
        assert.equal(await main.locator("#alias").inputValue(), "");
        assert.equal(await main.locator("#password").inputValue(), "");
        assert.equal(await frame.locator("#alias").inputValue(), "");
      }
      const otherPopup = await openAction(isolated);
      const isolatedState = await otherPopup.call("Runtime.evaluate", {
        expression: `new Promise(resolve=>{const poll=setInterval(()=>{if(document.querySelector('#sites strong')){clearInterval(poll);resolve(document.getElementById('fill-panel').hidden)}},20)})`,
        awaitPromise: true,
        returnByValue: true,
      });
      assert.equal(
        isolatedState.result.value,
        true,
        "another site exposed alias filling",
      );
      await closeAction(otherPopup);
      await main
        .getByRole("textbox", { name: "Alias field", exact: true })
        .focus();
      const popup = await openAction(main);
      assert.equal((await fillThroughPopup(popup)).filled, true);
      assert.equal(
        await main
          .getByRole("textbox", { name: "Alias field", exact: true })
          .inputValue(),
        "chosen-alias@example.invalid",
      );
      assert.equal(submissions, 0);
      await closeAction(popup);
      await manager.setViewportSize({ width: 390, height: 844 });
      assert.equal(
        await manager.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
      );
      await context.close();
      context = await launch();
      worker =
        context.serviceWorkers()[0] ||
        (await context.waitForEvent("serviceworker"));
      manager = await context.newPage();
      await manager.goto("chrome-extension://" + id + "/protection.html");
      await manager.getByText("Enabled", { exact: true }).waitFor();
      assert.equal(
        await manager
          .getByLabel("Alias for " + site, { exact: true })
          .inputValue(),
        "chosen-alias@example.invalid",
      );
      const restarted = await context.newPage();
      await restarted.goto(site);
      await send(restarted, "blocked");
      assert.equal(await permission(restarted), "denied");
      const beforePause = received;
      await control("Pause", "Paused");
      await send(restarted, "sent");
      assert.equal(received, beforePause + 1);
      assert.equal(await permission(restarted), priorLocation);
      await control("Enable", "Enabled");
      await manager
        .getByRole("button", { name: "Undo and remove", exact: true })
        .click();
      await manager
        .getByText("No profiles imported.", { exact: false })
        .waitFor();
      const after = await worker.evaluate(async () => ({
        rules: await chrome.declarativeNetRequest.getDynamicRules(),
        state: (await chrome.storage.local.get("protection")).protection,
      }));
      assert.deepEqual(after.rules, []);
      assert.deepEqual(after.state.entries, []);
      assert.equal(await permission(restarted), priorLocation);
      await send(restarted, "sent");
      assert.equal(received, beforePause + 2);
      assert.equal(submissions, 0);
      t.diagnostic(
        "Verified native rules, embedded/worker/WebSocket blocking, exact-site explicit alias fill, browser restart, restored location preferences, and complete undo.",
      );
    } finally {
      if (context) await context.close();
      siteServer.closeAllConnections();
      collector.closeAllConnections();
      await Promise.all([
        new Promise((r) => siteServer.close(r)),
        new Promise((r) => collector.close(r)),
      ]);
      assert.ok(
        resolve(root).startsWith(resolve(tmpdir()) + sep) &&
          basename(root).startsWith("ariadne-companion-"),
      );
      await rm(root, {
        recursive: true,
        force: true,
        maxRetries: 20,
        retryDelay: 100,
      });
    }
  },
);
