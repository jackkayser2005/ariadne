package dev.ariadne.fixture;

import java.io.IOException;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.util.Objects;

final class NetworkReporter {
    private static final int REQUEST_TIMEOUT_MILLIS = 2_000;

    private NetworkReporter() {}

    static void send(int port, byte[] body) throws IOException {
        if (port < 1 || port > 65_535) {
            throw new IOException("collector port is invalid");
        }
        Objects.requireNonNull(body, "body");

        HttpURLConnection connection =
                (HttpURLConnection) URI.create("http://127.0.0.1:" + port + "/observe")
                        .toURL()
                        .openConnection();
        connection.setConnectTimeout(REQUEST_TIMEOUT_MILLIS);
        connection.setReadTimeout(REQUEST_TIMEOUT_MILLIS);
        connection.setInstanceFollowRedirects(false);
        connection.setRequestMethod("POST");
        connection.setRequestProperty("Content-Type", "application/json");
        connection.setDoOutput(true);
        connection.setFixedLengthStreamingMode(body.length);

        try {
            try (OutputStream output = connection.getOutputStream()) {
                output.write(body);
            }
            if (connection.getResponseCode() != HttpURLConnection.HTTP_NO_CONTENT) {
                throw new IOException("collector rejected observation");
            }
        } finally {
            connection.disconnect();
        }
    }
}
