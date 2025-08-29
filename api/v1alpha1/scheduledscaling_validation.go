package v1alpha1

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// ValidateSchedule validates the schedule configuration based on the type
func (s *Schedule) Validate() error {
	switch s.Type {
	case ScheduleTypeTime:
		return s.validateTimeBasedSchedule()
	case ScheduleTypeCron:
		return s.validateCronBasedSchedule()
	default:
		return fmt.Errorf("invalid schedule type: %s, must be 'time' or 'cron'", s.Type)
	}
}

// validateTimeBasedSchedule validates time-based scheduling parameters
func (s *Schedule) validateTimeBasedSchedule() error {
	// Validate required fields for time-based scheduling
	if s.StartAt == "" {
		return fmt.Errorf("startAt is required when type is 'time'")
	}
	if s.FinishAt == "" {
		return fmt.Errorf("finishAt is required when type is 'time'")
	}

	// Validate time format (RFC3339)
	startTime, err := time.Parse(time.RFC3339, s.StartAt)
	if err != nil {
		return fmt.Errorf("invalid startAt format: %v, expected RFC3339 format (e.g., '2024-01-15T10:00:00Z')", err)
	}

	finishTime, err := time.Parse(time.RFC3339, s.FinishAt)
	if err != nil {
		return fmt.Errorf("invalid finishAt format: %v, expected RFC3339 format (e.g., '2024-01-15T18:00:00Z')", err)
	}

	// Validate that finish time is after start time
	if finishTime.Before(startTime) || finishTime.Equal(startTime) {
		return fmt.Errorf("finishAt (%s) must be after startAt (%s)", s.FinishAt, s.StartAt)
	}

	// Validate that cron-specific fields are not set
	if s.CronExpression != "" {
		return fmt.Errorf("cronExpression cannot be set when type is 'time'")
	}
	if s.Duration != "" {
		return fmt.Errorf("duration cannot be set when type is 'time'")
	}

	return nil
}

// validateCronBasedSchedule validates cron-based scheduling parameters
func (s *Schedule) validateCronBasedSchedule() error {
	// Validate required fields for cron-based scheduling
	if s.CronExpression == "" {
		return fmt.Errorf("cronExpression is required when type is 'cron'")
	}
	if s.Duration == "" {
		return fmt.Errorf("duration is required when type is 'cron'")
	}

	// Validate cron expression format
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(s.CronExpression)
	if err != nil {
		return fmt.Errorf("invalid cronExpression format: %v, expected format 'minute hour day month dayofweek' (e.g., '0 9 * * 1-5')", err)
	}

	// Validate duration format
	duration, err := time.ParseDuration(s.Duration)
	if err != nil {
		return fmt.Errorf("invalid duration format: %v, expected Go duration format (e.g., '8h', '30m', '1h30m')", err)
	}

	// Validate duration is reasonable (not too short or too long)
	if duration < time.Minute {
		return fmt.Errorf("duration must be at least 1 minute")
	}
	if duration > 24*time.Hour {
		return fmt.Errorf("duration must not exceed 24 hours")
	}

	// Validate timezone format if provided
	if s.TimeZone != "" && s.TimeZone != "Asia/Tokyo" {
		_, err := time.LoadLocation(s.TimeZone)
		if err != nil {
			return fmt.Errorf("invalid timeZone: %v, expected IANA timezone format (e.g., 'Asia/Tokyo', 'America/New_York')", err)
		}
	}

	// Validate that time-specific fields are not set
	if s.StartAt != "" {
		return fmt.Errorf("startAt cannot be set when type is 'cron'")
	}
	if s.FinishAt != "" {
		return fmt.Errorf("finishAt cannot be set when type is 'cron'")
	}

	return nil
}

// GetNextScheduleWindow returns the next scheduled scaling window for cron-based scheduling
func (s *Schedule) GetNextScheduleWindow(from time.Time) (startTime, endTime time.Time, err error) {
	if s.Type != ScheduleTypeCron {
		return time.Time{}, time.Time{}, fmt.Errorf("GetNextScheduleWindow is only supported for cron-based scheduling")
	}

	// Parse timezone
	location, _ := time.LoadLocation("Asia/Tokyo") // Default to Tokyo timezone
	if s.TimeZone != "" && s.TimeZone != "Asia/Tokyo" {
		location, err = time.LoadLocation(s.TimeZone)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("failed to parse timezone: %v", err)
		}
	}

	// Parse duration
	duration, err := time.ParseDuration(s.Duration)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse duration: %v", err)
	}

	// Parse cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(s.CronExpression)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse cron expression: %v", err)
	}

	// Convert from time to the specified timezone
	fromInLocation := from.In(location)

	// Get next scheduled time
	nextTime := schedule.Next(fromInLocation)
	endTimeInLocation := nextTime.Add(duration)

	// Convert back to UTC for consistency
	return nextTime.UTC(), endTimeInLocation.UTC(), nil
}

// IsCurrentlyActive checks if the current time falls within an active scaling window
func (s *Schedule) IsCurrentlyActive(now time.Time) (bool, time.Time, error) {
	switch s.Type {
	case ScheduleTypeTime:
		startTime, err := time.Parse(time.RFC3339, s.StartAt)
		if err != nil {
			return false, time.Time{}, err
		}
		finishTime, err := time.Parse(time.RFC3339, s.FinishAt)
		if err != nil {
			return false, time.Time{}, err
		}

		isActive := now.After(startTime) && now.Before(finishTime)
		return isActive, finishTime, nil

	case ScheduleTypeCron:
		// For cron-based scheduling, we need to check if we're within the current window
		// We'll look back to find the most recent start time
		lookBackTime := now.Add(-25 * time.Hour) // Look back 25 hours to catch daily schedules

		// Get the most recent schedule window that could be active
		startTime, endTime, err := s.GetNextScheduleWindow(lookBackTime)
		if err != nil {
			return false, time.Time{}, err
		}

		// Check if we're currently in this window
		for startTime.Before(now) {
			if now.Before(endTime) {
				// We're in an active window
				return true, endTime, nil
			}

			// Move to next window
			startTime, endTime, err = s.GetNextScheduleWindow(startTime.Add(time.Minute))
			if err != nil {
				return false, time.Time{}, err
			}

			// Prevent infinite loop - if start time is too far in the future, break
			if startTime.After(now.Add(time.Hour)) {
				break
			}
		}

		return false, time.Time{}, nil

	default:
		return false, time.Time{}, fmt.Errorf("unsupported schedule type: %s", s.Type)
	}
}
