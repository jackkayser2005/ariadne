package dev.ariadne.fixture;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertThrows;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.UUID;

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

    @Test
    public void requestIDsAreValidAndFresh() {
        String first = ExperimentLogic.requestID();
        String second = ExperimentLogic.requestID();

        assertEquals(first, UUID.fromString(first).toString());
        assertFalse(first.equals(second));
    }

    @Test
    public void treatmentNetworkOnlyOmitsTreatmentStorage() {
        assertFalse(
                ExperimentLogic.shouldWriteStorage(
                        "treatment@example.invalid",
                        "treatment_network_only"));
    }

    @Test
    public void treatmentNetworkOnlyKeepsBaselineStorage() {
        assertTrue(
                ExperimentLogic.shouldWriteStorage(
                        "baseline@example.invalid",
                        "treatment_network_only"));
    }

    @Test
    public void defaultModeKeepsTreatmentStorage() {
        assertTrue(ExperimentLogic.shouldWriteStorage("treatment@example.invalid", null));
    }
}
