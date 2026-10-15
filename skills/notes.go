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
