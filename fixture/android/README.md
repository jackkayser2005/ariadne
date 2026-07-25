# Android Fixture

`dev.ariadne.fixture` is the authorized target for Experiment 001. ADB starts
`MainActivity` with `email` and `region` string extras. The activity writes
`files/observation.json` and exits.

Build and test:

```console
./gradlew testDebugUnitTest createDebugUnitTestCoverageReport assembleDebug
```

The debug APK is written to `app/build/outputs/apk/debug/app-debug.apk`.
The fixture requests no Android permissions and does not send network traffic.
