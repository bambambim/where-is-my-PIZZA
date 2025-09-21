package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config holds all configuration for the application.
type Config struct {
	Database struct {
		Host         string
		Port         int
		User         string
		Password     string
		Database     string
		PoolMaxConns int
	}
	RabbitMQ struct {
		Host     string
		Port     int
		User     string
		Password string
	}
}

// Load populates the Config struct by reading and parsing a JSON file.
func Load(path string) (*Config, error) {
	f, err := YAMLToJSON(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(f, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func YAMLToJSON(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
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
		var intVal int
		if _, err := fmt.Sscanf(val, "%d", &intVal); err == nil {
			v = intVal
		} else {
			// remove quotes if exist
			v = strings.Trim(val, `"'`)
		}

		if currentSection != "" {
			result[currentSection][key] = v
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Marshal map to JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return jsonBytes, nil
}
