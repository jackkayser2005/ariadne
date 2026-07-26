package dev.ariadne.fixture;

import java.util.Objects;

final class ExperimentLogic {
    private static final String TREATMENT_NETWORK_ONLY = "treatment_network_only";

    private ExperimentLogic() {}

    static String variantFor(String email) {
        Objects.requireNonNull(email, "email");
        return email.startsWith("treatment@") ? "personalized" : "standard";
    }

    static boolean shouldWriteStorage(String email, String captureMode) {
        Objects.requireNonNull(email, "email");
        return !TREATMENT_NETWORK_ONLY.equals(captureMode) || !email.startsWith("treatment@");
    }
}
