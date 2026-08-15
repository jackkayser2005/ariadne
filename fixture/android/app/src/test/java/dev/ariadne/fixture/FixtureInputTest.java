package dev.ariadne.fixture;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.fail;

import java.io.IOException;
import java.nio.charset.StandardCharsets;

import org.junit.Test;

public final class FixtureInputTest {
    private static final String CHALLENGE = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    private static final String PROCEDURE = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    @Test
    public void acceptsCanonicalInput() throws Exception {
        FixtureInput input = FixtureInput.decode(validInput().getBytes(StandardCharsets.UTF_8));

        assertEquals("dev.ariadne.fixture", input.packageName());
        assertEquals(CHALLENGE, input.challenge());
        assertEquals("baseline", input.role());
        assertEquals("baseline-treatment", input.order());
        assertEquals(PROCEDURE, input.procedureSHA256());
        assertEquals(43210, input.collectorPort());
        assertEquals("baseline@example.invalid", input.value("email"));
        assertEquals("us-east", input.value("region"));
        assertNull(input.value("capture_mode"));
    }

    @Test
    public void rejectsDuplicateKeys() {
        assertInvalid(validInput().replace(
                "\"role\":\"baseline\"",
                "\"role\":\"baseline\",\"role\":\"baseline\""));
    }

    @Test
    public void rejectsWhitespaceAndReorderedFields() {
        assertInvalid(validInput().replace("{\"schema_version\"", "{ \"schema_version\""));
        assertInvalid(
                "{\"package_name\":\"dev.ariadne.fixture\",\"schema_version\":1,"
                        + "\"challenge\":\"" + CHALLENGE + "\",\"role\":\"baseline\","
                        + "\"order\":\"baseline-treatment\",\"procedure_sha256\":\"" + PROCEDURE
                        + "\",\"collector_port\":43210,\"persona\":{\"email\":\"baseline@example.invalid\","
                        + "\"region\":\"us-east\"}}\n");
    }

    @Test
    public void rejectsUnknownAndMissingFields() {
        assertInvalid(validInput().replace(
                "\"persona\":{",
                "\"extra\":\"unexpected\",\"persona\":{"));
        assertInvalid(validInput().replace("\"challenge\":\"" + CHALLENGE + "\",", ""));
    }

    @Test
    public void rejectsInvalidChallengeAndUnsafePersona() {
        assertInvalid(validInput().replace(CHALLENGE, CHALLENGE.substring(0, 63) + "G"));
        assertInvalid(validInput().replace("us-east", "us east"));
    }

    @Test
    public void rejectsOversizedInput() {
        assertInvalid("{" + "x".repeat(32 * 1024) + "}");
    }

    private static String validInput() {
        return "{\"schema_version\":1,\"package_name\":\"dev.ariadne.fixture\","
                + "\"challenge\":\"" + CHALLENGE + "\",\"role\":\"baseline\","
                + "\"order\":\"baseline-treatment\",\"procedure_sha256\":\"" + PROCEDURE
                + "\",\"collector_port\":43210,\"persona\":{\"email\":\"baseline@example.invalid\","
                + "\"region\":\"us-east\"}}\n";
    }

    private static void assertInvalid(String value) {
        try {
            FixtureInput.decode(value.getBytes(StandardCharsets.UTF_8));
            fail("expected invalid fixture input");
        } catch (IOException expected) {
            // Expected: malformed, reordered, or unsafe fixture input is rejected.
        }
    }
}
