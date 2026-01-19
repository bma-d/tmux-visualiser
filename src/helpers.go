package main

import "sort"

func orderedSessionNames(state appState) []string {
	names := make([]string, 0, len(state.sessions))
	for name := range state.sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func focusIndexForName(names []string, name string) int {
	if name == "" {
		return -1
	}
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}
