---
title: Guided website investigation
weight: 5
---

Run `ariadne investigate https://example.com` to open the local guide. Ariadne
finds installed Chrome or Edge automatically. Start recording to open a separate,
visible browser with a fresh temporary profile. Only investigate software and
data you are authorized to test.

1. Choose a website and select **Start investigation**.
2. Browse in the recording window. Select a text field, return to the guide,
   and use **Fill test email** or **Fill test account ID**. You can also copy
   a generated test value. Passwords, files, and non-text controls cannot be filled.
3. Select **Stop and review** to close the recording browser and save evidence.
   **Cancel and discard** closes it without saving. Closing the local server
   also ends recording and cleans up its temporary profile.
4. Read the information cards and visibility gaps. Expand the timeline for
   ordered observations and the evidence disclosure for technical details.
5. Select **Export verified evidence** to download a checked, portable JSON file.

If saving fails after recording, keep the local server open: Ariadne retains the
stopped evidence for **Export verified evidence** or **Retry saving**. Restore the
output directory before retrying. **Cancel and discard** releases that retained
recording when you no longer need it.

Recording is bounded to 20 minutes, 2,048 observations, and 64 interaction steps.
An additional cumulative limit of 8 MiB of protocol event data or 8,192 events
ends recording even if a page paces large payloads below the transport limit.
Navigation outside the chosen origin and downloads are blocked. Subresources,
frames, and supported workers may contact other origins so their destinations
can be observed. Clicks and submissions remain manual checkpoints; Ariadne does
not automatically submit forms or replay purchases and other consequential actions.

## Read the evidence

The recorder looks for generated markers in supported form-input events,
script-visible cookies and storage, worker messages, fetch/XHR, beacons, requests,
redirects, and text WebSocket frames. Matching supports exact values, URL encoding,
Base64, and SHA-256. A matching representation is an observation, not proof of who
transformed or transferred a value. Relationships in the timeline are inferred.

A request observation records an attempt. A linked **blocked** observation records
browser enforcement. A response references a request without claiming that its
body contained the marker. No browser observation proves what a server retained.

Hooks are page-owned and cannot attest their own completeness. Worker startup,
unsupported or encrypted bodies, browser crashes, overflow, and other missing
visibility stay explicit. Current recordings therefore remain partial; absent
matches never establish that no information was shared. The instrumentation
approach is informed by [OpenWPM](https://openwpm.readthedocs.io/en/stable/apidoc/openwpm.config.html)
and [Leaky Forms](https://www.usenix.org/conference/usenixsecurity22/presentation/senol).

Raw payloads are matched transiently. Saved `journey.json` retains only bounded
observations, category labels, destination aliases, references, inferred links,
and gaps. `trace.json` preserves trace-v1 semantics. `receipt.json` binds both
identities. Private destination names and structural interaction steps are stored
separately in `private-context.json`, with a separate integrity receipt. Share
the exported JSON, not the private investigation directory. Hashes check
consistency; they are not signatures or independent proof of source truth.

## Try a privacy control

After saving, **Try sharing less** starts another fresh recording with selected
destination blocks, location denial, or a fixed synthetic approximate location.
Repeat the same task using the same markers. The approximate location is a lab
input, not an approximation of your actual location. Persistent protection and
verified baseline/treatment comparisons are later milestones; the current guide
keeps each trial's observations and uncertainty visible without claiming success.

## Terminal access

```console
ariadne investigate --no-open https://example.com
ariadne investigate --duration 30s --location deny --block-origin https://collector.example --output .ariadne/my-trial https://example.com
ariadne inspect .ariadne/my-trial
ariadne inspect --no-open ariadne-evidence.json
ariadne inspect --json ariadne-evidence.json
```

Put flags before the website or bundle argument. `--no-open` prints the loopback
interface URL; `--duration` records from the terminal and prints synthetic inputs.
Ctrl+C cancels a timed recording without saving. Output directories must be new.
`inspect` verifies saved content before opening a read-only view; `--json` emits
only portable evidence. Existing weather, trace, Android, and evidence-review
commands retain their original behavior.

Capture controls run on a separate loopback handler with a per-session token and
exact host/origin checks. The bootstrap token is in the URL fragment, not a query
string. Keep that local session link private. Existing evidence-review routes do
not gain write access.

## Development checks

Full coverage includes headless tests against local fixtures in installed Chrome
or Edge. Set `ARIADNE_BROWSER_TESTS=1` before running
`go test -race -covermode=atomic -coverprofile=coverage.out ./...`.
CI requires these tests on Linux and repeats the guided acceptance suite on Windows.
Transport, parser, and hostile-payload tests also run without a browser.
