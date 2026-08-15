package dev.ariadne.fixture;

import android.content.Context;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.FileInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import java.util.Set;

final class FixtureInput {
    private static final int SCHEMA_VERSION = 1;
    private static final int MAX_BYTES = 32 * 1024;
    private static final Set<String> TOP_LEVEL_FIELDS = new HashSet<>(Arrays.asList(
            "schema_version",
            "package_name",
            "challenge",
            "role",
            "order",
            "procedure_sha256",
            "collector_port",
            "persona"));

    private final String packageName;
    private final String challenge;
    private final String role;
    private final String order;
    private final String procedureSHA256;
    private final int collectorPort;
    private final Map<String, String> persona;

    private FixtureInput(
            String packageName,
            String challenge,
            String role,
            String order,
            String procedureSHA256,
            int collectorPort,
            Map<String, String> persona) {
        this.packageName = packageName;
        this.challenge = challenge;
        this.role = role;
        this.order = order;
        this.procedureSHA256 = procedureSHA256;
        this.collectorPort = collectorPort;
        this.persona = persona;
    }

    static FixtureInput read(Context context) throws IOException {
        byte[] data;
        try (FileInputStream input = context.openFileInput(MainActivity.INPUT_FILE)) {
            data = readBounded(input);
        } finally {
            context.deleteFile(MainActivity.INPUT_FILE);
        }
        return decode(data);
    }

    static FixtureInput decode(byte[] data) throws IOException {
        if (data == null || data.length == 0 || data.length > MAX_BYTES) {
            throw invalidInput();
        }

        String source = new String(data, StandardCharsets.UTF_8);
        try {
            JSONObject root = new JSONObject(source);
            if (!hasExactly(root, TOP_LEVEL_FIELDS)) {
                throw invalidInput();
            }
            if (root.getInt("schema_version") != SCHEMA_VERSION) {
                throw invalidInput();
            }

            String packageName = root.getString("package_name");
            String challenge = root.getString("challenge");
            String role = root.getString("role");
            String order = root.getString("order");
            String procedureSHA256 = root.getString("procedure_sha256");
            int collectorPort = root.getInt("collector_port");
            JSONObject personaObject = root.getJSONObject("persona");
            Map<String, String> persona = readPersona(personaObject);

            if (!validToken(packageName) || !validHex(challenge) ||
                    (!role.equals("baseline") && !role.equals("treatment")) ||
                    (!order.equals("baseline-treatment") && !order.equals("treatment-baseline")) ||
                    !validHex(procedureSHA256) || collectorPort < 1 || collectorPort > 65_535 ||
                    persona.isEmpty() || persona.size() > 64) {
                throw invalidInput();
            }

            String canonical = canonical(
                    packageName,
                    challenge,
                    role,
                    order,
                    procedureSHA256,
                    collectorPort,
                    persona);
            if (!source.equals(canonical + "\n")) {
                throw invalidInput();
            }
            return new FixtureInput(
                    packageName,
                    challenge,
                    role,
                    order,
                    procedureSHA256,
                    collectorPort,
                    persona);
        } catch (JSONException | RuntimeException error) {
            throw invalidInput();
        }
    }

    String packageName() {
        return packageName;
    }

    String challenge() {
        return challenge;
    }

    String role() {
        return role;
    }

    String order() {
        return order;
    }

    String procedureSHA256() {
        return procedureSHA256;
    }

    int collectorPort() {
        return collectorPort;
    }

    String value(String key) {
        return persona.get(key);
    }

    private static Map<String, String> readPersona(JSONObject object) throws JSONException, IOException {
        Map<String, String> persona = new HashMap<>();
        Iterator<String> keys = object.keys();
        while (keys.hasNext()) {
            String key = keys.next();
            String value = object.getString(key);
            if (!validToken(key) || !validToken(value)) {
                throw invalidInput();
            }
            persona.put(key, value);
        }
        return persona;
    }

    private static boolean hasExactly(JSONObject object, Set<String> expected) {
        Set<String> actual = new HashSet<>();
        Iterator<String> keys = object.keys();
        while (keys.hasNext()) {
            actual.add(keys.next());
        }
        return actual.equals(expected) && actual.size() == expected.size();
    }

    private static String canonical(
            String packageName,
            String challenge,
            String role,
            String order,
            String procedureSHA256,
            int collectorPort,
            Map<String, String> persona) {
        StringBuilder output = new StringBuilder();
        output.append("{\"schema_version\":1")
                .append(",\"package_name\":\"").append(packageName).append('"')
                .append(",\"challenge\":\"").append(challenge).append('"')
                .append(",\"role\":\"").append(role).append('"')
                .append(",\"order\":\"").append(order).append('"')
                .append(",\"procedure_sha256\":\"").append(procedureSHA256).append('"')
                .append(",\"collector_port\":").append(collectorPort)
                .append(",\"persona\":{");

        List<String> keys = new ArrayList<>(persona.keySet());
        Collections.sort(keys);
        for (int index = 0; index < keys.size(); index++) {
            if (index > 0) {
                output.append(',');
            }
            String key = keys.get(index);
            output.append('"').append(key).append("\":\"").append(persona.get(key)).append('"');
        }
        return output.append("}}").toString();
    }

    private static byte[] readBounded(FileInputStream input) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[4096];
        int total = 0;
        int count;
        while ((count = input.read(buffer)) != -1) {
            total += count;
            if (total > MAX_BYTES) {
                throw invalidInput();
            }
            output.write(buffer, 0, count);
        }
        return output.toByteArray();
    }

    private static boolean validHex(String value) {
        if (value == null || value.length() != 64) {
            return false;
        }
        for (int index = 0; index < value.length(); index++) {
            char character = value.charAt(index);
            if (!((character >= '0' && character <= '9') ||
                    (character >= 'a' && character <= 'f'))) {
                return false;
            }
        }
        return true;
    }

    private static boolean validToken(String value) {
        if (value == null || value.isEmpty() || value.length() > 1024) {
            return false;
        }
        for (int index = 0; index < value.length(); index++) {
            char character = value.charAt(index);
            boolean letter = character >= 'a' && character <= 'z' ||
                    character >= 'A' && character <= 'Z';
            boolean digit = character >= '0' && character <= '9';
            if (!letter && !digit && "._@:+-".indexOf(character) < 0) {
                return false;
            }
        }
        return true;
    }

    private static IOException invalidInput() {
        return new IOException("fixture input is invalid");
    }
}
