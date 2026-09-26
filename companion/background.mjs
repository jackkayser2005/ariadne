import { createController } from "./controller.mjs";

const controller = createController(chrome);
let startupError = "";
const ready = controller.recover().catch((error) => {
  startupError = error.message;
});
// These listeners wake the worker so an interrupted settings change is reconciled.
chrome.runtime.onStartup.addListener(() => {
  void ready;
});
chrome.runtime.onInstalled.addListener(() => {
  void ready;
});
chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (
    sender.id !== chrome.runtime.id ||
    sender.url !== chrome.runtime.getURL("protection.html")
  )
    return false;
  ready
    .then(async () => {
      if (startupError && !["reset", "retry"].includes(message?.action))
        throw new Error(startupError);
      const state =
        message?.action === "retry"
          ? await controller.recover()
          : await controller.handle(message);
      if (["reset", "retry"].includes(message.action)) startupError = "";
      return { ok: true, state };
    })
    .then(respond, (error) => respond({ ok: false, error: error.message }));
  return true;
});
