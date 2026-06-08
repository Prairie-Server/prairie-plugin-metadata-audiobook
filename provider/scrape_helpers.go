package provider

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxTraverseDepth = 64

func readLimitedBody(resp *http.Response, source string) (string, error) {
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: HTTP %d", source, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("%s: read response: %w", source, err)
	}
	return string(body), nil
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func numberPosition(value float64) string {
	if value == 0 {
		return ""
	}
	if value == float64(int(value)) {
		return fmt.Sprintf("%d", int(value))
	}
	return fmt.Sprintf("%v", value)
}
