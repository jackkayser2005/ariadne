import {
  compileRules,
  origin,
  parseProfile,
  validateProfile,
} from "./profile.mjs";

const empty = () => ({ schema_version: 1, entries: [] });
const locationOrigins = (state) =>
  state.entries
    .filter((e) => e.enabled && e.profile.controls.location === "deny")
    .map((e) => e.profile.site_origin)
    .sort();

export function fillSelectedAlias(site, value) {
  if (location.origin !== site) return false;
  const field = document.activeElement;
  if (
    !field ||
    field.disabled ||
    field.readOnly ||
    !(
      field instanceof HTMLTextAreaElement ||
      (field instanceof HTMLInputElement &&
        ["text", "email", "url", "tel", "search"].includes(field.type))
    )
  )
    return false;
  const prototype =
    field instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value").set.call(field, value);
  field.dispatchEvent(new Event("input", { bubbles: true }));
  field.dispatchEvent(new Event("change", { bubbles: true }));
  return true;
}

export function createController(api) {
  const setting = api.contentSettings.location;
  let chain = Promise.resolve();
  const serial = (operation) => {
    const task = chain.then(operation);
    chain = task.catch(() => {});
    return task;
  };
  async function read() {
    const stored = (await api.storage.local.get("protection")).protection;
    if (stored === undefined) return empty();
    if (
      stored?.schema_version !== 1 ||
      !Array.isArray(stored.entries) ||
      stored.entries.length > 32
    )
      throw new Error(
        "Saved companion settings are invalid. Undo all controls to reset them.",
      );
    const sites = new Set();
    const entries = [];
    for (const entry of stored.entries) {
      const profile = await validateProfile(entry.profile);
      if (
        typeof entry.enabled !== "boolean" ||
        typeof entry.alias !== "string" ||
        entry.alias.length > 256 ||
        /[\x00-\x1f\x7f]/.test(entry.alias) ||
        sites.has(profile.site_origin)
      )
        throw new Error(
          "Saved companion settings are invalid. Undo all controls to reset them.",
        );
      sites.add(profile.site_origin);
      entries.push({ profile, enabled: entry.enabled, alias: entry.alias });
    }
    return { schema_version: 1, entries };
  }
  async function apply(state, previous) {
    const rules = compileRules(state.entries);
    if (
      rules.length &&
      !Object.values(
        api.declarativeNetRequest.RuleConditionKeys || {},
      ).includes("topDomains")
    )
      throw new Error(
        "This browser cannot enforce site-scoped blocking. Use Chrome or Edge 145 or newer.",
      );
    const current = await api.declarativeNetRequest.getDynamicRules();
    await api.declarativeNetRequest.updateDynamicRules({
      removeRuleIds: current.map((rule) => rule.id),
      addRules: rules,
    });
    const nextLocations = locationOrigins(state);
    if (
      !previous ||
      JSON.stringify(nextLocations) !==
        JSON.stringify(locationOrigins(previous))
    ) {
      if (!setting) {
        if (nextLocations.length)
          throw new Error("Location control is unavailable in this browser.");
      } else {
        await setting.clear({ scope: "regular" });
        for (const site of nextLocations)
          await setting.set({
            primaryPattern: site + "/*",
            setting: "block",
            scope: "regular",
          });
      }
    }
  }
  async function update(previous, next) {
    compileRules(next.entries); // Reject overlap before touching stored settings or browser rules.
    await api.storage.local.set({ protection: next });
    try {
      await apply(next, previous);
    } catch (error) {
      try {
        await api.storage.local.set({ protection: previous });
        await apply(previous);
      } catch {
        throw new Error(
          "The change failed and restoration could not be verified. Disable Ariadne in the browser extension manager to remove its controls.",
        );
      }
      throw error;
    }
    return next;
  }
  return {
    recover: () =>
      serial(async () => {
        await api.storage.local.setAccessLevel({
          accessLevel: "TRUSTED_CONTEXTS",
        });
        const state = await read();
        await apply(state);
        return state;
      }),
    handle: (message) =>
      serial(async () => {
        if (!message || typeof message.action !== "string")
          throw new Error("Unsupported companion action.");
        if (message.action === "reset") {
          const current = await api.declarativeNetRequest.getDynamicRules();
          await api.declarativeNetRequest.updateDynamicRules({
            removeRuleIds: current.map((r) => r.id),
          });
          if (setting) await setting.clear({ scope: "regular" });
          await api.storage.local.set({ protection: empty() });
          return empty();
        }
        const previous = await read();
        if (message.action === "state") return previous;
        const next = structuredClone(previous);
        if (message.action === "import") {
          const profile = await parseProfile(message.text);
          if (
            next.entries.some(
              (e) => e.profile.site_origin === profile.site_origin,
            )
          )
            throw new Error(
              "This site already has a profile. Undo it before importing a replacement.",
            );
          if (next.entries.length >= 32)
            throw new Error(
              "Remove a profile before importing more than 32 sites.",
            );
          next.entries.push({ profile, enabled: false, alias: "" });
        } else {
          origin(message.site);
          const entry = next.entries.find(
            (e) => e.profile.site_origin === message.site,
          );
          if (!entry) throw new Error("This site has no imported profile.");
          if (message.action === "toggle") {
            if (typeof message.enabled !== "boolean")
              throw new Error("Choose enable or pause.");
            entry.enabled = message.enabled;
          } else if (message.action === "remove")
            next.entries = next.entries.filter((e) => e !== entry);
          else if (message.action === "alias") {
            if (
              typeof message.alias !== "string" ||
              message.alias.length > 256 ||
              /[\x00-\x1f\x7f]/.test(message.alias)
            )
              throw new Error(
                "Use an alias of at most 256 characters without control characters.",
              );
            entry.alias = message.alias;
          } else if (message.action === "fill") {
            if (
              !entry.enabled ||
              !entry.alias ||
              !Number.isSafeInteger(message.tab)
            )
              throw new Error(
                "Enable this site and save an alias before filling.",
              );
            const result = await api.scripting.executeScript({
              target: { tabId: message.tab, frameIds: [0] },
              func: fillSelectedAlias,
              args: [entry.profile.site_origin, entry.alias],
            });
            if (result.length !== 1 || result[0].result !== true)
              throw new Error(
                "Select an editable text field in the tested site’s main page, then fill again.",
              );
            return previous;
          } else throw new Error("Unsupported companion action.");
        }
        return update(previous, next);
      }),
  };
}
