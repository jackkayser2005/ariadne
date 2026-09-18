---
title: Website location investigation
weight: 4
---

Ariadne's weather slice is a fixed, scripted investigation of the National
Weather Service beta site. It uses one fresh Chrome profile per session and
records only reviewed labels and SHA-256 identities. The site is live and can
change, so workflow failure or missing visibility stays `unknown`. The local review page explains the procedure-specific `weather-service` and `undeclared` labels as Weather service and Blocked destination while preserving their stable IDs.

## Run it

Use an absolute Node executable as the driver and pass the reviewed weather
driver plus an absolute Chrome executable:

~~~console
go run ./cmd/ariadne browser weather --json \
  --driver "C:\Program Files\nodejs\node.exe" \
  --driver-arg cmd/browser-fixture-driver/weather_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\Program Files\Google\Chrome\Application\chrome.exe" \
  --output .ariadne/runs/weather-location

go run ./cmd/ariadne browser weather verify --json \
  --expect-sha256 <receipt-sha256> .ariadne/runs/weather-location
~~~

The runner executes eight sessions in a fixed order: precise, coarse, coarse,
precise, precise, denied, denied, precise. The precise and city-center values
are synthetic. The driver allows only `https://beta.weather.gov`, blocks other
origins, bounds request bodies and events, and removes the temporary profile
before returning. Unsupported encodings are recorded as an explicit gap. It
does not accept arbitrary URLs, scripts, selectors, headers, payloads, or
coordinates.

Start the local read-only review page after verification:

~~~console
go run ./cmd/ariadne experiment serve --addr 127.0.0.1:8787 \
  --weather .ariadne/runs/weather-location .ariadne/ui-archive
~~~

Keep the final archive-root argument separate from the weather output. It may be an empty directory when you are reviewing only this investigation; weather artifacts are not archive bundles.

Open `http://127.0.0.1:8787/weather`. The page re-verifies the run and its
portable traces on every request. It renders the candidate comparison,
functionality classification, attempted and response-backed labels, visibility
gaps, and retained identities. It never renders coordinates, URLs, request
bodies, browser paths, or local artifact paths.

The JSON result also includes `candidate_evidence`, derived while the bundle is
reverified. Each candidate reports session counts for forecast available,
forecast unavailable, forecast unknown, attempted supported matches, and
response-backed supported matches. Its `visibility_gaps` list contains each
distinct validated gap with a fixed explanation. The non-JSON CLI summary also
shows a short browser -> Location -> Weather service trail before the detailed
counts. This is presentation data:
it is not written into `weather.json` and does not change the receipt identity
or conservative ladder selection.

## Recorded live result

The latest acceptance run on 2026-09-16 completed all eight sessions. Precise and
coarse sessions rendered the fixed local forecast; denied sessions were
unavailable. Matching requests were observed as attempted and
response-backed to the reviewed weather service. The run also recorded blocked
origins, capture-incomplete, unsupported-channel, and server-side-unobservable
gaps. Because those gaps affect every candidate pair, the conservative ladder
left the selection `unknown` and selected no minimum disclosure.

The independently verified run receipt was
`7288ae2f0dac3b73350a97a438223af412e37bacc5968204abae442f44e7580b`. The
portable bundle contains no real coordinates, URLs, payloads, or personal data.

`geolocation` in a session records the browser permission configured for that
session. It does not prove that the page invoked the geolocation API. Network
observations and rendered forecast availability are reported separately. The
driver cannot observe server-side onward sharing, unsupported encodings, unsupported
channels, or weather-service internals; those limits remain explicit gaps.

This is a bounded website procedure and a local evidence view. It is not a
universal browser monitor, a causal proof, or a full repository security audit.
## Reading the result

The review starts with plain-language observations and a browser → location →
website explanation. Precise, city-level, and denied-location cards show how
often a forecast appeared. Missing visibility is explained separately from
those observations. Detailed counts, sessions, and verification identities are
collapsed under **Explore the technical evidence**. The page never interprets
a working forecast as proof that location sharing is safe, or missing location
matches as proof of privacy.

The archive landing page puts this guided example first when configured. A short
glossary explains attempted requests, responses, unknowns, and verification.
Broader website capture import is still pending; the local review does not
monitor personal browsing, messages, or other apps.
