package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// SelectIndices prompts the user to select indices from a list.
// Input format: comma-separated numbers (e.g., "1,3,5") or "all" or "q" to quit.
func SelectIndices(total int) ([]int, error) {
	prompt := &survey.Input{
		Message: fmt.Sprintf("Select items (1-%d, comma-separated, 'all', or 'q' to quit):", total),
	}

	var answer string
	if err := survey.AskOne(prompt, &answer); err != nil {
		return nil, err
	}

	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil, nil
	}

	if answer == "q" || answer == "quit" {
		return nil, fmt.Errorf("cancelled by user")
	}

	if answer == "all" {
		indices := make([]int, total)
		for i := 0; i < total; i++ {
			indices[i] = i
		}
		return indices, nil
	}

	parts := strings.Split(answer, ",")
	var indices []int
	seen := make(map[int]bool)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle ranges like "1-5"
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			if start < 1 || end > total || start > end {
				continue
			}
			for i := start; i <= end; i++ {
				if !seen[i-1] {
					indices = append(indices, i-1)
					seen[i-1] = true
				}
			}
			continue
		}

		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > total {
			continue
		}

		if !seen[n-1] {
			indices = append(indices, n-1)
			seen[n-1] = true
		}
	}

	return indices, nil
}

// SelectOne prompts the user to select a single option from a list.
func SelectOne(options []string, message string) (int, error) {
	var result int
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return -1, err
	}
	return result, nil
}

// SelectMulti prompts the user to select multiple options.
func SelectMulti(options []string, message string) ([]string, error) {
	var result []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Input prompts for a text input with an optional default.
func Input(message, defaultValue string) (string, error) {
	prompt := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	var result string
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// Password prompts for a password (hidden input).
func Password(message string) (string, error) {
	prompt := &survey.Password{
		Message: message,
	}
	var result string
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}
