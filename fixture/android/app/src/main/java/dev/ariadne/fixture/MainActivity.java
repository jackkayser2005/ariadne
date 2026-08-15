package dev.ariadne.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

public final class MainActivity extends Activity {
    static final String INPUT_FILE = "ariadne-input.json";
    static final String OBSERVATION_FILE = "observation.json";
    private static final int REPORT_TIMEOUT_MILLIS = 5_000;

    private String email;
    private String region;
    private String captureMode;
    private int collectorPort;
    private String challenge;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        FixtureInput input;
        try {
            input = FixtureInput.read(this);
        } catch (IOException error) {
            setResult(RESULT_CANCELED);
            finish();
            return;
        }
        if (!getPackageName().equals(input.packageName())) {
            setResult(RESULT_CANCELED);
            finish();
            return;
        }

        email = input.value("email");
        region = input.value("region");
        challenge = input.challenge();
        if (email == null || region == null || challenge == null) {
            setResult(RESULT_CANCELED);
            finish();
            return;
        }

        captureMode = input.value("capture_mode");
        collectorPort = input.collectorPort();
        setContentView(R.layout.activity_main);
        Button observeButton = findViewById(R.id.observe_button);
        observeButton.setOnClickListener(this::runObservation);
        observeButton.requestFocus();
    }

    private void runObservation(View view) {
        view.setEnabled(false);
        try {
            byte[] observation = observationFor(email, region, challenge);
            if (ExperimentLogic.shouldWriteStorage(email, captureMode)) {
                writeObservation(observation);
            }
            if (collectorPort != 0) {
                reportObservation(collectorPort, observation);
            }
            setResult(RESULT_OK);
        } catch (IOException | JSONException error) {
            setResult(RESULT_CANCELED);
        }
        finish();
    }

    private byte[] observationFor(String email, String region, String challenge) throws JSONException {
        JSONObject observation = new JSONObject()
                .put("schema_version", 1)
                .put("challenge", challenge)
                .put("region", region)
                .put("request_id", ExperimentLogic.requestID())
                .put("variant", ExperimentLogic.variantFor(email));
        return observation.toString().getBytes(StandardCharsets.UTF_8);
    }

    private void writeObservation(byte[] observation) throws IOException {
        try (FileOutputStream output = openFileOutput(OBSERVATION_FILE, MODE_PRIVATE)) {
            output.write(observation);
        }
    }

    private void reportObservation(int port, byte[] observation) throws IOException {
        AtomicReference<IOException> failure = new AtomicReference<>();
        Thread report = new Thread(() -> {
            try {
                NetworkReporter.send(port, observation);
            } catch (IOException error) {
                failure.set(error);
            }
        }, "ariadne-fixture-report");
        report.start();

        try {
            report.join(REPORT_TIMEOUT_MILLIS);
        } catch (InterruptedException error) {
            Thread.currentThread().interrupt();
            throw new IOException("network report interrupted", error);
        }
        if (report.isAlive()) {
            report.interrupt();
            throw new IOException("network report timed out");
        }
        if (failure.get() != null) {
            throw failure.get();
        }
    }
}
