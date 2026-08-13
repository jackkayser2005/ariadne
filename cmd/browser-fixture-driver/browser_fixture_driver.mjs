import { createServer as createHTTPServer } from "node:http";
import { createServer as createTCPServer } from "node:net";
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { isAbsolute, join } from "node:path";

const procedureID = "browser-local-fixture-v1";
const maxInputBytes = 16 << 10;
const maxDurationMS = 5 * 60 * 1000;
const maxEvents = 1024;
const safeFieldNames = new Map([
  ["account_id", "account-id"],
  ["account-id", "account-id"],
  ["consent", "consent"],
  ["device_id", "device-id"],
  ["device-id", "device-id"],
  ["region", "region"],
  ["session_id", "session-id"],
  ["session-id", "session-id"],
]);
let stage = "input";

function browserArgument() {
  const args = process.argv.slice(2);
  const index = args.indexOf("--browser");
  if (index < 0 || !args[index + 1] || args[index + 1].startsWith("-")) {
    throw new Error("an explicit browser executable is required");
  }
  const executable = args[index + 1];
  if (!isAbsolute(executable) || !existsSync(executable)) {
    throw new Error("browser executable is unavailable");
  }
  return executable;
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function readProcedure() {
  const chunks = [];
  let length = 0;
  for await (const chunk of process.stdin) {
    length += chunk.length;
    if (length > maxInputBytes) {
      throw new Error("procedure input is too large");
    }
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function listen(server) {
  return new Promise((resolve, reject) => {
    const onError = (error) => {
      server.removeListener("listening", onListening);
      reject(error);
    };
    const onListening = () => {
      server.removeListener("error", onError);
      resolve(server.address().port);
    };
    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(0, "127.0.0.1");
  });
}

function closeServer(server) {
  server.closeAllConnections?.();
  return new Promise((resolve) => {
    server.close(() => resolve());
  });
}

async function stopBrowser(browser) {
  if (browser.exitCode !== null || browser.signalCode !== null || !browser.pid) {
    return;
  }
  if (process.platform === "win32") {
    const killer = spawn("taskkill.exe", ["/PID", String(browser.pid), "/T", "/F"], {
      stdio: "ignore",
      windowsHide: true,
    });
    await new Promise((resolve) => killer.once("exit", resolve));
    await delay(500);
    return;
  }
  try {
    process.kill(-browser.pid, "SIGTERM");
  } catch {
    browser.kill();
  }
}

async function removeProfile(profile) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      await rm(profile, {recursive: true, force: true, maxRetries: 1, retryDelay: 250});
    } catch {
      // Chrome may release one last profile handle after the process exits.
    }
    if (!existsSync(profile)) {
      return;
    }
    await delay(250);
  }
  throw new Error("temporary browser profile could not be removed");
}

async function freePort() {
  const server = createTCPServer();
  const port = await listen(server);
  await closeServer(server);
  return port;
}

function fixturePage(port) {
  return `<!doctype html>
<meta charset="utf-8">
<title>Ariadne local browser fixture</title>
<script>
document.cookie = "consent=fixture";
localStorage.setItem("session_id", "fixture");
fetch("http://analytics.localhost:${port}/collect?region=fixture&session_id=fixture", {mode: "no-cors", keepalive: true});
navigator.sendBeacon("http://analytics.localhost:${port}/beacon?consent=fixture", "fixture");
</script>`;
}

function fixtureServer() {
  let portRef = 0;
  const server = createHTTPServer((request, response) => {
    const host = String(request.headers.host || "").split(":")[0].toLowerCase();
    if (host === "app.localhost" && request.url === "/") {
      response.writeHead(200, {"content-type": "text/html; charset=utf-8"});
      response.end(fixturePage(portRef));
      return;
    }
    if (host === "analytics.localhost" && (request.url?.startsWith("/collect") || request.url?.startsWith("/beacon"))) {
      response.writeHead(204, {"access-control-allow-origin": "*"});
      response.end();
      return;
    }
    response.writeHead(404);
    response.end();
  });
  return {
    server,
    setPort(port) {
      portRef = port;
    },
  };
}

function chromePath() {
  return browserArgument();
}

async function waitForPage(debugPort, deadline) {
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`);
      if (response.ok) {
        const targets = await response.json();
        const page = targets.find((target) => target.type === "page" && typeof target.webSocketDebuggerUrl === "string");
        if (page) {
          return page.webSocketDebuggerUrl;
        }
      }
    } catch {
      // The browser may still be starting.
    }
    await delay(50);
  }
  throw new Error("browser startup timed out");
}

function browserFailure(browser) {
  return new Promise((_, reject) => {
    browser.once("error", () => reject(new Error("browser process failed")));
    browser.once("exit", (code) => {
      if (code !== null && code !== 0) {
        reject(new Error("browser process exited"));
      }
    });
  });
}

class DevTools {
  constructor(url) {
    this.url = url;
    this.nextID = 1;
    this.pending = new Map();
    this.listeners = new Map();
    this.socket = null;
  }

  async connect() {
    this.socket = new WebSocket(this.url);
    this.socket.addEventListener("message", (event) => this.message(event.data));
    this.socket.addEventListener("close", () => {
      for (const pending of this.pending.values()) {
        pending.reject(new Error("browser connection closed"));
      }
      this.pending.clear();
    });
    await new Promise((resolve, reject) => {
      this.socket.addEventListener("open", resolve, {once: true});
      this.socket.addEventListener("error", () => reject(new Error("browser connection failed")), {once: true});
    });
  }

  on(method, listener) {
    const listeners = this.listeners.get(method) || [];
    listeners.push(listener);
    this.listeners.set(method, listeners);
  }

  async command(method, params = {}) {
    const id = this.nextID++;
    const result = new Promise((resolve, reject) => this.pending.set(id, {resolve, reject}));
    this.socket.send(JSON.stringify({id, method, params}));
    return result;
  }

  message(data) {
    try {
      const text = typeof data === "string" ? data : Buffer.from(data).toString("utf8");
      const message = JSON.parse(text);
      if (message.id !== undefined) {
        const pending = this.pending.get(message.id);
        if (!pending) {
          return;
        }
        this.pending.delete(message.id);
        if (message.error) {
          pending.reject(new Error("browser command failed"));
        } else {
          pending.resolve(message.result || {});
        }
        return;
      }
      for (const listener of this.listeners.get(message.method) || []) {
        listener(message.params || {});
      }
    } catch {
      // Malformed protocol messages are ignored; the bounded run will fail closed.
    }
  }

  close() {
    if (this.socket && this.socket.readyState < 2) {
      this.socket.close();
    }
  }
}

function collector() {
  const events = new Map();
  let incomplete = false;
  let unsupported = false;

  function destination(parsed) {
    if (parsed.hostname === "app.localhost") {
      return "first-party";
    }
    if (parsed.hostname === "analytics.localhost") {
      return "analytics";
    }
    return "unknown";
  }

  function fields(parsed) {
    const result = new Set();
    let unknown = false;
    for (const key of parsed.searchParams.keys()) {
      const safeName = safeFieldNames.get(String(key).toLowerCase());
      if (safeName) {
        result.add(safeName);
      } else {
        unknown = true;
      }
    }
    if (unknown || result.size === 0) {
      result.add("unknown");
    }
    return [...result].sort();
  }

  function add(kind, value) {
    let parsed;
    try {
      parsed = new URL(value);
    } catch {
      incomplete = true;
      return;
    }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return;
    }
    const event = {
      channel: "network",
      kind,
      destination: destination(parsed),
      fields: fields(parsed),
    };
    const key = `${event.channel}\u0000${event.kind}\u0000${event.destination}`;
    const previous = events.get(key);
    if (!previous) {
      events.set(key, event);
      return;
    }
    previous.fields = [...new Set([...previous.fields, ...event.fields])].sort();
  }

  return {
    request(params) {
      if (params.redirectResponse) {
        incomplete = true;
      }
      if (params.type === "WebSocket") {
        unsupported = true;
        return;
      }
      const kind = params.type === "Ping" ? "beacon" : "request";
      if (params.request?.url) {
        add(kind, params.request.url);
      }
    },
    response(params) {
      if (params.response?.fromServiceWorker) {
        incomplete = true;
      }
      if (params.type === "WebSocket") {
        unsupported = true;
        return;
      }
      if (params.response?.url) {
        add("response", params.response.url);
      }
    },
    failed() {
      incomplete = true;
    },
    socket() {
      unsupported = true;
    },
    result() {
      return {
        incomplete: incomplete || unsupported,
        events: [...events.values()].sort((left, right) => {
          const a = `${left.channel}\u0000${left.kind}\u0000${left.destination}`;
          const b = `${right.channel}\u0000${right.kind}\u0000${right.destination}`;
          return a.localeCompare(b);
        }),
      };
    },
  };
}

async function capture(procedure) {
  if (procedure?.schema_version !== 1 || procedure?.procedure_id !== procedureID || procedure.scope !== "outbound" ||
      !Number.isInteger(procedure.duration_ms) || procedure.duration_ms < 100 ||
      procedure.duration_ms > maxDurationMS || !Number.isInteger(procedure.max_events) ||
      procedure.max_events < 1 || procedure.max_events > maxEvents) {
    throw new Error("procedure is not supported");
  }
  const deadline = Date.now() + procedure.duration_ms;
  const executable = chromePath();
  stage = "fixture";
  const fixture = fixtureServer();
  const fixturePort = await listen(fixture.server);
  fixture.setPort(fixturePort);
  stage = "chrome";
  const profile = await mkdtemp(join(tmpdir(), "ariadne-browser-fixture-"));
  const debugPort = await freePort();
  const browser = spawn(executable, [
    "--headless=new",
    "--disable-gpu",
    "--disable-extensions",
    "--disable-background-networking",
    "--disable-component-update",
    "--disable-sync",
    "--disable-crash-reporter",
    "--disable-breakpad",
    "--no-proxy-server",
    "--host-resolver-rules=MAP *.localhost 127.0.0.1, MAP * ~NOTFOUND",
    "--no-first-run",
    "--no-default-browser-check",
    `--user-data-dir=${profile}`,
    "--remote-debugging-address=127.0.0.1",
    `--remote-debugging-port=${debugPort}`,
    "about:blank",
  ], {stdio: ["ignore", "ignore", "ignore"], windowsHide: true, detached: process.platform !== "win32"});
  let devTools;
  try {
    stage = "cdp-discovery";
    const socketURL = await Promise.race([waitForPage(debugPort, deadline), browserFailure(browser)]);
    stage = "cdp-connect";
    devTools = new DevTools(socketURL);
    await devTools.connect();
    stage = "cdp-events";
    const observed = collector();
    let loadedResolve;
    const loaded = new Promise((resolve) => { loadedResolve = resolve; });
    devTools.on("Page.loadEventFired", () => loadedResolve());
    devTools.on("Network.requestWillBeSent", (params) => observed.request(params));
    devTools.on("Network.responseReceived", (params) => observed.response(params));
    devTools.on("Network.loadingFailed", () => observed.failed());
    devTools.on("Network.webSocketCreated", () => observed.socket());
    await devTools.command("Page.enable");
    await devTools.command("Network.enable");
    stage = "navigate";
    await devTools.command("Page.navigate", {url: `http://app.localhost:${fixturePort}/`});
    const remaining = Math.max(1, deadline - Date.now());
    await Promise.race([loaded, delay(Math.min(1000, remaining))]);
    if (Date.now() >= deadline) {
      throw new Error("fixture page timed out");
    }
    stage = "settle";
    await delay(Math.min(250, Math.max(0, deadline - Date.now())));
    stage = "result";
    const result = observed.result();
    if (result.events.length > procedure.max_events) {
      throw new Error("event limit exceeded");
    }
    return {
      schema_version: 1,
      redacted: true,
      scope: "outbound",
      completeness: result.incomplete ? "partial" : "complete",
      events: result.events,
    };
  } finally {
    stage = "cleanup-socket";
    devTools?.close();
    stage = "cleanup-browser";
    await stopBrowser(browser);
    stage = "cleanup-server";
    await closeServer(fixture.server);
    stage = "cleanup-profile";
    await removeProfile(profile);
  }
}

try {
  const procedure = await readProcedure();
  process.stdout.write(JSON.stringify(await capture(procedure)));
} catch {
  process.stderr.write(`browser fixture driver failed at ${stage}\n`);
  process.exitCode = 1;
}
