import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import {
  parseProfile,
  validateProfile,
  compileRules,
  origin,
} from "./profile.mjs";
import { createController } from "./controller.mjs";

export function fixture(site = "https://site.invalid") {
  const profile = {
    schema_version: 1,
    kind: "private-site-protection",
    site_origin: site,
    controls: {
      location: "deny",
      block_origins: ["https://collector.invalid"],
    },
    test: {
      baseline_journey_sha256: "a".repeat(64),
      treatment_journey_sha256: "b".repeat(64),
      comparison_sha256: "c".repeat(64),
      order: "baseline-treatment",
      baseline: { requests: 2, responses: 2, blocked: 0, websocket_frames: 0 },
      treatment: { requests: 2, responses: 0, blocked: 2, websocket_frames: 0 },
      reduction: "unknown",
      evidence_state: "unknown",
      functionality: { baseline: "works", treatment: "broken" },
      functionality_state: "claimed",
      automatic_functionality: "unknown",
    },
  };
  profile.profile_sha256 = createHash("sha256")
    .update(JSON.stringify(profile))
    .digest("hex");
  return profile;
}
function resign(profile) {
  delete profile.profile_sha256;
  profile.profile_sha256 = createHash("sha256")
    .update(JSON.stringify(profile))
    .digest("hex");
  return profile;
}
function fakeAPI() {
  let protection,
    rules = [],
    locations = [];
  const calls = [];
  let fail = "";
  const check = (method) => {
    calls.push(method);
    if (fail === method) {
      fail = "";
      throw new Error("Injected browser failure");
    }
  };
  const api = {
    storage: {
      local: {
        get: async () => ({ protection: structuredClone(protection) }),
        set: async (value) => {
          check("storage");
          protection = structuredClone(value.protection);
        },
        setAccessLevel: async () => {},
      },
    },
    declarativeNetRequest: {
      RuleConditionKeys: { TOP_DOMAINS: "topDomains" },
      getDynamicRules: async () => structuredClone(rules),
      updateDynamicRules: async ({ addRules = [] }) => {
        check("rules");
        rules = structuredClone(addRules);
      },
    },
    contentSettings: {
      location: {
        clear: async () => {
          check("clear");
          locations = [];
        },
        set: async (value) => {
          check("location");
          locations.push(value);
        },
      },
    },
    scripting: { executeScript: async () => [{ result: true }] },
  };
  return {
    api,
    get rules() {
      return rules;
    },
    get locations() {
      return locations;
    },
    get state() {
      return protection;
    },
    get calls() {
      return calls;
    },
    failNext(method) {
      fail = method;
    },
  };
}

test("strict bounded profiles preserve uncertainty and reject misleading or malformed receipts", async () => {
  const profile = fixture();
  assert.deepEqual(
    await parseProfile(JSON.stringify(profile, null, 2)),
    profile,
  );
  for (const text of [
    "{}",
    "{",
    " ".repeat(65537),
    "[".repeat(17) + "]".repeat(17),
    JSON.stringify(profile).replace(
      '"schema_version":1',
      '"schema_version":1,"schema_version":1',
    ),
    JSON.stringify(profile).replace(
      '"kind":',
      '"k\\u0069nd":"duplicate","kind":',
    ),
  ])
    await assert.rejects(parseProfile(text));
  for (const mutate of [
    (p) => (p.extra = true),
    (p) => (p.schema_version = 2),
    (p) => (p.controls.location = "approximate"),
    (p) => (p.controls.block_origins = null),
    (p) => p.controls.block_origins.push("https://collector.invalid"),
    (p) => (p.site_origin = "https://*.invalid"),
    (p) => (p.test.functionality_state = "observed"),
    (p) => (p.test.automatic_functionality = "sufficient"),
    (p) => (p.test.baseline.requests = -1),
    (p) => (p.test.baseline.blocked = 3),
    (p) => (p.test.reduction = "reduced"),
    (p) => (p.test.evidence_state = "claimed"),
    (p) => (p.test.functionality.baseline = "verified"),
    (p) => (p.test.comparison_sha256 = "not-a-hash"),
  ]) {
    const changed = fixture();
    mutate(changed);
    await assert.rejects(validateProfile(resign(changed)));
  }
  const tampered = fixture();
  tampered.controls.location = "unchanged";
  await assert.rejects(validateProfile(tampered), /integrity/);
  const observed = fixture();
  observed.test.evidence_state = "observed";
  observed.test.reduction = "reduced";
  await validateProfile(resign(observed));
  observed.test.reduction = "not-reduced";
  await assert.rejects(validateProfile(resign(observed)), /disagrees/);
  for (const address of [
    "https://user:password@site.invalid",
    "file:///private",
    "https://site.invalid/path",
    "https://site.invalid:443",
    "https://*.invalid",
    "https://bücher.invalid",
  ])
    assert.throws(() => origin(address));
  assert.equal(
    origin("https://xn--bcher-kva.invalid").hostname,
    "xn--bcher-kva.invalid",
  );
});

test("blocking is anchored to exact destination and top-level site including WebSockets", () => {
  const profile = fixture();
  const rules = compileRules([{ profile, enabled: true }]);
  assert.deepEqual(
    rules.map((r) => r.condition.urlFilter),
    ["|https://collector.invalid/", "|wss://collector.invalid/"],
  );
  for (const rule of rules)
    assert.deepEqual(rule.condition.topDomains, ["site.invalid"]);
  assert.equal(compileRules([{ profile, enabled: false }]).length, 0);
  for (const site of ["https://site.invalid:8443", "https://a.site.invalid"])
    assert.throws(
      () =>
        compileRules([
          { profile, enabled: true },
          { profile: fixture(site), enabled: true },
        ]),
      /overlap/,
    );
});

test("import starts paused; enable, restart, pause and undo retain site isolation", async () => {
  const fake = fakeAPI();
  let controller = createController(fake.api);
  await controller.recover();
  for (const site of ["https://site.invalid", "https://other.invalid"])
    await controller.handle({
      action: "import",
      text: JSON.stringify(fixture(site)),
    });
  assert.equal(fake.rules.length, 0);
  assert.equal(fake.locations.length, 0);
  for (const site of ["https://site.invalid", "https://other.invalid"])
    await controller.handle({ action: "toggle", site, enabled: true });
  assert.equal(fake.rules.length, 4);
  assert.equal(fake.locations.length, 2);
  await controller.handle({
    action: "alias",
    site: "https://site.invalid",
    alias: "my-alias@example.invalid",
  });
  controller = createController(fake.api);
  await controller.recover();
  assert.equal(fake.rules.length, 4);
  assert.equal(fake.state.entries[0].alias, "my-alias@example.invalid");
  await controller.handle({
    action: "toggle",
    site: "https://site.invalid",
    enabled: false,
  });
  assert.deepEqual(
    fake.rules.map((r) => r.condition.topDomains),
    [["other.invalid"], ["other.invalid"]],
  );
  assert.equal(fake.locations[0].primaryPattern, "https://other.invalid/*");
  assert.equal(fake.locations[0].secondaryPattern, undefined);
  await assert.rejects(
    controller.handle({ action: "fill", site: "https://site.invalid", tab: 1 }),
  );
  await controller.handle({ action: "remove", site: "https://site.invalid" });
  assert.equal(fake.state.entries.length, 1);
  assert.equal(JSON.stringify(fake.state).includes("my-alias"), false);
  await controller.handle({ action: "remove", site: "https://other.invalid" });
  assert.deepEqual(fake.rules, []);
  assert.deepEqual(fake.locations, []);
  assert.deepEqual(fake.state.entries, []);
});

test("failed browser mutations roll back and overlapping controls cannot silently broaden scope", async () => {
  const fake = fakeAPI();
  const controller = createController(fake.api);
  await controller.recover();
  await controller.handle({
    action: "import",
    text: JSON.stringify(fixture()),
  });
  for (const failure of ["rules", "location"]) {
    fake.failNext(failure);
    await assert.rejects(
      controller.handle({
        action: "toggle",
        site: "https://site.invalid",
        enabled: true,
      }),
      /Injected/,
    );
    assert.equal(fake.state.entries[0].enabled, false);
    assert.equal(fake.rules.length, 0);
    assert.equal(fake.locations.length, 0);
  }
  delete fake.api.declarativeNetRequest.RuleConditionKeys;
  await assert.rejects(
    controller.handle({
      action: "toggle",
      site: "https://site.invalid",
      enabled: true,
    }),
    /145/,
  );
  assert.equal(fake.rules.length, 0);
  fake.api.declarativeNetRequest.RuleConditionKeys = {
    TOP_DOMAINS: "topDomains",
  };
  await controller.handle({
    action: "toggle",
    site: "https://site.invalid",
    enabled: true,
  });
  await controller.handle({
    action: "import",
    text: JSON.stringify(fixture("https://a.site.invalid")),
  });
  await assert.rejects(
    controller.handle({
      action: "toggle",
      site: "https://a.site.invalid",
      enabled: true,
    }),
    /overlap/,
  );
  assert.equal(fake.rules.length, 2);
  await assert.rejects(
    controller.handle({ action: "import", text: JSON.stringify(fixture()) }),
    /already/,
  );
  for (const alias of ["x".repeat(257), "bad\nvalue"])
    await assert.rejects(
      controller.handle({
        action: "alias",
        site: "https://site.invalid",
        alias,
      }),
    );
  await controller.handle({ action: "reset" });
  assert.deepEqual(fake.state.entries, []);
  assert.equal(fake.rules.length, 0);
  assert.equal(fake.locations.length, 0);
});

test("manifest has no capture, network upload, or broad page-reading permission", async () => {
  const manifest = JSON.parse(
    await readFile(new URL("./manifest.json", import.meta.url), "utf8"),
  );
  assert.deepEqual(manifest.permissions, [
    "storage",
    "declarativeNetRequest",
    "contentSettings",
    "activeTab",
    "scripting",
  ]);
  assert.equal(manifest.host_permissions, undefined);
  assert.equal(manifest.content_scripts, undefined);
  assert.equal(manifest.externally_connectable, undefined);
  assert.match(
    manifest.content_security_policy.extension_pages,
    /connect-src 'none'/,
  );
});
