# Android Fixture

`dev.ariadne.fixture` is the authorized target for Experiment 001. The runner
writes one bounded, canonical `ariadne-input.json` document through `adb
exec-in` stdin into the app-private files area, then starts the
`android.permission.DUMP`-protected `MainActivity` through the ADB shell,
without persona or collector-port extras. The activity consumes
and deletes that document once, renders one `Run observation` button, and the
runner taps that declared control before the activity writes
`files/observation.json` and posts the same observation to
`http://127.0.0.1:<port>/observe`.

The input contains a per-session challenge, role, order, procedure digest,
collector port, and persona. Duplicate keys, reordered or whitespace-padded
JSON, unknown fields, unsafe values, stale shape, and oversized input are
rejected. The challenge is included in local network/storage observations so
the runner can require the two evidence channels to agree; reports, traces,
logs, and portable receipts retain only the challenge commitment or omit it.

The fixture-only `capture_mode=treatment_network_only` control leaves baseline
behavior unchanged but omits treatment storage after sending the treatment
network observation. `examples/experiment-001-storage-gap.json` uses this mode
to verify incomplete-capture reporting. Do not use it as an application
behavior claim.

Build and test:

```console
./gradlew testDebugUnitTest createDebugUnitTestCoverageReport assembleDebug
```

The debug APK is written to `app/build/outputs/apk/debug/app-debug.apk`.
The fixture requests only Android's normal `INTERNET` permission. Its network
security configuration denies cleartext traffic except to IPv4 loopback.
