# Android Fixture

`dev.ariadne.fixture` is the authorized target for Experiment 001. ADB starts
`MainActivity` with `email` and `region` string extras. The activity writes
`files/observation.json` and exits. When ADB also supplies an integer
`collector_port`, the activity posts the same JSON to
`http://127.0.0.1:<port>/observe`.

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
