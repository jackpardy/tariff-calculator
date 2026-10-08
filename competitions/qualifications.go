package competitions

import (
	"fmt"
	"slices"
)

// Coaching qualifications (ADR 0007 Decision 1): a governing body's level
// in a discipline. A competition asks for a level, and either body's
// qualification at that level or above meets it. The names follow the
// bodies' published pathways (2026) and are still to be confirmed.

// Governing bodies.
const (
	BodyBG = "bg" // British Gymnastics
	BodyGI = "gi" // Gymnastics Ireland
)

var bodyNames = map[string]string{BodyBG: "British Gymnastics", BodyGI: "Gymnastics Ireland"}

// Qualification is a coaching qualification.
type Qualification struct {
	Key        string // e.g. "bg-trampoline-2"
	Body       string
	Discipline string // Trampoline (which covers synchro), Tumbling or DMT
	Level      int    // 1 to 4
}

// Name is the qualification's name, e.g. "British Gymnastics Level 2
// Trampoline Coach".
func (q Qualification) Name() string {
	return fmt.Sprintf("%s Level %d %s Coach", bodyNames[q.Body], q.Level, DisciplineName(q.Discipline))
}

// Meets says whether it's enough for a discipline at a level: the same
// discipline (trampoline's for synchro), at that level or above.
func (q Qualification) Meets(discipline string, level int) bool {
	if discipline == Synchro {
		discipline = Trampoline
	}
	return q.Discipline == discipline && q.Level >= level
}

// Qualifications are every qualification, by body, discipline and level:
// British Gymnastics' levels 1 to 4, Gymnastics Ireland's 1 to 3.
var Qualifications = func() []Qualification {
	var out []Qualification
	for _, body := range []string{BodyBG, BodyGI} {
		top := map[string]int{BodyBG: 4, BodyGI: 3}[body]
		for _, d := range []string{Trampoline, Tumbling, DMT} {
			for level := 1; level <= top; level++ {
				name := d
				if d == Trampoline {
					name = "trampoline"
				}
				out = append(out, Qualification{Key: fmt.Sprintf("%s-%s-%d", body, name, level), Body: body, Discipline: d, Level: level})
			}
		}
	}
	return out
}()

// QualificationByKey is the qualification with a key.
func QualificationByKey(key string) (Qualification, bool) {
	i := slices.IndexFunc(Qualifications, func(q Qualification) bool { return q.Key == key })
	if i < 0 {
		return Qualification{}, false
	}
	return Qualifications[i], true
}

// DefaultCoachLevel is the level a competition asks for to start with: the
// first that coaches somersaults and runs a session alone.
const DefaultCoachLevel = 2
