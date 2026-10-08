package competitions

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Entry fees (roadmap 2026-10-08): what a competition charges, per entry by
// discipline (a synchro pair is one entry) and per club, and how to pay.
// The tools say what's owed and record what's paid; no money goes through
// them.
type Fees struct {
	Currency     string         `json:"currency,omitempty"`  // CurrencyEUR (the default) or CurrencyGBP
	PerEntry     map[string]int `json:"per_entry,omitempty"` // in cents, by discipline
	PerClub      int            `json:"per_club,omitempty"`  // in cents, once for each club entered
	Instructions string         `json:"instructions,omitempty"`
}

// The currencies fees can be in.
const (
	CurrencyEUR = ""
	CurrencyGBP = "GBP"
)

// MaxFee is the most any one fee can be, in cents.
const MaxFee = 100000

// On says whether the competition charges anything.
func (f Fees) On() bool {
	if f.PerClub > 0 {
		return true
	}
	for _, c := range f.PerEntry {
		if c > 0 {
			return true
		}
	}
	return false
}

// Money is an amount in cents in the fees' currency, e.g. "€12.50", or
// "€12" for whole amounts.
func (f Fees) Money(cents int) string {
	symbol := "€"
	if f.Currency == CurrencyGBP {
		symbol = "£"
	}
	sign := ""
	if cents < 0 {
		sign, cents = "−", -cents
	}
	whole := strconv.Itoa(cents / 100)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if cents%100 == 0 {
		return sign + symbol + whole
	}
	return fmt.Sprintf("%s%s%s.%02d", sign, symbol, whole, cents%100)
}

// ParseMoney reads an amount as typed, e.g. "12", "12.5" or "€12.50", in
// cents; "" is 0.
func ParseMoney(s string) (int, error) {
	s = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "€£"))
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, errors.New("an amount should be a number, like 12 or 12.50")
	}
	cents := int(math.Round(v * 100))
	if cents > MaxFee {
		return 0, fmt.Errorf("an amount can be at most %d", MaxFee/100)
	}
	return cents, nil
}

// InvoiceLine is one thing charged.
type InvoiceLine struct {
	What   string // e.g. "Ann Ryan · BUCS L3"
	Amount int    // in cents
}

// Invoice is what one club or individual owes for their entries: a line for
// each entry, and the club fee for a club.
func (f Fees) Invoice(entries []Entry, club bool) ([]InvoiceLine, int) {
	var lines []InvoiceLine
	total := 0
	if club && f.PerClub > 0 && len(entries) > 0 {
		lines, total = append(lines, InvoiceLine{"Club fee", f.PerClub}), f.PerClub
	}
	for _, e := range entries {
		amount := f.PerEntry[e.Discipline]
		if amount == 0 {
			continue
		}
		what := e.Gymnasts() + " · " + e.Event()
		lines, total = append(lines, InvoiceLine{what, amount}), total+amount
	}
	return lines, total
}
