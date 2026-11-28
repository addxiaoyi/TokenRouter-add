import { apiClient } from './client'

// experience.ts reads the signed-in user's cumulative experience and level progress.
// The main application combines completed check-ins with successful invitations and
// supplies level thresholds and Free-group eligibility, so pages display the same
// decisions as request authorization.

// ExperienceStatus describes the server-calculated progress of the current user.
export interface ExperienceStatus {
  level: number // Current earned membership level.
  exp: number // Total cumulative experience points.
  checkin_exp: number // Experience from completed check-ins.
  invite_exp: number // Experience from successful invitations.
  streak: number // Current consecutive check-in days.
  total: number // Total completed check-in days.
  level_exp: number // Experience earned within this level.
  level_required_exp: number // Experience required for next upgrade.
  next_level_exp: number // Cumulative experience for next level.
  exp_to_next: number // Experience remaining until next level.
  free_unlocked: boolean // Whether earned XP meets the Free group level requirement.
  checked_in_today: boolean // Today's completed check-in status.
}

// readExperience retrieves the current user's combined check-in and invitation progress.
// The caller may cancel with its page lifetime; HTTP errors remain available for
// an explicit refresh warning.
export async function readExperience(signal?: AbortSignal): Promise<ExperienceStatus> {
  const { data } = await apiClient.get<ExperienceStatus>('/user/experience', { signal })
  return data
}
