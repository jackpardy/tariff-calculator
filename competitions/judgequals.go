package competitions

import (
	"fmt"
	"slices"
)

// Judging qualifications (ADR 0014): a governing body's judging level in a
// discipline, as coaching qualifications are (ADR 0007 Decision 1). A judge
// gives theirs, with its certificate, and the organiser looks at them before
// marking the judge qualified. The names follow the bodies' published
// pathways (2026) and are still to be confirmed.

// JudgeQualification is a judging qualification.
type JudgeQualification struct {
	Key        string // e.g. "bg-trampoline-judge-2"
	Body       string
	Discipline string // Trampoline (which covers synchro), Tumbling or DMT
	Level      int    // 1 up
}

// Name is the qualification's name, e.g. "British Gymnastics Level 2
// Trampoline Judge".
func (q JudgeQualification) Name() string {
	return fmt.Sprintf("%s Level %d %s Judge", bodyNames[q.Body], q.Level, DisciplineName(q.Discipline))
}

// Covers says whether it's a qualification to judge a discipline (a
// trampoline one covers synchro).
func (q JudgeQualification) Covers(discipline string) bool {
	if discipline == Synchro {
		discipline = Trampoline
	}
	return q.Discipline == discipline
}

// JudgeQualifications are every judging qualification, by body, discipline
// and level: British Gymnastics' levels 1 to 4, Gymnastics Ireland's 1 to 3.
var JudgeQualifications = func() []JudgeQualification {
	var out []JudgeQualification
	for _, body := range []string{BodyBG, BodyGI} {
		top := map[string]int{BodyBG: 4, BodyGI: 3}[body]
		for _, d := range []string{Trampoline, Tumbling, DMT} {
			for level := 1; level <= top; level++ {
				name := d
				if d == Trampoline {
					name = "trampoline"
				}
				out = append(out, JudgeQualification{Key: fmt.Sprintf("%s-%s-judge-%d", body, name, level), Body: body, Discipline: d, Level: level})
			}
		}
	}
	return out
}()

// JudgeQualificationByKey is the judging qualification with a key.
func JudgeQualificationByKey(key string) (JudgeQualification, bool) {
	i := slices.IndexFunc(JudgeQualifications, func(q JudgeQualification) bool { return q.Key == key })
	if i < 0 {
		return JudgeQualification{}, false
	}
	return JudgeQualifications[i], true
}
