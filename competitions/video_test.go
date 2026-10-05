package competitions

import (
	"strings"
	"testing"

	"tariffCalculator/requirements"
	"tariffCalculator/skills"
)

func ptr[T any](v T) *T { return &v }

func TestVideoValidate(t *testing.T) {
	c := competition(t)
	c.Video = Video{Need: VideoSkills}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "needs to say which") {
		t.Errorf("video for some skills needs matchers: %v", err)
	}
	c.Video = Video{Need: "sometimes"}
	if err := c.Validate(); err == nil {
		t.Error("an unknown kind of video proof")
	}
	c.Video = Video{Need: VideoSkills, Skills: []requirements.Matcher{{Rotation: &requirements.Range{Min: ptr(12)}, Label: "any triple"}, {Tariff: &requirements.TariffRange{Min: ptr(1.5)}}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := c.Video.Describe(); got != "Video of any triple, or tariff at least 1.5" {
		t.Errorf("described as %q", got)
	}
}

func TestVideoNeeds(t *testing.T) {
	c := competition(t)
	tripleBack := skills.TrampolineSkill{Rotation: 12, TwistDistribution: []int{0, 0, 0}, TakeoffPosition: skills.Feet, Shape: skills.Tuck, Backward: true}
	e := Entry{Gymnast: "B", Level: "FIG AG3 (17–21)", Exercises: [2]Exercise{
		{Skills: []skills.TrampolineSkill{backTuck, tripleBack}},
		{Skills: []skills.TrampolineSkill{barani}},
	}}
	if err := c.ValidateEntry(&e); err != nil {
		t.Fatal(err)
	}
	card, _ := c.Check(e)

	if needs := c.VideoNeeds(card); needs[0].Needed || needs[1].Needed {
		t.Error("no video asked for")
	}
	c.Video = Video{Need: VideoSkills, Skills: []requirements.Matcher{{Rotation: &requirements.Range{Min: ptr(12)}}}}
	needs := c.VideoNeeds(card)
	if !needs[0].Needed || len(needs[0].Skills) != 1 || !strings.Contains(needs[0].Skills[0], "Triple") || needs[1].Needed {
		t.Errorf("the triple needs video, nothing in the second exercise: %+v", needs)
	}
	if !VideoMissing(e, needs) {
		t.Error("missing until linked")
	}
	e.Exercises[0].Video = "https://youtu.be/abc"
	if VideoMissing(e, needs) {
		t.Error("linked")
	}
	c.Video = Video{Need: VideoRoutine}
	if needs := c.VideoNeeds(card); !needs[0].Needed || !needs[1].Needed || needs[0].Skills != nil {
		t.Errorf("both whole routines: %+v", needs)
	}
}

func TestCheckVideoLink(t *testing.T) {
	for _, ok := range []string{"https://www.youtube.com/watch?v=abc", "https://youtu.be/abc", "https://m.youtube.com/watch?v=abc",
		"https://drive.google.com/file/d/x/view", "https://vimeo.com/123", "https://www.dropbox.com/s/x/v.mp4", "https://1drv.ms/v/s!x", "https://ucd-my.sharepoint.com/x"} {
		if err := CheckVideoLink(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://youtu.be/abc", "youtu.be/abc", "https://example.com/v.mp4", "https://youtube.com.evil.example/x",
		"https://user@youtube.com/x", "javascript:alert(1)", "https://notyoutube.com/x", "https://youtu.be/" + strings.Repeat("a", MaxVideoLink)} {
		if err := CheckVideoLink(bad); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}

	c := competition(t)
	e := Entry{Gymnast: "B", Level: "FIG AG3 (17–21)", Exercises: [2]Exercise{{Video: " https://example.com/x "}, {VideoNote: strings.Repeat("x", MaxVideoNote+1)}}}
	err := c.ValidateEntry(&e)
	if err == nil || !strings.Contains(err.Error(), "first exercise's video: the link should be to YouTube") || !strings.Contains(err.Error(), "video note is 201 characters") {
		t.Errorf("entries check their video links: %v", err)
	}
}
