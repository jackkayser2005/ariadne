package browser

import "sort"

type harInformationDestination struct {
	Label   string
	Index   int
	Entries []int
}

type harInformation struct {
	Title        string
	Requests     int
	Destinations []harInformationDestination
}

// informationOverview groups only exact matches of supplied test values. It
// deduplicates entries across channels; field-name clues never create a flow.
func informationOverview(review HARReview, rules []harRule) []harInformation {
	var result []harInformation
	for _, category := range []struct{ id, title string }{{"email", "Email test value"}, {"phone", "Phone test value"}, {"account-id", "Account identifier test value"}} {
		configured := false
		for _, rule := range rules {
			if rule.Category == category.id {
				configured = true
				break
			}
		}
		if !configured {
			continue
		}
		info := harInformation{Title: category.title}
		for i, destination := range review.Destinations {
			entries := map[int]bool{}
			for _, match := range destination.Matches {
				if match.Category == category.id {
					for _, entry := range match.Entries {
						entries[entry] = true
					}
				}
			}
			if len(entries) == 0 {
				continue
			}
			path := harInformationDestination{Label: destination.Label, Index: i + 1}
			for entry := range entries {
				path.Entries = append(path.Entries, entry)
			}
			sort.Ints(path.Entries)
			info.Requests += len(path.Entries)
			info.Destinations = append(info.Destinations, path)
		}
		result = append(result, info)
	}
	return result
}
