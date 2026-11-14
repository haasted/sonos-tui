// The filter every list modal shares: / opens a box, what the reader types
// narrows the list, and esc clears it before it closes the modal. What counts
// as a match, and what makes two rows the same item, stay with the content.
package main

import (
	"fmt"
	"strings"
)

// listFilter is how one kind of list is filtered: match says whether an item
// answers what was typed, and same tells one item from another, so the
// cursor can stay on its item while the list narrows around it.
type listFilter[T any] struct {
	match func(item T, text string) bool
	same  func(a, b T) bool
}

// ClaimsEsc keeps esc from closing the modal while there is a filter to
// clear. The reader's first esc undoes the narrowing; a second closes it.
func (l listView[T]) ClaimsEsc() bool { return l.filtering || l.filter != "" }

// everything is the whole list, filtered or not.
func (l listView[T]) everything() []T {
	if l.all != nil {
		return l.all
	}
	return l.items
}

// applyFilter narrows items to those that answer the filter, keeping the
// whole list in all. The cursor keeps the item it was on where that item is
// still shown, and otherwise goes to the top: a cursor left on a row number
// would land on a different item every keystroke.
func (l listView[T]) applyFilter(rule listFilter[T]) listView[T] {
	var on *T
	if l.cursor < len(l.items) {
		on = &l.items[l.cursor]
	}

	l = l.narrow(rule)

	l.cursor = 0
	if on != nil {
		for i, item := range l.items {
			if rule.same(item, *on) {
				l.cursor = i
				break
			}
		}
	}
	return l.follow()
}

// narrow is applyFilter without the cursor keeping, for a list replaced from
// outside: whatever replaced it settles the cursor itself.
func (l listView[T]) narrow(rule listFilter[T]) listView[T] {
	all := l.everything()
	if l.filter == "" {
		l.items, l.all = all, nil
		return l
	}

	shown := make([]T, 0, len(all))
	for _, item := range all {
		if rule.match(item, l.filter) {
			shown = append(shown, item)
		}
	}
	l.items, l.all = shown, all
	return l
}

// filterKey is one key while the filter box is open. enter and down both
// leave the box and keep the narrowing, so the keys that act on rows reach
// the rows it found: down because the reader's next move after typing is
// into the list, and reaching for it should be enough.
func (l listView[T]) filterKey(rule listFilter[T], key string) listView[T] {
	switch key {
	case "esc":
		return l.closeFilter(rule)
	case "enter", "down":
		l.filtering = false
		return l
	case "backspace":
		return l.typeFilter(rule, "backspace")
	}
	if len([]rune(key)) == 1 {
		return l.typeFilter(rule, key)
	}
	return l
}

// openFilter starts taking the reader's keys as a filter, from nothing. A box
// that opened holding the last search would narrow the list before the reader
// had asked for anything.
func (l listView[T]) openFilter(rule listFilter[T]) listView[T] {
	l.filtering, l.filter = true, ""
	return l.applyFilter(rule)
}

// closeFilter stops filtering and shows the whole list again.
func (l listView[T]) closeFilter(rule listFilter[T]) listView[T] {
	if !l.filtering && l.filter == "" {
		return l
	}
	l.filtering, l.filter = false, ""
	return l.applyFilter(rule)
}

// typeFilter adds to what the reader has typed, or takes the last rune back.
func (l listView[T]) typeFilter(rule listFilter[T], key string) listView[T] {
	switch key {
	case "backspace":
		if l.filter == "" {
			return l
		}
		runes := []rune(l.filter)
		l.filter = string(runes[:len(runes)-1])
	default:
		l.filter += key
	}
	return l.applyFilter(rule)
}

// anyContains reports whether any of fields holds text, ignoring case. Empty
// text matches everything.
func anyContains(text string, fields ...string) bool {
	want := strings.ToLower(text)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), want) {
			return true
		}
	}
	return false
}

// matchLabel is how much of the list is showing.
func (l listView[T]) matchLabel() string {
	return fmt.Sprintf("%d of %d", len(l.items), len(l.everything()))
}
