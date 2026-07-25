package dev.ariadne.fixture;

import android.app.Activity;
import android.os.Bundle;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

public final class MainActivity extends Activity {
    static final String OBSERVATION_FILE = "observation.json";

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
            writeObservation(email, region);
            setResult(RESULT_OK);
        } catch (IOException | JSONException error) {
            setResult(RESULT_CANCELED);
        }
        finish();
    }

    private void writeObservation(String email, String region)
            throws IOException, JSONException {
        JSONObject observation = new JSONObject()
                .put("schema_version", 1)
                .put("region", region)
                .put("variant", ExperimentLogic.variantFor(email));

        try (FileOutputStream output = openFileOutput(OBSERVATION_FILE, MODE_PRIVATE)) {
            output.write(observation.toString().getBytes(StandardCharsets.UTF_8));
        }
    }
}
