---
title: Everyday browser protection
weight: 6
---

The optional Ariadne Companion applies controls from a private profile exported
by a [paired investigation](../guided-investigation/). It runs locally in Chrome
or Edge, keeps profiles and aliases in that browser's local storage, and does
not capture browsing or upload data. Ariadne's local investigation server does
not need to remain running.

## Install and import

This first version is distributed as an unpacked extension. Use the `companion`
directory from the repository, or extract the `ariadne-companion.zip` artifact
from a successful **Browser fixture** workflow run. The archive contains only
the seven runtime files; Node.js and the test dependencies are not needed to
use the extension.

1. Open `chrome://extensions` or `edge://extensions`, enable **Developer mode**,
   choose **Load unpacked**, and select the directory containing `manifest.json`.
2. Open Ariadne from the browser's extension menu. **Open full page** gives the
   controls more room.
3. After a paired investigation, use **Save private protection profile** in
   Ariadne. In the companion, choose that file with **Profile file**.
4. Review the site, destinations, scope, results, and task reports. Select
   **Import paused**. Importing does not enable any controls.
5. Select **Enable** for that site, reload it, and try the task again. **Pause**
   stops its controls while retaining the profile and alias. **Undo and remove**
   stops its controls and deletes its local profile and alias.

The profile is private: it contains site and destination addresses. Share
Ariadne's portable evidence export instead. Its integrity receipt checks file
consistency, not a signature or an independent attestation of the original
captures. A reduction marked **unknown** stays unknown. User reports remain
separate from automatic functionality verification, including reports that the
trial broke the task.

## What the controls cover

Destination blocking uses browser-native
[declarative request rules](https://developer.chrome.com/docs/extensions/reference/api/declarativeNetRequest).
It covers the tested top-level host and its subdomains across ports and schemes.
Requests without a top-level page use their initiating host. Each destination
rule covers its exact origin and corresponding WebSocket origin. This broader
site scope is shown before enabling. Overlapping enabled site profiles are
rejected so pausing one cannot silently leave another profile in control.

Location denial uses the browser's native content setting for the exact tested
origin, including when that origin is embedded elsewhere. Frames within a denied
page cannot bypass its denial. Ariadne removes only its own settings when paused
or undone; existing browser preferences then apply again. Approximate location
remains a synthetic lab experiment and cannot be imported for everyday use.

Blocking requires Chrome or Edge 145 or newer and the native `topDomains`
capability. Unsupported browsers show an error when enabling; controls are not
silently broadened. Profiles are bounded to 64 KiB, 64 destinations, and 32
imported sites. Malformed, ambiguous, tampered, or unsupported files are rejected
before they change browser settings.

## Fill an alias explicitly

Save an alias of your choosing in the site's card. Focus an editable text field
in that site's main page, reopen the companion from the toolbar, and choose
**Fill alias in selected field**. Ariadne requires the exact tested origin and
an enabled profile. Password, file, disabled, read-only, and embedded-frame
fields are excluded. It does not generate an alias, fill automatically, or click
Submit; sites can react immediately to edited fields.

Profiles, aliases, destination rules, and location controls survive browser
restarts. **Undo all controls and delete local profiles** removes every
companion-owned rule and setting. Reload pages after changing controls. Undo
cannot recall information already sent. If a browser API fails, Ariadne attempts
to restore the previous state and reports any restoration failure explicitly;
the browser's extension manager can always disable or remove the companion.

## Development checks

The runtime files have no third-party dependencies. Node.js and the pinned
Playwright dependency are used only by the acceptance tests:

```console
cd companion
npm ci --ignore-scripts
npx --no-install playwright install chromium --no-shell
node --test controller.test.mjs acceptance.test.mjs
```

Set `ARIADNE_COMPANION_CHANNEL=msedge` to test installed Edge, or
`ARIADNE_COMPANION_HEADED=1` to watch the native browser flow. CI tests both
Chrome for Testing and Edge on Windows. Local fixtures cover file-picker import,
malformed/oversized/tampered files, keyboard import, main-page/frame/worker and
WebSocket blocking, location isolation, explicit alias filling, narrow layout,
restart persistence, pause, and complete undo. Each run owns and removes its
temporary browser profile. No personal browser profile is used.
