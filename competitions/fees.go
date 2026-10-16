package competitions

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
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

// thousands is an amount whose separator is followed by three digits.
var thousands = regexp.MustCompile(`[.,]\d{3}$`)

// MaxPayment is the most one payment can be, in cents: a club's whole bill,
// not one fee.
const MaxPayment = 10000000

// ParseMoney reads an amount as typed, e.g. "12", "12.5" or "€12.50", in
// cents; "" is 0. It's a fee: at most MaxFee.
func ParseMoney(s string) (int, error) { return parseAmount(s, MaxFee) }

// ParsePayment is ParseMoney for a payment, up to MaxPayment.
func ParsePayment(s string) (int, error) { return parseAmount(s, MaxPayment) }

func parseAmount(s string, most int) (int, error) {
	s = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "€£"))
	if s == "" {
		return 0, nil
	}
	// "1,000" or "1.000" may be a thousand or one: ask, rather than take €1.
	if thousands.MatchString(s) {
		return 0, errors.New("write an amount without a thousands separator, like 1000 or 12.50")
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, errors.New("an amount should be a number, like 12 or 12.50")
	}
	if v > float64(most)/100 {
		return 0, fmt.Errorf("an amount can be at most %d", most/100)
	}
	cents := int(math.Round(v * 100))
	return cents, nil
}

// InvoiceLine is one thing charged.
type InvoiceLine struct {
	What   string // e.g. "Ann Ryan · BUCS L3"
	Amount int    // in cents
}

// Invoice is what one club or individual owes for their entries: a line for
// each entry, by gymnast and then discipline in the usual order, and the
// club fee for a club.
func (f Fees) Invoice(entries []Entry, club bool) ([]InvoiceLine, int) {
	var lines []InvoiceLine
	total := 0
	if club && f.PerClub > 0 && len(entries) > 0 {
		lines, total = append(lines, InvoiceLine{"Club fee", f.PerClub}), f.PerClub
	}
	entries = slices.Clone(entries)
	slices.SortStableFunc(entries, func(a, b Entry) int {
		return cmp.Or(CompareNames(a.Gymnast, b.Gymnast),
			cmp.Compare(slices.Index(AllDisciplines, a.Discipline), slices.Index(AllDisciplines, b.Discipline)))
	})
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
