package trace

import "slices"

// CategoryDefinition describes one reviewed, raw-value-free information
// category. The ID is the stable value stored in trace documents; Label and
// Description are reader-facing explanations of that label.
type CategoryDefinition struct {
	ID          string
	Label       string
	Description string
}

var categoryDefinitions = []CategoryDefinition{
	{ID: "account-id", Label: "Account identifier", Description: "An identifier associated with an account."},
	{ID: "advertising-id", Label: "Advertising identifier", Description: "An identifier used for advertising or measurement."},
	{ID: "consent", Label: "Consent choice", Description: "A choice about permission or privacy preferences."},
	{ID: "cookie-id", Label: "Cookie identifier", Description: "An identifier stored in a browser cookie."},
	{ID: "device-id", Label: "Device identifier", Description: "An identifier associated with a device."},
	{ID: "email", Label: "Email", Description: "An email address or email label."},
	{ID: "ip-address", Label: "IP address", Description: "A network address that can indicate a connection."},
	{ID: "location", Label: "Location", Description: "A precise or approximate place."},
	{ID: "phone", Label: "Phone", Description: "A phone number or phone label."},
	{ID: "region", Label: "Region", Description: "A broader geographic area."},
	{ID: "session-id", Label: "Session identifier", Description: "An identifier for a visit or session."},
	{ID: "unknown", Label: "Unclassified information", Description: "Information that could not be classified safely."},
	{ID: "user-agent", Label: "Browser or device software", Description: "Information about browser or device software."},
}

// CategoryDefinitions returns the reviewed vocabulary in stable order. The
// returned slice can be modified by the caller without changing the catalog.
func CategoryDefinitions() []CategoryDefinition {
	return slices.Clone(categoryDefinitions)
}

// CategoryLabel returns a reader-facing label for a reviewed category. An
// unknown value gets a fixed fallback so callers never need to display an
// unreviewed description as if it were part of the vocabulary.
func CategoryLabel(id string) string {
	if definition, ok := categoryDefinition(id); ok {
		return definition.Label
	}
	return "Reviewed information category"
}

// CategoryMeaning returns a reader-facing explanation for a reviewed category.
// Unknown values receive a bounded explanation rather than being interpreted.
func CategoryMeaning(id string) string {
	if definition, ok := categoryDefinition(id); ok {
		return definition.Description
	}
	return "A category label retained by the verifier; its value is not shown."
}

// DestinationDefinition describes one reviewed, raw-value-free destination
// boundary. The label describes the boundary category; it does not identify an
// organization or establish onward sharing.
type DestinationDefinition struct {
	ID          string
	Label       string
	Description string
}

var destinationDefinitions = []DestinationDefinition{
	{ID: "advertising", Label: "Advertising", Description: "A reviewed boundary used for advertising or measurement."},
	{ID: "analytics", Label: "Analytics", Description: "A reviewed boundary used for product or usage measurement."},
	{ID: "crash-reporting", Label: "Crash reporting", Description: "A reviewed boundary used for crash or error reports."},
	{ID: "first-party", Label: "First-party", Description: "The reviewed boundary named as part of the product being tested."},
	{ID: "unknown", Label: "Unclassified destination", Description: "A destination that could not be classified safely."},
}

// DestinationDefinitions returns the reviewed destination vocabulary in stable
// order. The returned slice can be modified without changing the catalog.
func DestinationDefinitions() []DestinationDefinition {
	return slices.Clone(destinationDefinitions)
}

// DestinationLabel returns a reader-facing label for a reviewed destination.
// Unknown values get a fixed fallback and are not interpreted.
func DestinationLabel(id string) string {
	if definition, ok := destinationDefinition(id); ok {
		return definition.Label
	}
	return "Reviewed destination boundary"
}

// DestinationMeaning returns a reader-facing explanation for a reviewed
// destination. It does not infer ownership or server-side handling.
func DestinationMeaning(id string) string {
	if definition, ok := destinationDefinition(id); ok {
		return definition.Description
	}
	return "A destination label retained by the verifier; ownership and onward handling are not shown."
}

func destinationDefinition(id string) (DestinationDefinition, bool) {
	for _, definition := range destinationDefinitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return DestinationDefinition{}, false
}

func categoryDefinition(id string) (CategoryDefinition, bool) {
	for _, definition := range categoryDefinitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return CategoryDefinition{}, false
}
