package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

type harRule struct {
	Category string `json:"category"`
	Value    string `json:"value"`
}

// HARMatch counts requests containing an exact supplied value in one supported
// channel. The category is caller supplied; the value is never retained here.
type HARMatch struct {
	// Entries are one-based positions in the supplied log.entries array.
	Entries  []int  `json:"entries"`
	Category string `json:"category"`
	Channel  string `json:"channel"`
	Requests int    `json:"requests"`
}

func readHARRules(path string) ([]harRule, error) {
	if path == "" {
		return nil, nil
	}
	fail := func() ([]harRule, error) {
		return nil, errors.New("invalid test-value rules: require a bounded version 1 synthetic rule file")
	}
	data, err := bundle.ReadBoundedFile(path, 16<<10)
	if err != nil || !utf8.Valid(data) || !json.Valid(data) {
		return fail()
	}
	// The tiny schema cannot legitimately contain deeply nested containers.
	depth := 0
	quoted := false
	escaped := false
	for _, c := range data {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > 4 {
				return fail()
			}
		case '}', ']':
			depth--
		}
	}
	if jsoncheck.RejectDuplicateKeys(data) != nil {
		return fail()
	}
	var rules struct {
		SchemaVersion int       `json:"schema_version"`
		Synthetic     bool      `json:"synthetic"`
		Rules         []harRule `json:"rules"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&rules) != nil || rules.SchemaVersion != 1 || !rules.Synthetic || len(rules.Rules) == 0 || len(rules.Rules) > 16 {
		return fail()
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fail()
	}
	seen := map[string]bool{}
	for _, r := range rules.Rules {
		if r.Category != "email" && r.Category != "phone" && r.Category != "account-id" {
			return fail()
		}
		if len(r.Value) < 8 || len(r.Value) > 256 || seen[r.Value] {
			return fail()
		}
		for _, c := range r.Value {
			if unicode.IsControl(c) || unicode.IsSpace(c) {
				return fail()
			}
		}
		seen[r.Value] = true
	}
	return rules.Rules, nil
}

func addHARMatches(destination *HARDestination, query map[string][]string, params []harName, rules []harRule, entryNumber int, body harBody) {
	addHARMatchesWithRequestMetadata(destination, query, nil, params, nil, nil, rules, entryNumber, body)
}

func addHARMatchesWithRequestMetadata(destination *HARDestination, query map[string][]string, pathSegments []string, params, headers, cookies []harName, rules []harRule, entryNumber int, body harBody) {
	matched := map[string]bool{}
	for _, rule := range rules {
		for _, values := range query {
			for _, value := range values {
				if value == rule.Value {
					matched[rule.Category+"|URL query"] = true
				}
			}
		}
		for _, value := range pathSegments {
			if value == rule.Value {
				matched[rule.Category+"|URL path segment"] = true
			}
		}
		for _, value := range body.values {
			if value == rule.Value {
				matched[rule.Category+"|"+body.channel] = true
			}
		}
		for _, param := range params {
			if param.Value == rule.Value {
				matched[rule.Category+"|Exported form parameter"] = true
			}
		}
		for _, header := range headers {
			if header.Value == rule.Value {
				matched[rule.Category+"|Request header value"] = true
			}
		}
		for _, cookie := range cookies {
			if cookie.Value == rule.Value {
				matched[rule.Category+"|Request cookie value"] = true
			}
		}
	}
	channels := []string{"URL query", "URL path segment", "Exported form parameter", "JSON body string", "Form body value", "Request header value", "Request cookie value"}
	for _, rule := range rules {
		for _, channel := range channels {
			key := rule.Category + "|" + channel
			if !matched[key] {
				continue
			}
			delete(matched, key)
			found := false
			for i := range destination.Matches {
				m := &destination.Matches[i]
				if m.Category == rule.Category && m.Channel == channel {
					m.Entries = append(m.Entries, entryNumber)
					m.Requests = len(m.Entries)
					found = true
					break
				}
			}
			if !found {
				destination.Matches = append(destination.Matches, HARMatch{Category: rule.Category, Channel: channel, Requests: 1, Entries: []int{entryNumber}})
			}
		}
	}
	sort.Slice(destination.Matches, func(i, j int) bool {
		a, b := destination.Matches[i], destination.Matches[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Channel < b.Channel
	})
}
