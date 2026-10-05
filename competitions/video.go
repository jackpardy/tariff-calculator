package competitions

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"tariffCalculator/requirements"
)

// Video proof (ADR 0004 Decision 10): a competition can ask for video showing
// a gymnast can perform their routine safely. It's a link to a video hosted
// elsewhere, never an upload: the app records the link and the organiser's
// review, nothing more.

const (
	// VideoNone, VideoSkills and VideoRoutine are what a competition asks for:
	// no video, video of the skills its matchers describe, or of the whole routine.
	VideoNone    = ""
	VideoSkills  = "skills"
	VideoRoutine = "routine"

	// MaxVideoLink and MaxVideoNote bound a video link and its note, in characters.
	MaxVideoLink = 500
	MaxVideoNote = 200
)

// Video is what video proof a competition asks for.
type Video struct {
	Need   string                 `json:"need,omitempty"`   // VideoNone, VideoSkills or VideoRoutine
	Skills []requirements.Matcher `json:"skills,omitempty"` // with VideoSkills: a skill matching any of these needs video
}

func (v Video) validate() error {
	switch v.Need {
	case VideoNone, VideoRoutine:
		return nil
	case VideoSkills:
		if len(v.Skills) == 0 {
			return errors.New("video for some skills needs to say which")
		}
		var errs []error
		for _, m := range v.Skills {
			if err := m.Validate(); err != nil {
				errs = append(errs, fmt.Errorf("video skills: %w", err))
			}
		}
		return errors.Join(errs...)
	}
	return fmt.Errorf("unknown video proof %q", v.Need)
}

// Describe says what video a competition asks for, e.g. "Video of any triple
// somersault, or any skill of tariff 1.5 or more".
func (v Video) Describe() string {
	switch v.Need {
	case VideoRoutine:
		return "Video of each whole routine"
	case VideoSkills:
		var parts []string
		for _, m := range v.Skills {
			parts = append(parts, requirements.DescribeMatcher(m))
		}
		return "Video of " + strings.Join(parts, ", or ")
	}
	return ""
}

// VideoNeed is the video one exercise of an entry needs.
type VideoNeed struct {
	Needed bool
	Skills []string // the skills that need it, by name; none when the whole routine does
}

// VideoNeeds is the video each exercise of a checked entry needs. A set
// routine performed as written still needs video of the whole routine if the
// competition asks for it.
func (c Competition) VideoNeeds(card Card) [2]VideoNeed {
	var out [2]VideoNeed
	for i, ex := range []requirements.Checked{card.First, card.Second} {
		switch c.Video.Need {
		case VideoRoutine:
			out[i].Needed = len(ex.Validation.Skills) > 0
		case VideoSkills:
			for _, sv := range ex.Validation.Skills {
				for _, m := range c.Video.Skills {
					if m.Matches(sv.Skill) {
						out[i].Needed = true
						out[i].Skills = append(out[i].Skills, sv.Skill.Name)
						break
					}
				}
			}
		}
	}
	return out
}

// VideoMissing says whether an exercise that needs video has no link.
func VideoMissing(e Entry, needs [2]VideoNeed) bool {
	for i, need := range needs {
		if need.Needed && e.Exercises[i].Video == "" {
			return true
		}
	}
	return false
}

// videoHosts are where a video link can point (ADR 0004 Decision 10).
var videoHosts = []string{"youtube.com", "youtu.be", "drive.google.com", "vimeo.com", "dropbox.com", "onedrive.live.com", "1drv.ms", "sharepoint.com"}

// CheckVideoLink reports a link that isn't an https URL on a known video host.
// The video itself is never fetched.
func CheckVideoLink(link string) error {
	if len(link) > MaxVideoLink {
		return fmt.Errorf("the link is too long (the most is %d characters)", MaxVideoLink)
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("paste the video's full link, starting https://")
	}
	host := strings.ToLower(u.Hostname())
	for _, known := range videoHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return nil
		}
	}
	return errors.New("the link should be to YouTube, Google Drive, Vimeo, Dropbox or OneDrive")
}
