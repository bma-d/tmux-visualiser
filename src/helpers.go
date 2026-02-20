package main

import "sort"

func orderedSessionNames(state appState) []string {
	names := make([]string, 0, len(state.sessions))
	for name := range state.sessions {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left, lok := state.sessions[names[i]]
		right, rok := state.sessions[names[j]]
		if !lok || !rok {
			return names[i] < names[j]
		}
		if left.name == right.name {
			leftSocket := socketKey(left.socketPath)
			rightSocket := socketKey(right.socketPath)
			if leftSocket == rightSocket {
				return names[i] < names[j]
			}
			return leftSocket < rightSocket
		}
		return left.name < right.name
	})
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
