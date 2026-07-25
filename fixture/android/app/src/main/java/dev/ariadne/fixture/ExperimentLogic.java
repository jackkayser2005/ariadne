package dev.ariadne.fixture;

import java.util.Objects;

final class ExperimentLogic {
    private ExperimentLogic() {}

    static String variantFor(String email) {
        Objects.requireNonNull(email, "email");
        return email.startsWith("treatment@") ? "personalized" : "standard";
    }
}
