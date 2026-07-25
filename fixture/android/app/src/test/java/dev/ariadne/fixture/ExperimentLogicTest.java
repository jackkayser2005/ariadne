package dev.ariadne.fixture;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertThrows;

import org.junit.Test;

public final class ExperimentLogicTest {
    @Test
    public void baselineEmailSelectsStandardVariant() {
        assertEquals("standard", ExperimentLogic.variantFor("baseline@example.invalid"));
    }

    @Test
    public void treatmentEmailSelectsPersonalizedVariant() {
        assertEquals("personalized", ExperimentLogic.variantFor("treatment@example.invalid"));
    }

    @Test
    public void missingEmailIsRejected() {
        assertThrows(NullPointerException.class, () -> ExperimentLogic.variantFor(null));
    }
}
