package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jeircul/pim/pkg/azpim"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

// PromptSelection displays items and prompts for user selection
func PromptSelection[T any](items []T, displayFunc func(int, T) string, prompt string) (T, error) {
	var zero T
	if len(items) == 0 {
		return zero, azpim.ErrNoItems
	}

	// Display all items
	for i, item := range items {
		fmt.Println(displayFunc(i+1, item))
	}

	// Create reader for input
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("\n%s (1-%d or 'q' to quit): ", prompt, len(items))

		input, err := reader.ReadString('\n')
		if err != nil {
			return zero, fmt.Errorf("read input: %w", err)
		}

		input = trimInput(input)

		// Check for quit
		if input == "q" || input == "quit" {
			return zero, azpim.ErrUserCancelled
		}

		// Parse selection
		selection, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("❌ Invalid input. Please enter a number or 'q' to quit.")
			continue
		}

		if selection < 1 || selection > len(items) {
			fmt.Printf("❌ Selection must be between 1 and %d.\n", len(items))
			continue
		}

		return items[selection-1], nil
	}
}

// trimInput removes whitespace and newlines from input
func trimInput(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimRight(s, "\r\n")
}

// PromptMultiSelection displays items, supports fuzzy filtering, and allows selecting multiple entries.
func PromptMultiSelection[T any](items []T, displayFunc func(int, T) string, keyFunc func(T) string, prompt string) ([]T, error) {
	if len(items) == 0 {
		return nil, azpim.ErrNoItems
	}

	original := make([]viewItem[T], len(items))
	for i, item := range items {
		original[i] = viewItem[T]{idx: i, value: item}
	}
	current := append([]viewItem[T](nil), original...)
	keys := make([]string, len(items))
	keysLower := make([]string, len(items))
	for i, item := range items {
		key := keyFunc(item)
		keys[i] = key
		keysLower[i] = strings.ToLower(key)
	}

	printView(current, displayFunc)
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("\n%s (comma-separated numbers, search text, 'all', or 'q' to quit): ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read input: %w", err)
		}

		input = trimInput(input)
		if input == "" {
			printView(current, displayFunc)
			continue
		}

		lower := strings.ToLower(input)
		if lower == "q" || lower == "quit" {
			return nil, azpim.ErrUserCancelled
		}

		if lower == "all" || lower == "*" {
			current = append([]viewItem[T](nil), original...)
			fmt.Printf("\nShowing all results (%d):\n", len(current))
			printView(current, displayFunc)
			continue
		}

		if selections, ok, selErr := tryParseSelections(input, len(current)); ok {
			if selErr != nil {
				fmt.Println("❌", selErr)
				continue
			}
			chosen := make([]T, 0, len(selections))
			for _, sel := range selections {
				chosen = append(chosen, current[sel-1].value)
			}
			return chosen, nil
		}

		if matches, total := filterViewBySubstring(original, keysLower, lower, 20); total > 0 {
			current = matches
			fmt.Printf("\nShowing %d of %d match(es) containing %q:\n", len(matches), total, input)
			printView(current, displayFunc)
			continue
		}

		matches := fuzzy.RankFindFold(input, keys)
		if len(matches) == 0 {
			fmt.Printf("No matches for %q. Try another search or type 'all'.\n", input)
			continue
		}
		sort.Sort(matches)
		limit := len(matches)
		if limit > 20 {
			limit = 20
		}
		filtered := make([]viewItem[T], 0, limit)
		for i := 0; i < limit; i++ {
			idx := matches[i].OriginalIndex
			filtered = append(filtered, viewItem[T]{idx: idx, value: items[idx]})
		}
		current = filtered
		fmt.Printf("\nTop %d match(es) for %q:\n", len(current), input)
		printView(current, displayFunc)
	}
}

// PromptSingleSelection ensures exactly one item is returned using the fuzzy selection flow
func PromptSingleSelection[T any](items []T, displayFunc func(int, T) string, keyFunc func(T) string, prompt string) (T, error) {
	var zero T
	for {
		chosen, err := PromptMultiSelection(items, displayFunc, keyFunc, prompt)
		if err != nil {
			return zero, err
		}
		if len(chosen) == 1 {
			return chosen[0], nil
		}
		fmt.Println("❌ Please select exactly one entry.")
	}
}

type viewItem[T any] struct {
	idx   int
	value T
}

func printView[T any](items []viewItem[T], displayFunc func(int, T) string) {
	if len(items) == 0 {
		fmt.Println("(no results)")
		return
	}
	limit := len(items)
	const maxDisplay = 50
	if limit > maxDisplay {
		limit = maxDisplay
	}
	for i := 0; i < limit; i++ {
		fmt.Println(displayFunc(i+1, items[i].value))
	}
	if len(items) > limit {
		fmt.Printf("  ...and %d more. Narrow further or search.\n", len(items)-limit)
	}
}

func tryParseSelections(input string, limit int) ([]int, bool, error) {
	if strings.TrimSpace(input) == "" {
		return nil, false, nil
	}
	parts := strings.Split(input, ",")
	if len(parts) == 0 {
		return nil, false, nil
	}
	selections := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, true, fmt.Errorf("selection cannot be empty")
		}
		num, err := strconv.Atoi(value)
		if err != nil {
			return nil, false, nil
		}
		if num < 1 || num > limit {
			return nil, true, fmt.Errorf("selection must be between 1 and %d", limit)
		}
		if _, ok := seen[num]; ok {
			continue
		}
		seen[num] = struct{}{}
		selections = append(selections, num)
	}
	if len(selections) == 0 {
		return nil, true, fmt.Errorf("no selections provided")
	}
	return selections, true, nil
}

func filterViewBySubstring[T any](all []viewItem[T], lowerKeys []string, needle string, limit int) ([]viewItem[T], int) {
	if needle == "" {
		return nil, 0
	}
	total := 0
	filtered := make([]viewItem[T], 0, min(limit, len(all)))
	for i, key := range lowerKeys {
		if strings.Contains(key, needle) {
			total++
			if len(filtered) < limit {
				filtered = append(filtered, all[i])
			}
		}
	}
	return filtered, total
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PromptJustification requests a justification from the user, falling back to an existing value when provided.
func PromptJustification(existing string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		label := "Justification"
		if existing != "" {
			fmt.Printf("%s [%s] (enter to keep, 'q' to cancel): ", label, existing)
		} else {
			fmt.Printf("%s (required, 'q' to cancel): ", label)
		}
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read justification: %w", err)
		}
		value := trimInput(input)
		if isQuit(value) {
			return "", azpim.ErrUserCancelled
		}
		if value == "" {
			if existing != "" {
				return existing, nil
			}
			fmt.Println("❌ Justification is required.")
			continue
		}
		return value, nil
	}
}

// PromptDuration collects a duration within the allowed activation window.
func PromptDuration(currentMinutes int) (int, error) {
	if currentMinutes < azpim.MinMinutes || currentMinutes > azpim.MaxMinutes {
		currentMinutes = azpim.MinMinutes
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Duration (e.g., '1h', '90m', '1h30m', min %dm, max %dm) [%s]: ",
			azpim.MinMinutes, azpim.MaxMinutes, formatDurationPrompt(currentMinutes))
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read duration: %w", err)
		}
		value := trimInput(input)
		if isQuit(value) {
			return 0, azpim.ErrUserCancelled
		}
		if value == "" {
			return currentMinutes, nil
		}
		minutes, parseErr := parseDurationPrompt(value)
		if parseErr != nil {
			fmt.Printf("❌ %v\n", parseErr)
			continue
		}
		if minutes < azpim.MinMinutes || minutes > azpim.MaxMinutes {
			fmt.Printf("❌ Duration must be between %d and %d minutes.\n", azpim.MinMinutes, azpim.MaxMinutes)
			continue
		}
		if minutes%30 != 0 {
			fmt.Println("❌ Duration must be in 30-minute increments.")
			continue
		}
		return minutes, nil
	}
}

func formatDurationPrompt(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	if hours == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

func parseDurationPrompt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("duration cannot be empty")
	}

	// Try as plain number (minutes)
	if val, err := strconv.Atoi(s); err == nil {
		return val, nil
	}

	// Parse as duration string
	totalMinutes := 0
	current := ""

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' || ch == '.' {
			current += string(ch)
		} else if ch == 'h' || ch == 'H' {
			if current == "" {
				return 0, fmt.Errorf("missing number before 'h'")
			}
			var val float64
			if _, err := fmt.Sscanf(current, "%f", &val); err != nil {
				return 0, fmt.Errorf("invalid hours value")
			}
			totalMinutes += int(val * 60)
			current = ""
		} else if ch == 'm' || ch == 'M' {
			if current == "" {
				return 0, fmt.Errorf("missing number before 'm'")
			}
			var val float64
			if _, err := fmt.Sscanf(current, "%f", &val); err != nil {
				return 0, fmt.Errorf("invalid minutes value")
			}
			totalMinutes += int(val)
			current = ""
		} else if ch == ' ' {
			continue
		} else {
			return 0, fmt.Errorf("invalid character '%c' in duration", ch)
		}
	}

	if current != "" {
		return 0, fmt.Errorf("duration must end with 'h' or 'm' (e.g., '1h', '90m', '1h30m')")
	}

	if totalMinutes <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}

	return totalMinutes, nil
}

// PromptYesNo asks a yes/no question with a default answer.
func PromptYesNo(question string, defaultYes bool) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	var suffix string
	if defaultYes {
		suffix = "[Y/n]"
	} else {
		suffix = "[y/N]"
	}
	for {
		fmt.Printf("%s %s: ", question, suffix)
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read response: %w", err)
		}
		value := strings.ToLower(trimInput(input))
		if value == "" {
			return defaultYes, nil
		}
		if isQuit(value) {
			return false, azpim.ErrUserCancelled
		}
		switch value {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("❌ Please answer 'y' or 'n'.")
		}
	}
}

// PromptCSV captures a comma-separated list of values, trimming whitespace.
func PromptCSV(question string, existing []string) ([]string, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		prompt := question
		if len(existing) > 0 {
			prompt = fmt.Sprintf("%s [%s]", question, strings.Join(existing, ","))
		}
		fmt.Printf("%s (enter to skip, 'q' to cancel): ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read list: %w", err)
		}
		value := trimInput(input)
		if isQuit(value) {
			return nil, azpim.ErrUserCancelled
		}
		if value == "" {
			return append([]string(nil), existing...), nil
		}
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			result = append(result, trimmed)
		}
		return result, nil
	}
}

func isQuit(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower == "q" || lower == "quit"
}

// PromptConfirmActivation asks the user to confirm role activation
func PromptConfirmActivation(roleCount int) error {
	var question string
	if roleCount == 1 {
		question = "Proceed with role activation?"
	} else {
		question = fmt.Sprintf("Proceed with activating %d roles?", roleCount)
	}
	confirmed, err := PromptYesNo(question, false)
	if err != nil {
		return err
	}
	if !confirmed {
		return azpim.ErrUserCancelled
	}
	return nil
}

// PromptConfirmActivationDetailed shows detailed summary and asks for confirmation
func PromptConfirmActivationDetailed(roles []activationSummary, justification string, duration string) error {
	fmt.Println("\n📋 Ready to activate:")
	for _, r := range roles {
		fmt.Printf("  • %s @ %s\n", r.roleName, r.scopeDisplay)
	}
	fmt.Printf("  Duration: %s\n", duration)
	fmt.Printf("  Justification: %s\n", justification)

	confirmed, err := PromptYesNo("\nProceed with activation?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return azpim.ErrUserCancelled
	}
	return nil
}

type activationSummary struct {
	roleName     string
	scopeDisplay string
}
