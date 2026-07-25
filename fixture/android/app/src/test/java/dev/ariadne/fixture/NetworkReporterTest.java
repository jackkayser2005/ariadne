package dev.ariadne.fixture;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertThrows;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;

import org.junit.Test;

public final class NetworkReporterTest {
    @Test
    public void sendsObservationToLoopbackCollector() throws Exception {
        byte[] observation = "{\"variant\":\"standard\"}"
                .getBytes(StandardCharsets.UTF_8);

        try (TestServer server = new TestServer(204)) {
            NetworkReporter.send(server.port(), observation);
            server.await();

            assertEquals("POST /observe HTTP/1.1", server.requestLine);
            assertEquals("application/json", server.contentType);
            assertEquals(new String(observation, StandardCharsets.UTF_8), server.body);
        }
    }

    @Test
    public void rejectsInvalidPort() {
        assertThrows(IOException.class, () -> NetworkReporter.send(0, new byte[0]));
        assertThrows(IOException.class, () -> NetworkReporter.send(65_536, new byte[0]));
    }

    @Test
    public void rejectsCollectorError() throws Exception {
        try (TestServer server = new TestServer(500)) {
            assertThrows(
                    IOException.class,
                    () -> NetworkReporter.send(server.port(), new byte[0]));
            server.await();
        }
    }

    private static final class TestServer implements AutoCloseable {
        private final ServerSocket server;
        private final int status;
        private final Thread worker;
        private Throwable failure;
        private String requestLine;
        private String contentType;
        private String body;

        TestServer(int status) throws IOException {
            this.status = status;
            server = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"));
            worker = new Thread(this::serve, "ariadne-fixture-test-server");
            worker.start();
        }

        int port() {
            return server.getLocalPort();
        }

        void await() throws Exception {
            worker.join(5_000);
            if (worker.isAlive()) {
                throw new IOException("test server did not stop");
            }
            if (failure != null) {
                throw new IOException("test server failed", failure);
            }
        }

        private void serve() {
            try (Socket socket = server.accept()) {
                BufferedReader input = new BufferedReader(new InputStreamReader(
                        socket.getInputStream(),
                        StandardCharsets.ISO_8859_1));
                requestLine = input.readLine();

                int contentLength = 0;
                for (String line = input.readLine();
                        line != null && !line.isEmpty();
                        line = input.readLine()) {
                    int separator = line.indexOf(':');
                    if (separator < 0) {
                        continue;
                    }
                    String name = line.substring(0, separator);
                    String value = line.substring(separator + 1).trim();
                    if (name.equalsIgnoreCase("Content-Length")) {
                        contentLength = Integer.parseInt(value);
                    } else if (name.equalsIgnoreCase("Content-Type")) {
                        contentType = value;
                    }
                }

                char[] payload = new char[contentLength];
                int offset = 0;
                while (offset < payload.length) {
                    int count = input.read(payload, offset, payload.length - offset);
                    if (count < 0) {
                        throw new IOException("request body ended early");
                    }
                    offset += count;
                }
                body = new String(payload);

                String reason = status == 204 ? "No Content" : "Internal Server Error";
                byte[] response = (
                        "HTTP/1.1 " + status + " " + reason + "\r\n"
                                + "Content-Length: 0\r\n"
                                + "Connection: close\r\n\r\n")
                        .getBytes(StandardCharsets.US_ASCII);
                OutputStream output = socket.getOutputStream();
                output.write(response);
                output.flush();
            } catch (Throwable error) {
                failure = error;
            }
        }

        @Override
        public void close() throws Exception {
            server.close();
            await();
        }
    }
}
