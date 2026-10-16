package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Judging qualifications (ADR 0014): a judge gives theirs, each with its
// certificate, beside their offer to judge; the organiser opens them on
// Officials before marking the judge qualified. A club member's are kept
// with them (the member, or the comp sec for them, adds them); an
// individual's with their entry.

func (p *competitionPages) registerJudging(handle func(string, http.HandlerFunc)) {
	handle("POST /clubs/member/{token}/judging", p.addOwnJudging)
	handle("POST /clubs/member/{token}/judging/{q}/remove", p.removeOwnJudging)
	handle("GET /clubs/member/{token}/judging/{q}", p.ownJudgingCertificate)
	handle("POST /clubs/admin/{token}/members/{member}/judging", p.addMemberJudging)
	handle("POST /clubs/admin/{token}/members/{member}/judging/{q}/remove", p.removeMemberJudging)
	handle("GET /clubs/admin/{token}/members/{member}/judging/{q}", p.memberJudgingCertificate)
	handle("POST /competitions/entry/{token}/judging", p.addEntryJudging)
	handle("POST /competitions/entry/{token}/judging/{q}/remove", p.removeEntryJudging)
	handle("GET /competitions/entry/{token}/judging/{q}", p.entryJudgingCertificate)
	handle("GET /competitions/admin/{token}/officials/{id}/judging/{q}", p.officialJudgingCertificate)
}

// judgingOptions are the judging qualifications to choose from.
func judgingOptions() []views.FlightOption {
	var out []views.FlightOption
	for _, q := range competitions.JudgeQualifications {
		out = append(out, views.FlightOption{Value: q.Key, Label: q.Name()})
	}
	return out
}

// judgingName names a judging qualification by its key, or says it's no
// longer on the list.
func judgingName(key string) string {
	if q, ok := competitions.JudgeQualificationByKey(key); ok {
		return q.Name()
	}
	return "A qualification no longer on the list"
}

// judgingBox is a judge's qualifications as their offer shows them: added at
// base, each at base/<id>, coming back to back; with the organiser's
// approval where the competition approves judges (sent: the organiser has
// their offer; qualified: and marked them qualified).
func judgingBox(c store.Competition, quals []store.JudgeQualification, base, back, whose, kept string, sent, qualified bool) *views.JudgingBox {
	b := &views.JudgingBox{Whose: whose, Add: base, Back: back, Options: judgingOptions(), Full: len(quals) >= store.MaxQualifications, Kept: kept}
	for _, q := range quals {
		b.List = append(b.List, views.JudgingRow{Name: judgingName(q.Qualification), Certificate: base + "/" + q.ID, Remove: base + "/" + q.ID + "/remove"})
	}
	if c.Officials.Judge == competitions.JudgeQualified {
		switch {
		case sent && qualified:
			b.Status, b.Done = "Approved to judge by the organiser", true
		case sent:
			b.Status = "The organiser approves judges: waiting for them"
		default:
			b.Status = "The organiser approves judges: they see these with your offer"
		}
	}
	return b
}

// judgingBack is where a judging form comes back to: the page it was on (a
// path on this site), at its qualifications, else fallback.
func judgingBack(r *http.Request, fallback string) string {
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") || strings.ContainsAny(back, "\\#?") {
		back = fallback
	}
	return back + "#judging"
}

// addJudging adds the posted qualification and certificate with add,
// going back with what happened.
func addJudging(w http.ResponseWriter, r *http.Request, fallback, full string, add func(key, certType string, data []byte) error) {
	back := func(notice string) { seeOther(w, r, judgingBack(r, fallback), notice) }
	if problem := parseUpload(r); problem != "" {
		back("Nothing was added: " + problem)
		return
	}
	q, ok := competitions.JudgeQualificationByKey(r.FormValue("qualification"))
	if !ok {
		back("Nothing was added: choose the judging qualification.")
		return
	}
	data, certType, problem := postedFile(r)
	if problem != "" {
		back("Nothing was added: " + problem)
		return
	}
	err := add(q.Key, certType, data)
	switch {
	case errors.Is(err, store.ErrLimit):
		back(fmt.Sprintf("Nothing was added: a judge can have %d qualifications at most. Remove one first.", store.MaxQualifications))
	case errors.Is(err, store.ErrCertificatesFull):
		back("Nothing was added: " + full)
	case err != nil && certificateRefused(err) != "":
		back("Nothing was added: " + certificateRefused(err))
	case err != nil:
		failed(w, r, err)
	default:
		back("Added " + q.Name() + ".")
	}
}

// removedJudging goes back after removing a qualification.
func removedJudging(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, judgingBack(r, fallback), "Removed.")
}

// certificateOrFail serves a certificate the store found, or says why not.
func certificateOrFail(w http.ResponseWriter, r *http.Request, certType string, data []byte, err error) {
	if err != nil {
		failed(w, r, err)
		return
	}
	serveCertificate(w, certType, data)
}

const clubFull = "your club's certificates already take up the room a club has for them. Remove one, or add a smaller photo or PDF."

func (p *competitionPages) addOwnJudging(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	addJudging(w, r, memberPath(r.PathValue("token")), clubFull, func(key, certType string, data []byte) error {
		return p.st.AddMemberJudgeQualification(r.Context(), m.ClubID, m.ID, key, certType, data)
	})
}

func (p *competitionPages) removeOwnJudging(w http.ResponseWriter, r *http.Request) {
	if m, ok := p.member(w, r); ok {
		removedJudging(w, r, p.st.RemoveMemberJudgeQualification(r.Context(), m.ID, r.PathValue("q")), memberPath(r.PathValue("token")))
	}
}

func (p *competitionPages) ownJudgingCertificate(w http.ResponseWriter, r *http.Request) {
	if m, ok := p.member(w, r); ok {
		certType, data, err := p.st.MemberJudgeCertificate(r.Context(), m.ID, r.PathValue("q"))
		certificateOrFail(w, r, certType, data, err)
	}
}

// clubMember is the member the comp sec's address names.
func (p *competitionPages) clubMember(w http.ResponseWriter, r *http.Request) (store.Member, bool) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return store.Member{}, false
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return m, false
	}
	return m, true
}

func (p *competitionPages) addMemberJudging(w http.ResponseWriter, r *http.Request) {
	m, ok := p.clubMember(w, r)
	if !ok {
		return
	}
	addJudging(w, r, clubMembersPath(r.PathValue("token")), clubFull, func(key, certType string, data []byte) error {
		return p.st.AddMemberJudgeQualification(r.Context(), m.ClubID, m.ID, key, certType, data)
	})
}

func (p *competitionPages) removeMemberJudging(w http.ResponseWriter, r *http.Request) {
	if m, ok := p.clubMember(w, r); ok {
		removedJudging(w, r, p.st.RemoveMemberJudgeQualification(r.Context(), m.ID, r.PathValue("q")), clubMembersPath(r.PathValue("token")))
	}
}

func (p *competitionPages) memberJudgingCertificate(w http.ResponseWriter, r *http.Request) {
	if m, ok := p.clubMember(w, r); ok {
		certType, data, err := p.st.MemberJudgeCertificate(r.Context(), m.ID, r.PathValue("q"))
		certificateOrFail(w, r, certType, data, err)
	}
}

func (p *competitionPages) addEntryJudging(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := p.own(w, r); !ok {
		return
	}
	token := r.PathValue("token")
	addJudging(w, r, "/competitions/entry/"+token, "the certificates of this competition's individuals already take up the room it has for them. Add a smaller photo or PDF.",
		func(key, certType string, data []byte) error {
			return p.st.AddEntryJudgeQualification(r.Context(), token, key, certType, data)
		})
}

func (p *competitionPages) removeEntryJudging(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := p.own(w, r); ok {
		token := r.PathValue("token")
		removedJudging(w, r, p.st.RemoveEntryJudgeQualification(r.Context(), token, r.PathValue("q")), "/competitions/entry/"+token)
	}
}

func (p *competitionPages) entryJudgingCertificate(w http.ResponseWriter, r *http.Request) {
	if e, _, ok := p.own(w, r); ok {
		certType, data, err := p.st.EntryJudgeCertificate(r.Context(), e.ID, r.PathValue("q"))
		certificateOrFail(w, r, certType, data, err)
	}
}

// officialJudgingCertificate is an official's certificate, for the organiser
// (and links that see officials).
func (p *competitionPages) officialJudgingCertificate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	certType, data, err := p.st.OfficialJudgeCertificate(r.Context(), c.ID, r.PathValue("id"), r.PathValue("q"))
	certificateOrFail(w, r, certType, data, err)
}

// memberJudging is a member's judging box on their offer at a competition
// (by the comp sec when whose names them).
func (p *competitionPages) memberJudging(ctx context.Context, c store.Competition, m store.Member, base, back, whose string) (*views.JudgingBox, error) {
	quals, err := p.st.MemberJudgeQualifications(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	sent, qualified, err := p.st.MemberOfficial(ctx, m.ID, c.ID)
	if err != nil {
		return nil, err
	}
	return judgingBox(c, quals, base, back, whose, "It's kept at the club, for its competitions, until removed; only the comp sec and the organisers of competitions it sends the offer to can open it.", sent, qualified), nil
}

// entryJudging is an individual's judging box on their offer.
func (p *competitionPages) entryJudging(ctx context.Context, c store.Competition, e store.Entry, base, back string) (*views.JudgingBox, error) {
	quals, err := p.st.EntryJudgeQualifications(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	made, qualified, err := p.st.EntryOfficial(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	return judgingBox(c, quals, base, back, "Your", "It's kept with your entry, and deleted with the competition; only its organiser can open it.", made, qualified), nil
}
