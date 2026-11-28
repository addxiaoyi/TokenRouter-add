package identity

import (
	"context"
	"fmt"
	"time"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// experience.go combines completed daily check-ins and accumulated invitation
// counts into account growth. Every completed date and successful invitation
// contributes one experience point. Interrupted check-ins preserve earned
// experience, while the current streak remains a separate calendar observation.
// Level one requires seven points, and every subsequent level adds another
// seven points to the cost of advancing.
//
// Account storage supplies the same source totals used by free-group permission
// reads. The service calculates presentation progress from those totals, and the
// shared first-level threshold keeps the experience page and authorization in
// agreement. Calendar reads use the application's configured local timezone,
// matching the site timezone supplied to the check-in plugin.

const (
	// ExperienceLevelStep sets the first-level requirement and the additional cost
	// of each later level, keeping free-group authorization on the same threshold.
	// @project-doc docs/domains/identity_and_tenancy.md#free_group_experience
	ExperienceLevelStep int64 = 7
)

var (
	// ErrExperienceUnavailable reports that the account store cannot supply the
	// persisted observations required to calculate the current account's experience.
	ErrExperienceUnavailable = infraerrors.ServiceUnavailable("EXPERIENCE_UNAVAILABLE", "Experience is unavailable for this account store")
)

// ExperienceTotals contains the stored observations needed to calculate account growth.
type ExperienceTotals struct {
	CheckinExp     int64 // completed unique dates, including expired rows
	InviteExp      int64 // accumulated successful invitation count
	Streak         int64 // consecutive active check-in days
	CheckedInToday bool  // active completed site-date check-in
}

// ExperienceReader supplies persisted growth observations for one current account.
type ExperienceReader interface {
	// GetExperienceTotals counts all stored completed dates and accumulated
	// invitations; streak and today fields use only active dates. Today is an ISO
	// calendar date in the site timezone, and unsupported stores return service
	// unavailability.
	GetExperienceTotals(ctx context.Context, userID int64, today string) (*ExperienceTotals, error)
}

// ExperienceStatus describes cumulative growth and progress toward the next level.
type ExperienceStatus struct {
	Level            int64 `json:"level"`              // highest completed account level
	Exp              int64 `json:"exp"`                // total earned experience points
	CheckinExp       int64 `json:"checkin_exp"`        // completed unique check-in dates
	InviteExp        int64 `json:"invite_exp"`         // accumulated successful invitation count
	Streak           int64 `json:"streak"`             // current consecutive check-in days
	Total            int64 `json:"total"`              // total completed check-in days
	LevelExp         int64 `json:"level_exp"`          // points earned within current level
	LevelRequiredExp int64 `json:"level_required_exp"` // points required for this level's advancement
	NextLevelExp     int64 `json:"next_level_exp"`     // cumulative points required for next level
	ExpToNext        int64 `json:"exp_to_next"`        // remaining points until next level
	FreeUnlocked     bool  `json:"free_unlocked"`      // earned first-level free-group eligibility
	CheckedInToday   bool  `json:"checked_in_today"`   // completed current site-date check-in
}

// GetExperience calculates account levels from completed check-ins and recorded
// invitation counts. Each source contributes one point, and interrupted streaks
// retain every completed date's point. Successive level costs increase by the
// shared first-level requirement, so cumulative thresholds follow its triangular
// sequence.
//
// The account repository must implement ExperienceReader; unsupported stores
// report service unavailability. Today comes from the operation clock in the
// configured site timezone so calendar progress agrees with the check-in plugin
// even when UTC falls on a different date.
func (s *UserService) GetExperience(ctx context.Context, userID int64) (*ExperienceStatus, error) {
	if s == nil {
		return nil, ErrExperienceUnavailable
	}
	reader, ok := s.userRepo.(ExperienceReader)
	if !ok {
		return nil, ErrExperienceUnavailable
	}
	today := s.now().In(time.Local).Format(time.DateOnly)
	totals, err := reader.GetExperienceTotals(ctx, userID, today)
	if err != nil {
		return nil, fmt.Errorf("get account experience: %w", err)
	}
	points := totals.CheckinExp + totals.InviteExp
	level := int64(0)
	levelPoints := points
	requiredPoints := ExperienceLevelStep
	// Subtract complete level costs so exact thresholds begin the following
	// level at zero progress while retaining all previously earned experience.
	for levelPoints >= requiredPoints {
		levelPoints -= requiredPoints
		level++
		requiredPoints += ExperienceLevelStep
	}
	return &ExperienceStatus{
		Level:            level,
		Exp:              points,
		CheckinExp:       totals.CheckinExp,
		InviteExp:        totals.InviteExp,
		Streak:           totals.Streak,
		Total:            totals.CheckinExp,
		LevelExp:         levelPoints,
		LevelRequiredExp: requiredPoints,
		NextLevelExp:     points - levelPoints + requiredPoints,
		ExpToNext:        requiredPoints - levelPoints,
		FreeUnlocked:     points >= ExperienceLevelStep,
		CheckedInToday:   totals.CheckedInToday,
	}, nil
}
