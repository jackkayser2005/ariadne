const fail = (message) => {
  throw new Error(message);
};
const keys = (value, names) => {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !== [...names].sort().join(",")
  )
    fail(
      "Profile fields are invalid. Export a new private profile from Ariadne.",
    );
};
const digest = (value) =>
  typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
const oneOf = (value, values) => {
  if (!values.includes(value))
    fail("The profile contains an unsupported result or control.");
  return value;
};

export function origin(value) {
  if (typeof value !== "string" || value.length > 4096)
    fail("The profile contains an invalid site address.");
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    fail("The profile contains an invalid site address.");
  }
  if (
    !["http:", "https:"].includes(parsed.protocol) ||
    parsed.origin !== value ||
    !/^[\x21-\x7e]+$/.test(value)
  )
    fail(
      "Site addresses must be canonical HTTP(S) origins. Use punycode for international domain names.",
    );
  if (
    !/^(?:\[[0-9a-f:.]+\]|[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?\.?)$/.test(
      parsed.hostname,
    )
  )
    fail("Site addresses cannot contain request-rule pattern characters.");
  return parsed;
}

function strictJSON(text) {
  if (typeof text !== "string" || new TextEncoder().encode(text).length > 65536)
    fail("Profile files are limited to 64 KiB.");
  let result;
  try {
    result = JSON.parse(text);
  } catch {
    fail("The file is not valid JSON.");
  }
  const stack = [];
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '"') {
      const start = i++;
      while (i < text.length && text[i] !== '"') {
        if (text[i] === "\\") i++;
        i++;
      }
      let next = i + 1;
      while (/\s/.test(text[next] || "") && next < text.length) next++;
      if (text[next] === ":") {
        const key = JSON.parse(text.slice(start, i + 1));
        const seen = stack.at(-1);
        if (!seen || seen.has(key))
          fail("Duplicate profile fields are not allowed.");
        seen.add(key);
      }
    } else if (text[i] === "{" || text[i] === "[") {
      stack.push(text[i] === "{" ? new Set() : null);
      if (stack.length > 16) fail("The profile is nested too deeply.");
    } else if (text[i] === "}" || text[i] === "]") stack.pop();
  }
  return result;
}

function counts(value) {
  keys(value, ["requests", "responses", "blocked", "websocket_frames"]);
  for (const number of Object.values(value))
    if (!Number.isSafeInteger(number) || number < 0 || number > 2048)
      fail("Profile observation counts are invalid.");
  if (value.responses + value.blocked > value.requests)
    fail("Profile observation counts contradict each other.");
  return {
    requests: value.requests,
    responses: value.responses,
    blocked: value.blocked,
    websocket_frames: value.websocket_frames,
  };
}

// Match the Go profile's fixed field order. This checks consistency, not a signature.
export async function validateProfile(profile) {
  keys(profile, [
    "schema_version",
    "kind",
    "site_origin",
    "controls",
    "test",
    "profile_sha256",
  ]);
  if (
    profile.schema_version !== 1 ||
    profile.kind !== "private-site-protection" ||
    !digest(profile.profile_sha256)
  )
    fail("This is not a supported private protection profile.");
  origin(profile.site_origin);
  keys(profile.controls, ["location", "block_origins"]);
  const location = oneOf(profile.controls.location, ["unchanged", "deny"]);
  const blocked = profile.controls.block_origins;
  if (
    !Array.isArray(blocked) ||
    blocked.length > 64 ||
    (!blocked.length && location === "unchanged")
  )
    fail(
      "The profile has no supported persistent controls. Approximate location remains lab-only.",
    );
  blocked.forEach((value, i) => {
    origin(value);
    if (i && blocked[i - 1] >= value)
      fail("Blocked destinations must be sorted and unique.");
  });
  const test = profile.test;
  keys(test, [
    "baseline_journey_sha256",
    "treatment_journey_sha256",
    "comparison_sha256",
    "order",
    "baseline",
    "treatment",
    "reduction",
    "evidence_state",
    "functionality",
    "functionality_state",
    "automatic_functionality",
  ]);
  for (const key of [
    "baseline_journey_sha256",
    "treatment_journey_sha256",
    "comparison_sha256",
  ])
    if (!digest(test[key])) fail("The test receipt is invalid.");
  const order = oneOf(test.order, ["baseline-treatment", "treatment-baseline"]);
  const baseline = counts(test.baseline),
    treatment = counts(test.treatment);
  const state = oneOf(test.evidence_state, ["unknown", "observed"]);
  const reduction = oneOf(
    test.reduction,
    state === "unknown" ? ["unknown"] : ["reduced", "not-reduced"],
  );
  if (
    state === "observed" &&
    treatment.requests - treatment.blocked + treatment.websocket_frames <
      baseline.requests - baseline.blocked + baseline.websocket_frames !==
      (reduction === "reduced")
  )
    fail("The reduction disagrees with its observations.");
  keys(test.functionality, ["baseline", "treatment"]);
  const functionality = {
    baseline: oneOf(test.functionality.baseline, [
      "unknown",
      "works",
      "broken",
    ]),
    treatment: oneOf(test.functionality.treatment, [
      "unknown",
      "works",
      "broken",
    ]),
  };
  const functionalityState = Object.values(functionality).includes("unknown")
    ? "unknown"
    : "claimed";
  if (
    test.functionality_state !== functionalityState ||
    test.automatic_functionality !== "unknown"
  )
    fail("A user report cannot become automatic functionality verification.");
  const canonical = {
    schema_version: 1,
    kind: "private-site-protection",
    site_origin: profile.site_origin,
    controls: { location, block_origins: [...blocked] },
    test: {
      baseline_journey_sha256: test.baseline_journey_sha256,
      treatment_journey_sha256: test.treatment_journey_sha256,
      comparison_sha256: test.comparison_sha256,
      order,
      baseline,
      treatment,
      reduction,
      evidence_state: state,
      functionality,
      functionality_state: functionalityState,
      automatic_functionality: "unknown",
    },
  };
  const data = new TextEncoder().encode(JSON.stringify(canonical));
  const hash = Array.from(
    new Uint8Array(await crypto.subtle.digest("SHA-256", data)),
    (n) => n.toString(16).padStart(2, "0"),
  ).join("");
  if (hash !== profile.profile_sha256)
    fail("Profile integrity check failed. Export the profile again.");
  return { ...canonical, profile_sha256: hash };
}

export async function parseProfile(text) {
  return validateProfile(strictJSON(text));
}

export function compileRules(entries) {
  const active = entries.filter((entry) => entry.enabled);
  const hosts = active.map(
    (entry) => origin(entry.profile.site_origin).hostname,
  );
  for (let i = 0; i < hosts.length; i++)
    for (let j = i + 1; j < hosts.length; j++) {
      if (
        hosts[i] === hosts[j] ||
        hosts[i].endsWith("." + hosts[j]) ||
        hosts[j].endsWith("." + hosts[i])
      )
        fail(
          "Enabled profiles would overlap this host or its subdomains. Pause the other profile first.",
        );
    }
  const rules = [];
  for (const entry of active.sort((a, b) =>
    a.profile.site_origin.localeCompare(b.profile.site_origin),
  )) {
    for (const destination of entry.profile.controls.block_origins) {
      for (const url of [destination, destination.replace(/^http/, "ws")]) {
        rules.push({
          id: rules.length + 1,
          priority: 1,
          action: { type: "block" },
          condition: {
            urlFilter: "|" + url + "/",
            isUrlFilterCaseSensitive: true,
            topDomains: [origin(entry.profile.site_origin).hostname],
          },
        });
      }
    }
  }
  return rules;
}
