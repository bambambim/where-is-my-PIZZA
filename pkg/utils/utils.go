package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// PrettyDuration returns string information about duration in pretty format
// Examples:
// 15s                  => "15 seconds"
// 1m20s                => "1 minute 20 seconds"
// 1h15m0s              => "1 hour 15 minutes"
// 1h30m0s              => "1 hour 30 minutes"
// 2m3s                 => "2 minutes 3 seconds"
// 1h1m12s              => "1 hour 1 minute 12 seconds"
// 0s                   => "0 seconds"
func PrettyDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	var parts []string

	if hours > 0 {
		unit := "hour"
		if hours > 1 {
			unit = "hours"
		}
		parts = append(parts, fmt.Sprintf("%d %s", hours, unit))
	}

	if minutes > 0 {
		unit := "minute"
		if minutes > 1 {
			unit = "minutes"
		}
		parts = append(parts, fmt.Sprintf("%d %s", minutes, unit))
	}

	if seconds > 0 || len(parts) == 0 {
		unit := "second"
		if seconds != 1 {
			unit = "seconds"
		}
		parts = append(parts, fmt.Sprintf("%d %s", seconds, unit))
	}

	return joinParts(parts)
}

func joinParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " " + parts[1]
	default:
		return parts[0] + " " + parts[1] + " " + parts[2]
	}
}

func YAMLToJSON(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	result := make(map[string]map[string]interface{})
	var currentSection string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section (top-level key ending with ":")
		if strings.HasSuffix(line, ":") {
			currentSection = strings.TrimSuffix(line, ":")
			result[currentSection] = make(map[string]interface{})
			continue
		}

		// Parse key: value lines
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Try to convert to int if possible
		var v interface{} = val
		if n, err := fmt.Sscanf(val, "%d", new(int)); err == nil {
			v = n
		} else {
			// remove quotes if exist
			v = strings.Trim(val, `"'`)
		}

		if currentSection != "" {
			result[currentSection][key] = v
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	// Marshal map to JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
