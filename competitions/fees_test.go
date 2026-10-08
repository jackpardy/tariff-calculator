package competitions

import "testing"

func TestFees(t *testing.T) {
	f := Fees{PerEntry: map[string]int{Trampoline: 1200, Synchro: 1550}, PerClub: 2500}
	if got := f.Money(1200); got != "€12" || f.Money(873400) != "€8,734" {
		t.Errorf("whole euros: %s", got)
	}
	if got := (Fees{Currency: CurrencyGBP}).Money(1550); got != "£15.50" {
		t.Errorf("pence: %s", got)
	}
	for in, want := range map[string]int{"12": 1200, "12.5": 1250, "€12.50": 1250, "12,50": 1250, "": 0} {
		if got, err := ParseMoney(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"-1", "abc", "5000"} {
		if _, err := ParseMoney(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	entries := []Entry{
		{Gymnast: "Ann", Level: "BUCS L3"},
		{Gymnast: "Bea", Discipline: Synchro, Level: "BUCS L3", Partner: &Partner{Name: "Cat"}},
		{Gymnast: "Dee", Discipline: Tumbling, Level: "Novice"}, // no tumbling fee
	}
	lines, total := f.Invoice(entries, true)
	if total != 2500+1200+1550 || len(lines) != 3 || lines[0].What != "Club fee" || lines[1].What != "Ann · BUCS L3" {
		t.Errorf("the club's invoice: %v %d", lines, total)
	}
	if _, total := f.Invoice(entries[:1], false); total != 1200 {
		t.Errorf("an individual pays no club fee: %d", total)
	}
	if (Fees{}).On() || !f.On() {
		t.Error("whether anything's charged")
	}
}
