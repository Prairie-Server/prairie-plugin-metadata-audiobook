package provider

import (
	"encoding/json"
	"regexp"

	"github.com/prairie-server/prairie-plugin-metadata-audiobook/metadata"
)

var audiotekaInitialStateRE = regexp.MustCompile(
	`(?i)window\.__(?:INITIAL|PRELOADED)_STATE__\s*=\s*(\{[\s\S]*?\});`)

func parseJSONLDMatch(html string, providerName string) *metadata.Match {
	m := jsonLdRE.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var node struct {
		Type          string      `json:"@type"`
		Name          string      `json:"name"`
		Description   string      `json:"description"`
		Image         string      `json:"image"`
		InLanguage    string      `json:"inLanguage"`
		DatePublished string      `json:"datePublished"`
		Duration      string      `json:"duration"`
		ISBN          string      `json:"isbn"`
		Author        interface{} `json:"author"`
		ReadBy        interface{} `json:"readBy"`
		Publisher     interface{} `json:"publisher"`
	}
	if err := json.Unmarshal([]byte(m[1]), &node); err != nil {
		return nil
	}
	if node.Type != "Audiobook" && node.Type != "Book" {
		return nil
	}
	match := &metadata.Match{
		Provider:    providerName,
		Title:       node.Name,
		Description: node.Description,
		CoverURL:    node.Image,
		Language:    node.InLanguage,
		ISBN:        node.ISBN,
		PublishYear: extractYear(node.DatePublished),
		DurationMin: parseISODurationToMin(node.Duration),
	}
	match.Authors = extractJSONLDNamesFromInterface(node.Author)
	match.Narrators = extractJSONLDNamesFromInterface(node.ReadBy)
	match.Publisher = extractPublisherName(node.Publisher)
	return match
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func namesFromArray(value interface{}) []string {
	arr, ok := value.([]interface{})
	if !ok {
		return nil
	}
	var names []string
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			if v != "" {
				names = append(names, v)
			}
		case map[string]interface{}:
			if name := stringField(v, "name"); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func stringArrayFromField(value interface{}) []string {
	arr, ok := value.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if value, ok := item.(string); ok && value != "" {
			out = append(out, value)
		}
	}
	return out
}

func bestCoverURL(m map[string]interface{}) string {
	if direct := stringField(m, "coverUrl"); direct != "" {
		return direct
	}
	cover, ok := m["cover"].(map[string]interface{})
	if !ok {
		return ""
	}
	bestURL := ""
	bestWidth := -1
	if sizes, ok := cover["sizes"].([]interface{}); ok {
		for _, item := range sizes {
			size, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			width, _ := size["width"].(float64)
			if int(width) > bestWidth {
				bestWidth = int(width)
				bestURL = stringField(size, "url")
			}
		}
	}
	if bestURL != "" {
		return bestURL
	}
	return stringField(cover, "url")
}
