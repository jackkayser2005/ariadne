package dev.ariadne.fixture;

import android.app.Activity;
import android.os.Bundle;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

public final class MainActivity extends Activity {
    static final String OBSERVATION_FILE = "observation.json";
    private static final int REPORT_TIMEOUT_MILLIS = 5_000;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        String email = getIntent().getStringExtra("email");
        String region = getIntent().getStringExtra("region");
        if (email == null || region == null) {
            setResult(RESULT_CANCELED);
            finish();
            return;
        }

        try {
            byte[] observation = observationFor(email, region);
            String captureMode = getIntent().getStringExtra("capture_mode");
            if (ExperimentLogic.shouldWriteStorage(email, captureMode)) {
                writeObservation(observation);
            }
            int collectorPort = getIntent().getIntExtra("collector_port", 0);
            if (collectorPort != 0) {
                reportObservation(collectorPort, observation);
            }
            setResult(RESULT_OK);
        } catch (IOException | JSONException error) {
            setResult(RESULT_CANCELED);
        }
        finish();
    }

    private byte[] observationFor(String email, String region) throws JSONException {
        JSONObject observation = new JSONObject()
                .put("schema_version", 1)
                .put("region", region)
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
