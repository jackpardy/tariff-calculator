package skills

import (
	"fmt"
	"strings"
)

// ElementNotes are what the validation flags on element i (from 0) as
// plain sentences, for gymnasts and comp secs to read: "10th Must Land
// Feet" is "Element 10 must land on feet." msg is the element's message in
// RoutineValidation.Messages, its labels joined by " / " as the calculator
// shows them; "" has no notes.
func ElementNotes(i int, msg string) []string {
	if msg == "" {
		return nil
	}
	var out []string
	for _, label := range strings.Split(msg, " / ") {
		out = append(out, fmt.Sprintf("Element %d %s.", i+1, elementNote(i, label)))
	}
	return out
}

// ElementNote is element i's notes as one line for the element's own row,
// where its number shows already: "Must land on feet."
func ElementNote(i int, msg string) string {
	var out []string
	for _, label := range strings.Split(msg, " / ") {
		if label != "" {
			note := elementNote(i, label)
			out = append(out, strings.ToUpper(note[:1])+note[1:]+".")
		}
	}
	return strings.Join(out, " ")
}

// elementNote is a validation label as the rest of a sentence about the
// element.
func elementNote(i int, label string) string {
	if from, ok := strings.CutPrefix(label, "Bad Transition: "); ok {
		landed, takeoff, _ := strings.Cut(from, " -> ")
		return fmt.Sprintf("takes off from %s, but element %d lands on %s", positionWord(takeoff), i, positionWord(landed))
	}
	switch label {
	case "Duplicate (Counts Once)":
		return "is repeated later, so its difficulty counts once"
	case "Duplicate":
		return "repeats an earlier element, so its difficulty doesn't count again"
	case "Can't Repeat: Scored In 1st Exercise":
		return "scored difficulty in the first exercise, so it doesn't count again"
	case "Must Start From Feet":
		return "must take off from feet"
	case "Straight Jump Interrupts Routine":
		return "is a straight jump, which ends the routine there"
	case "Straight Jump":
		return "is a straight jump"
	case "Invalid Landing Interrupts Routine":
		return "lands in a position that isn't allowed, which ends the routine there"
	case "Invalid Landing":
		return "lands in a position that isn't allowed"
	case "After Interruption (No Tariff)":
		return "comes after the routine ended, so its difficulty doesn't count"
	case "10th Must Land Feet":
		return "must land on feet"
	case "Skill >10 (No Tariff)":
		return "is past the tenth, so its difficulty doesn't count"
	}
	return strings.ToLower(label)
}

// positionWord is a body position's name, as a sentence says it.
func positionWord(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// RoutineNotes are the notes on a whole routine (messages: each element's,
// as RoutineValidation.Messages has them), for a list of its problems: each
// element's as ElementNotes has them, except that what follows from one
// cause isn't said over and over. Once the routine has ended early (a
// straight jump, a landing that isn't allowed), the elements after it are
// one sentence ("Elements 3 to 11 come after the routine ended…") and
// nothing else is said about them: not that they repeat an element, are
// past the tenth or must land on feet, nor that an earlier element counts
// once for being repeated only after the end. The elements past the tenth
// are one sentence too.
func RoutineNotes(messages []string) []string {
	labels := make([][]string, len(messages))
	ended := len(messages) // the first element after the end, if the routine ended early
	repeats := 0           // elements repeating an earlier one, before the end
	for i, msg := range messages {
		if msg != "" {
			labels[i] = strings.Split(msg, " / ")
		}
		for _, label := range labels[i] {
			if (label == "Straight Jump Interrupts Routine" || label == "Invalid Landing Interrupts Routine") && i+1 < ended {
				ended = i + 1
			}
		}
	}
	for i := range min(ended, len(labels)) {
		for _, label := range labels[i] {
			if label == "Duplicate" {
				repeats++
			}
		}
	}
	var past []int
	var out []string
	for i := range min(ended, len(labels)) {
		var kept []string
		for _, label := range labels[i] {
			switch {
			case label == "Skill >10 (No Tariff)":
				past = append(past, i)
			case label == "Duplicate (Counts Once)" && repeats == 0:
				// Repeated only after the end, which doesn't count.
			default:
				kept = append(kept, label)
			}
		}
		if len(kept) > 0 {
			out = append(out, ElementNotes(i, strings.Join(kept, " / "))...)
		}
	}
	if ended < len(messages) {
		var after []int
		for i := ended; i < len(messages); i++ {
			after = append(after, i)
		}
		out = append(out, groupedNote(after, "After Interruption (No Tariff)", "come after the routine ended, so their difficulty doesn't count"))
	}
	if len(past) > 0 {
		out = append(out, groupedNote(past, "Skill >10 (No Tariff)", "are past the tenth, so their difficulty doesn't count"))
	}
	return out
}

// groupedNote is one sentence about several elements (from 0) flagged the
// same: label's note for one, or plural for more ("Elements 3 to 11 …").
func groupedNote(elements []int, label, plural string) string {
	if len(elements) == 1 {
		return fmt.Sprintf("Element %d %s.", elements[0]+1, elementNote(elements[0], label))
	}
	return "Elements " + elementsPhrase(elements) + " " + plural + "."
}

// elementsPhrase names elements (from 0, in order) as a sentence does: runs
// as "3 to 11", the rest listed, "3, 5 and 7".
func elementsPhrase(elements []int) string {
	var parts []string
	for i := 0; i < len(elements); {
		j := i
		for j+1 < len(elements) && elements[j+1] == elements[j]+1 {
			j++
		}
		switch {
		case j-i >= 2:
			parts = append(parts, fmt.Sprintf("%d to %d", elements[i]+1, elements[j]+1))
		case j > i:
			parts = append(parts, fmt.Sprint(elements[i]+1), fmt.Sprint(elements[j]+1))
		default:
			parts = append(parts, fmt.Sprint(elements[i]+1))
		}
		i = j + 1
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
