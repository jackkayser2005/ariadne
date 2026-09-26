import { parseProfile, origin } from "./profile.mjs";

const $ = (id) => document.getElementById(id);
const node = (tag, text) => {
  const el = document.createElement(tag);
  if (text !== undefined) el.textContent = text;
  return el;
};
let selected = "",
  currentTab = null,
  busy = false,
  selection = 0;
const showError = (error) => {
  $("error").textContent = error.message;
  $("error").hidden = false;
  $("retry").hidden = false;
};
async function request(message) {
  const result = await chrome.runtime.sendMessage(message);
  if (!result?.ok)
    throw new Error(
      result?.error || "The companion could not complete this change.",
    );
  return result.state;
}
function describe(profile, container) {
  container.append(node("h3", profile.site_origin));
  const scope = node(
    "p",
    "Blocking covers " +
      origin(profile.site_origin).hostname +
      " and its subdomains, across ports and schemes. The test used " +
      profile.site_origin +
      ".",
  );
  scope.className = "scope";
  container.append(scope);
  container.append(
    node(
      "p",
      profile.controls.location === "deny"
        ? "Location: denied at the tested address, including when embedded in another page. Embedded pages cannot bypass that denial."
        : "Location: existing browser preferences.",
    ),
  );
  const list = node("ul");
  for (const destination of profile.controls.block_origins)
    list.append(
      node("li", "Block " + destination + " and its WebSocket equivalent."),
    );
  container.append(list);
  const labels = {
    unknown: "not confirmed",
    works: "worked",
    broken: "did not work",
  };
  container.append(
    node(
      "p",
      "Test reduction: " +
        profile.test.reduction +
        ". User reports: baseline " +
        labels[profile.test.functionality.baseline] +
        ", trial " +
        labels[profile.test.functionality.treatment] +
        ". Automatic functionality verification: unknown.",
    ),
  );
  if (profile.test.functionality.treatment === "broken")
    container.append(
      node(
        "p",
        "The task did not work in this trial according to the recorded user report. Enabling may break it again.",
      ),
    );
}
async function act(message, success) {
  if (busy) return;
  busy = true;
  $("error").hidden = true;
  $("retry").hidden = true;
  document
    .querySelectorAll("button")
    .forEach((button) => (button.disabled = true));
  try {
    render(await request(message));
    $("message").textContent = success;
  } catch (error) {
    showError(error);
  } finally {
    busy = false;
    document
      .querySelectorAll("button")
      .forEach((button) => (button.disabled = false));
  }
}
function render(state) {
  $("sites").replaceChildren();
  $("empty").hidden = state.entries.length > 0;
  $("fill-panel").hidden = true;
  for (const entry of state.entries) {
    const article = node("article");
    describe(entry.profile, article);
    article.append(node("strong", entry.enabled ? "Enabled" : "Paused"));
    const toggle = node("button", entry.enabled ? "Pause" : "Enable");
    toggle.type = "button";
    toggle.addEventListener("click", () =>
      act(
        {
          action: "toggle",
          site: entry.profile.site_origin,
          enabled: !entry.enabled,
        },
        "Controls updated. Reload the site to test them.",
      ),
    );
    const undo = node("button", "Undo and remove");
    undo.type = "button";
    undo.className = "secondary";
    undo.addEventListener("click", () =>
      act(
        { action: "remove", site: entry.profile.site_origin },
        "Site controls, profile, and alias removed.",
      ),
    );
    article.append(
      node(
        "p",
        "Your alias is used only when you choose Fill in the companion on this exact site.",
      ),
    );
    const alias = node("input");
    alias.type = "text";
    alias.value = entry.alias;
    alias.maxLength = 256;
    alias.autocomplete = "off";
    alias.id = "alias-" + entry.profile.profile_sha256;
    const label = node("label", "Alias for " + entry.profile.site_origin);
    label.htmlFor = alias.id;
    const save = node("button", "Save alias");
    save.type = "button";
    save.addEventListener("click", () =>
      act(
        {
          action: "alias",
          site: entry.profile.site_origin,
          alias: alias.value,
        },
        "Alias saved locally.",
      ),
    );
    article.append(label, alias, save, node("div"));
    article.append(toggle, undo);
    $("sites").append(article);
    if (
      currentTab?.url &&
      new URL(currentTab.url).origin === entry.profile.site_origin &&
      entry.enabled &&
      entry.alias
    ) {
      $("fill-panel").hidden = false;
      $("fill-site").textContent = entry.profile.site_origin;
      $("fill").onclick = () =>
        act(
          {
            action: "fill",
            site: entry.profile.site_origin,
            tab: currentTab.id,
          },
          "Alias filled. Ariadne did not submit the form.",
        );
    }
  }
}
$("file").addEventListener("change", async () => {
  const version = ++selection;
  selected = "";
  $("preview").replaceChildren();
  $("preview").hidden = true;
  $("import").hidden = true;
  $("error").hidden = true;
  try {
    const file = $("file").files[0];
    if (!file) return;
    if (file.size > 65536)
      throw new Error("Profile files are limited to 64 KiB.");
    const text = await file.text();
    const profile = await parseProfile(text);
    if (version !== selection) return;
    selected = text;
    describe(profile, $("preview"));
    $("preview").hidden = false;
    $("import").hidden = false;
  } catch (error) {
    if (version === selection) showError(error);
  }
});
$("import").addEventListener("click", async () => {
  if (!selected) return;
  await act(
    { action: "import", text: selected },
    "Profile imported paused. Review its scope before enabling.",
  );
});
$("manage").addEventListener("click", () => chrome.runtime.openOptionsPage());
$("retry").addEventListener("click", () =>
  act({ action: "retry" }, "Saved controls reapplied."),
);
$("reset").addEventListener("click", () =>
  act({ action: "reset" }, "All Ariadne controls and local profiles removed."),
);
try {
  const own = await chrome.tabs.getCurrent();
  if (own) {
    document.body.classList.add("full");
    $("manage").hidden = true;
  } else {
    [currentTab] = await chrome.tabs.query({
      active: true,
      currentWindow: true,
    });
  }
  render(await request({ action: "state" }));
} catch (error) {
  showError(error);
}
