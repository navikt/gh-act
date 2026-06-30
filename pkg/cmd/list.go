package cmd

import "fmt"

// ListActions prints every external action reference found in the repository's
// workflow and composite-action files. It performs no network calls.
func ListActions(opts CollectOptions) error {
	actions, err := findActions(opts)
	if err != nil {
		return fmt.Errorf("find actions: %w", err)
	}

	for _, action := range actions {
		comment := ""
		if action.Node.LineComment != "" {
			comment = " " + action.Node.LineComment
		}

		fmt.Printf("%s:%d:%d: %s%s\n", action.FilePath, action.Node.Line, action.Node.Column, action.Node.Value, comment)
	}

	return nil
}

// findActions discovers and parses every action reference across all workflow
// and composite-action files.
func findActions(opts CollectOptions) ([]Action, error) {
	_, refs, err := collectActionRefs(opts)
	if err != nil {
		return nil, err
	}

	return refs, nil
}
