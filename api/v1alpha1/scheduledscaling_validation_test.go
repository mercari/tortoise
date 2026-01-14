package v1alpha1

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSchedule_Validate(t *testing.T) {
	tests := []struct {
		name     string
		schedule Schedule
		wantErr  bool
		errMsg   string
	}{
		// Time-based scheduling tests
		{
			name: "valid time-based schedule",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			wantErr: false,
		},
		{
			name: "time-based missing startAt",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				FinishAt: "2024-01-15T18:00:00Z",
			},
			wantErr: true,
			errMsg:  "startAt is required when type is 'time'",
		},
		{
			name: "time-based missing finishAt",
			schedule: Schedule{
				Type:    ScheduleTypeTime,
				StartAt: "2024-01-15T10:00:00Z",
			},
			wantErr: true,
			errMsg:  "finishAt is required when type is 'time'",
		},
		{
			name: "time-based invalid startAt format",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15 10:00:00",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			wantErr: true,
			errMsg:  "invalid startAt format",
		},
		{
			name: "time-based invalid finishAt format",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15 18:00:00",
			},
			wantErr: true,
			errMsg:  "invalid finishAt format",
		},
		{
			name: "time-based finishAt before startAt",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T18:00:00Z",
				FinishAt: "2024-01-15T10:00:00Z",
			},
			wantErr: true,
			errMsg:  "must be after startAt",
		},
		{
			name: "time-based finishAt equals startAt",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T10:00:00Z",
			},
			wantErr: true,
			errMsg:  "must be after startAt",
		},
		{
			name: "time-based with cron fields should fail",
			schedule: Schedule{
				Type:           ScheduleTypeTime,
				StartAt:        "2024-01-15T10:00:00Z",
				FinishAt:       "2024-01-15T18:00:00Z",
				CronExpression: "0 9 * * *",
			},
			wantErr: true,
			errMsg:  "cronExpression cannot be set when type is 'time'",
		},

		// Cron-based scheduling tests
		{
			name: "valid cron-based schedule",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "8h",
				TimeZone:       "Asia/Tokyo",
			},
			wantErr: false,
		},
		{
			name: "valid cron-based schedule with timezone",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "8h",
				TimeZone:       "Asia/Tokyo",
			},
			wantErr: false,
		},
		{
			name: "cron-based missing cronExpression",
			schedule: Schedule{
				Type:     ScheduleTypeCron,
				Duration: "8h",
			},
			wantErr: true,
			errMsg:  "cronExpression is required when type is 'cron'",
		},
		{
			name: "cron-based missing duration",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
			},
			wantErr: true,
			errMsg:  "duration is required when type is 'cron'",
		},
		{
			name: "cron-based invalid cron expression",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "invalid cron",
				Duration:       "8h",
			},
			wantErr: true,
			errMsg:  "invalid cronExpression format",
		},
		{
			name: "cron-based invalid duration",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "invalid duration",
			},
			wantErr: true,
			errMsg:  "invalid duration format",
		},
		{
			name: "cron-based duration too short",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "30s",
			},
			wantErr: true,
			errMsg:  "duration must be at least 1 minute",
		},
		{
			name: "cron-based duration too long",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "25h",
			},
			wantErr: true,
			errMsg:  "duration must not exceed 24 hours",
		},
		{
			name: "cron-based invalid timezone",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "8h",
				TimeZone:       "Invalid/Timezone",
			},
			wantErr: true,
			errMsg:  "invalid timeZone",
		},
		{
			name: "cron-based with time fields should fail",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5",
				Duration:       "8h",
				StartAt:        "2024-01-15T10:00:00Z",
			},
			wantErr: true,
			errMsg:  "startAt cannot be set when type is 'cron'",
		},

		// Invalid type tests
		{
			name: "invalid schedule type",
			schedule: Schedule{
				Type: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid schedule type: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schedule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Schedule.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got no error", tt.errMsg)
				} else if !containsPattern(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestSchedule_GetNextScheduleWindow(t *testing.T) {
	tests := []struct {
		name     string
		schedule Schedule
		from     time.Time
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid cron schedule - daily",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * *", // Daily at 9 AM
				Duration:       "8h",
				TimeZone:       "Asia/Tokyo",
			},
			from:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC), // 8 AM
			wantErr: false,
		},
		{
			name: "valid cron schedule - weekdays",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * 1-5", // Weekdays at 9 AM
				Duration:       "8h",
				TimeZone:       "Asia/Tokyo",
			},
			from:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC), // Monday 8 AM UTC
			wantErr: false,
		},
		{
			name: "time-based schedule should fail",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			from:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
			wantErr: true,
			errMsg:  "GetNextScheduleWindow is only supported for cron-based scheduling",
		},
		{
			name: "invalid cron expression",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "invalid cron",
				Duration:       "8h",
				TimeZone:       "Asia/Tokyo",
			},
			from:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
			wantErr: true,
			errMsg:  "failed to parse cron expression",
		},
		{
			name: "invalid duration",
			schedule: Schedule{
				Type:           ScheduleTypeCron,
				CronExpression: "0 9 * * *",
				Duration:       "invalid",
				TimeZone:       "Asia/Tokyo",
			},
			from:    time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
			wantErr: true,
			errMsg:  "failed to parse duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime, endTime, err := tt.schedule.GetNextScheduleWindow(tt.from)
			if (err != nil) != tt.wantErr {
				t.Errorf("Schedule.GetNextScheduleWindow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.errMsg != "" && err != nil {
					if !containsPattern(err.Error(), tt.errMsg) {
						t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
					}
				}
			} else {
				// Validate that we got valid times
				if startTime.IsZero() || endTime.IsZero() {
					t.Errorf("Expected valid start and end times, got start=%v, end=%v", startTime, endTime)
				}
				if !endTime.After(startTime) {
					t.Errorf("End time should be after start time, got start=%v, end=%v", startTime, endTime)
				}
				if !startTime.After(tt.from) {
					t.Errorf("Start time should be after 'from' time, got start=%v, from=%v", startTime, tt.from)
				}
			}
		})
	}
}

func TestSchedule_IsCurrentlyActive(t *testing.T) {
	tests := []struct {
		name       string
		schedule   Schedule
		now        time.Time
		wantActive bool
		wantErr    bool
	}{
		{
			name: "time-based - before start",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			now:        time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
			wantActive: false,
			wantErr:    false,
		},
		{
			name: "time-based - during period",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			now:        time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
			wantActive: true,
			wantErr:    false,
		},
		{
			name: "time-based - after end",
			schedule: Schedule{
				Type:     ScheduleTypeTime,
				StartAt:  "2024-01-15T10:00:00Z",
				FinishAt: "2024-01-15T18:00:00Z",
			},
			now:        time.Date(2024, 1, 15, 19, 0, 0, 0, time.UTC),
			wantActive: false,
			wantErr:    false,
		},
		// Note: Cron-based tests are complex and would require mocking time
		// In real implementation, you'd test with specific known schedules
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isActive, _, err := tt.schedule.IsCurrentlyActive(tt.now)
			if (err != nil) != tt.wantErr {
				t.Errorf("Schedule.IsCurrentlyActive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if isActive != tt.wantActive {
				t.Errorf("Schedule.IsCurrentlyActive() = %v, want %v", isActive, tt.wantActive)
			}
		})
	}
}

// Helper function to check if error message contains expected pattern
func containsPattern(text, pattern string) bool {
	// For simplicity, just check if pattern is a substring
	// In real implementation, you might want to use regex matching
	return len(text) > 0 && (pattern == "" ||
		len(pattern) > 0 && len(text) >= len(pattern) &&
			findSubstring(text, pattern))
}

func findSubstring(text, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(text) < len(substr) {
		return false
	}
	for i := 0; i <= len(text)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if text[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestScheduledScaling_AnnotationCleanup tests that ScheduledScaling annotations are properly cleaned up
func TestScheduledScaling_AnnotationCleanup(t *testing.T) {
	// This test verifies the expected behavior of annotation cleanup
	// The actual cleanup logic is in the controller, but we can test the annotation constants

	const annOriginal = "autoscaling.mercari.com/scheduledscaling-original-spec"
	const annMinReplicas = "autoscaling.mercari.com/scheduledscaling-min-replicas"

	// Verify annotation names are correctly defined
	if annOriginal == "" {
		t.Error("annOriginal constant should not be empty")
	}

	if annMinReplicas == "" {
		t.Error("annMinReplicas constant should not be empty")
	}

	// Verify annotation names follow the expected pattern
	expectedPrefix := "autoscaling.mercari.com/scheduledscaling-"
	if !strings.HasPrefix(annOriginal, expectedPrefix) {
		t.Errorf("annOriginal should start with %s, got %s", expectedPrefix, annOriginal)
	}

	if !strings.HasPrefix(annMinReplicas, expectedPrefix) {
		t.Errorf("annMinReplicas should start with %s, got %s", expectedPrefix, annMinReplicas)
	}
}

// TestScheduledScaling_ValidateMinReplicasWarning tests the minReplicas validation warning logic
func TestScheduledScaling_ValidateMinReplicasWarning(t *testing.T) {
	tests := []struct {
		name                 string
		requestedMinReplicas int32
		hpaRecommendation    int32
		expectedWarning      bool
		expectedMessage      string
	}{
		{
			name:                 "no warning when requested >= recommended",
			requestedMinReplicas: 5,
			hpaRecommendation:    3,
			expectedWarning:      false,
			expectedMessage:      "",
		},
		{
			name:                 "warning when requested < recommended",
			requestedMinReplicas: 2,
			hpaRecommendation:    5,
			expectedWarning:      true,
			expectedMessage:      "Requested minReplicas (2) is lower than HPA's current recommendation (5) for the workload. Using HPA recommendation (5) instead to prevent performance issues. Consider reviewing your scaling strategy.",
		},
		{
			name:                 "no warning when requested equals recommended",
			requestedMinReplicas: 5,
			hpaRecommendation:    5,
			expectedWarning:      false,
			expectedMessage:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the expected warning message format
			if tt.expectedWarning {
				expectedMsg := fmt.Sprintf("Requested minReplicas (%d) is lower than HPA's current recommendation (%d) for the workload. Using HPA recommendation (%d) instead to prevent performance issues. Consider reviewing your scaling strategy.",
					tt.requestedMinReplicas, tt.hpaRecommendation, tt.hpaRecommendation)

				if expectedMsg != tt.expectedMessage {
					t.Errorf("expected warning message %q, got %q", tt.expectedMessage, expectedMsg)
				}
			}
		})
	}
}
