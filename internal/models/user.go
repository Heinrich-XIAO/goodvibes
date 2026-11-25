// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import "fmt"

// TrustLevel enum mirrors app/models/user.rb.
type TrustLevel int

const (
	TrustLevelBlue   TrustLevel = iota // 0: unscored
	TrustLevelRed                      // 1: convicted
	TrustLevelGreen                    // 2: trusted
	TrustLevelYellow                   // 3: suspected (invisible to user)
)

func (t TrustLevel) String() string {
	switch t {
	case TrustLevelBlue:
		return "blue"
	case TrustLevelRed:
		return "red"
	case TrustLevelGreen:
		return "green"
	case TrustLevelYellow:
		return "yellow"
	default:
		return fmt.Sprintf("TrustLevel(%d)", t)
	}
}

func ParseTrustLevel(s string) (TrustLevel, bool) {
	switch s {
	case "blue":
		return TrustLevelBlue, true
	case "red":
		return TrustLevelRed, true
	case "green":
		return TrustLevelGreen, true
	case "yellow":
		return TrustLevelYellow, true
	default:
		return TrustLevelBlue, false
	}
}

// AdminLevel enum mirrors app/models/user.rb.
type AdminLevel int

const (
	AdminLevelDefault    AdminLevel = iota // 0: pleebs
	AdminLevelSuperadmin                   // 1
	AdminLevelAdmin                        // 2
	AdminLevelViewer                       // 3
)

func (a AdminLevel) String() string {
	switch a {
	case AdminLevelDefault:
		return "default"
	case AdminLevelSuperadmin:
		return "superadmin"
	case AdminLevelAdmin:
		return "admin"
	case AdminLevelViewer:
		return "viewer"
	default:
		return fmt.Sprintf("AdminLevel(%d)", a)
	}
}

// HackatimeExtensionTextType enum mirrors app/models/user.rb.
type HackatimeExtensionTextType int

const (
	ExtensionSimpleText     HackatimeExtensionTextType = iota // 0
	ExtensionClockEmoji                                       // 1
	ExtensionComplimentText                                   // 2
)

func (e HackatimeExtensionTextType) String() string {
	switch e {
	case ExtensionSimpleText:
		return "simple_text"
	case ExtensionClockEmoji:
		return "clock_emoji"
	case ExtensionComplimentText:
		return "compliment_text"
	default:
		return fmt.Sprintf("Extension(%d)", e)
	}
}

// User mirrors app/models/user.rb + Hack Club Auth fields.
type User struct {
	ID                         int64                      `json:"id"`
	HackClubID                 string                     `json:"hackclub_id,omitempty"`
	Username                   string                     `json:"username"`
	DisplayName                string                     `json:"display_name"`
	Email                      string                     `json:"email,omitempty"`
	SlackUID                   string                     `json:"slack_uid,omitempty"`
	GithubUID                  string                     `json:"github_uid,omitempty"`
	GithubUsername             string                     `json:"github_username,omitempty"`
	Timezone                   string                     `json:"timezone"`
	CountryCode                string                     `json:"country_code,omitempty"`
	VerificationStatus         string                     `json:"verification_status,omitempty"`
	YSWSEligible               bool                       `json:"ysws_eligible"`
	Bio                        string                     `json:"bio"`
	AllowPublicStatsLookup     bool                       `json:"allow_public_stats_lookup"`
	DefaultTimezoneLeaderboard bool                       `json:"default_timezone_leaderboard"`
	TrustLevel                 TrustLevel                 `json:"trust_level"`
	AdminLevel                 AdminLevel                 `json:"admin_level"`
	HackatimeExtensionTextType HackatimeExtensionTextType `json:"hackatime_extension_text_type"`
}

// CanConvictUsers mirrors User#can_convict_users?.
func (u *User) CanConvictUsers() bool {
	return u.AdminLevel == AdminLevelSuperadmin
}

// PublicTrustLevel masks yellow ("suspected") as blue, matching StatsController.
func (u *User) PublicTrustLevel() TrustLevel {
	if u.TrustLevel == TrustLevelYellow {
		return TrustLevelBlue
	}
	return u.TrustLevel
}

// StreakDaysFormatted mirrors User#streak_days_formatted.
func StreakDaysFormatted(days int) *string {
	if days > 30 {
		s := "30+"
		return &s
	}
	if days < 1 {
		return nil
	}
	s := fmt.Sprintf("%d", days)
	return &s
}

// FormatExtensionText mirrors User#format_extension_text.
func (u *User) FormatExtensionText(durationSec int) string {
	switch u.HackatimeExtensionTextType {
	case ExtensionSimpleText:
		if durationSec == 0 {
			return "Start coding to track your time"
		}
		hours := durationSec / 3600
		mins := (durationSec % 3600) / 60
		if hours > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dm", mins)
	case ExtensionClockEmoji:
		return "🕒"
	case ExtensionComplimentText:
		return "You're crushing it!"
	default:
		return ""
	}
}
