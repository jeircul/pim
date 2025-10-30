package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jeircul/pim/pkg/azpim"
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
