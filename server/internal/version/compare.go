package version

import (
	"strconv"
	"strings"
)

func Normalize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "refs/tags/")
	value = strings.TrimPrefix(value, "release-")
	value = strings.TrimPrefix(value, "v")
	value = strings.TrimPrefix(value, "V")
	return value
}

func IsNewer(latest, current string) bool {
	latest = Normalize(latest)
	current = Normalize(current)
	if latest == "" || latest == current {
		return false
	}
	latestParts := parseParts(latest)
	currentParts := parseParts(current)
	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}
	for i := 0; i < maxLen; i++ {
		var left, right int
		if i < len(latestParts) {
			left = latestParts[i]
		}
		if i < len(currentParts) {
			right = currentParts[i]
		}
		if left > right {
			return true
		}
		if left < right {
			return false
		}
	}
	return latest != current
}

func parseParts(value string) []int {
	value = strings.Split(value, "-")[0]
	value = strings.Split(value, "+")[0]
	rawParts := strings.Split(value, ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		raw = leadingDigits(raw)
		if raw == "" {
			parts = append(parts, 0)
			continue
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, parsed)
	}
	return parts
}

func leadingDigits(value string) string {
	var builder strings.Builder
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			break
		}
		builder.WriteRune(ch)
	}
	return builder.String()
}
