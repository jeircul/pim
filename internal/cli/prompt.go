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
	for i, item := range items {
		keys[i] = keyFunc(item)
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
