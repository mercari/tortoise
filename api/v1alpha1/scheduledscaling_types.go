package v1alpha1

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScheduledScalingSpec defines the desired state of ScheduledScaling
// +kubebuilder:object:generate=true
type ScheduledScalingSpec struct {
	// Schedule defines when the scaling should occur
	// +kubebuilder:validation:Required
	Schedule Schedule `json:"schedule"`

	// TargetRefs specifies which resources this scheduled scaling should affect
	// +kubebuilder:validation:Required
	TargetRefs TargetRefs `json:"targetRefs"`

	// Strategy defines how the scaling should be performed
	// +kubebuilder:validation:Required
	Strategy Strategy `json:"strategy"`

	// Status indicates the current state of the scheduled scaling
	// +kubebuilder:validation:Enum=Inactive;Active
	// +kubebuilder:default=Inactive
	Status ScheduledScalingState `json:"status,omitempty"`
}

// ScheduleType defines the type of scheduling to use
type ScheduleType string

const (
	// ScheduleTypeTime uses specific start and end times
	ScheduleTypeTime ScheduleType = "time"
	// ScheduleTypeCron uses cron expression for periodic scheduling
	ScheduleTypeCron ScheduleType = "cron"
)

// Schedule defines the timing for scheduled scaling
type Schedule struct {
	// Type specifies the scheduling type: "time" or "cron"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=time;cron
	Type ScheduleType `json:"type"`

	// Time-based scheduling fields (used when type="time")
	// StartAt specifies when the scaling should begin
	// Format: RFC3339 (e.g., "2024-01-15T10:00:00Z")
	// +kubebuilder:validation:Optional
	StartAt string `json:"startAt,omitempty"`

	// FinishAt specifies when the scaling should end and return to normal
	// Format: RFC3339 (e.g., "2024-01-15T18:00:00Z")
	// +kubebuilder:validation:Optional
	FinishAt string `json:"finishAt,omitempty"`

	// Cron-based scheduling fields (used when type="cron")
	// CronExpression defines when scaling periods should start using cron format
	// Format: "minute hour day month dayofweek" (e.g., "0 9 * * 1-5" for 9 AM weekdays)
	// +kubebuilder:validation:Optional
	CronExpression string `json:"cronExpression,omitempty"`

	// Duration specifies how long each scaling period should last
	// Format: Go duration (e.g., "8h", "30m", "1h30m")
	// +kubebuilder:validation:Optional
	Duration string `json:"duration,omitempty"`

	// TimeZone specifies the timezone for cron-based scheduling
	// Format: IANA timezone (e.g., "Asia/Tokyo", "UTC", "America/New_York")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="Asia/Tokyo"
	TimeZone string `json:"timeZone,omitempty"`
}

// TargetRefs specifies which resources to scale
type TargetRefs struct {
	// TortoiseName is the name of the Tortoise resource to scale
	// +kubebuilder:validation:Required
	TortoiseName string `json:"tortoiseName"`
}

// Strategy defines how the scaling should be performed
// +kubebuilder:object:generate=true
type Strategy struct {
	// Static defines static scaling parameters
	// +kubebuilder:validation:Required
	Static StaticStrategy `json:"static"`
}

// StaticStrategy defines static scaling parameters
// +kubebuilder:object:generate=true
type StaticStrategy struct {
	// MinimumMinReplicas sets the minimum number of replicas during scaling
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Optional
	MinimumMinReplicas *int32 `json:"minimumMinReplicas,omitempty"`

	// MinAllocatedResources sets the minimum allocated resources during scaling
	// This can be either global (applied to all containers) or container-specific
	// +kubebuilder:validation:Optional
	MinAllocatedResources *ResourceRequirements `json:"minAllocatedResources,omitempty"`

	// ContainerMinAllocatedResources sets container-specific minimum allocated resources
	// If specified, this takes precedence over MinAllocatedResources for specific containers
	// +kubebuilder:validation:Optional
	ContainerMinAllocatedResources []ContainerResourceRequirements `json:"containerMinAllocatedResources,omitempty"`
}

// ResourceRequirements describes the compute resource requirements
// +kubebuilder:object:generate=true
type ResourceRequirements struct {
	// CPU specifies the CPU resource requirements
	// +kubebuilder:validation:Required
	CPU string `json:"cpu"`

	// Memory specifies the memory resource requirements
	// +kubebuilder:validation:Required
	Memory string `json:"memory"`
}

// ContainerResourceRequirements describes container-specific resource requirements
// +kubebuilder:object:generate=true
type ContainerResourceRequirements struct {
	// ContainerName specifies which container these resources apply to
	// +kubebuilder:validation:Required
	ContainerName string `json:"containerName"`

	// Resources specifies the resource requirements for this container
	// +kubebuilder:validation:Required
	Resources ResourceRequirements `json:"resources"`
}

// ScheduledScalingState represents the desired state of a scheduled scaling operation
type ScheduledScalingState string

const (
	// ScheduledScalingStateInactive means the scheduled scaling is not active
	ScheduledScalingStateInactive ScheduledScalingState = "Inactive"
	// ScheduledScalingStateActive means the scheduled scaling is currently active
	ScheduledScalingStateActive ScheduledScalingState = "Active"
)

// ScheduledScalingStatus defines the observed state of ScheduledScaling
// +kubebuilder:object:generate=true
type ScheduledScalingStatus struct {
	// Phase indicates the current phase of the scheduled scaling
	// +kubebuilder:validation:Enum=Pending;Active;Completed;Failed
	Phase ScheduledScalingPhase `json:"phase,omitempty"`

	// LastTransitionTime is the last time the status transitioned from one phase to another
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Message provides additional information about the current phase
	Message string `json:"message,omitempty"`

	// Reason indicates why the scheduled scaling is in the current phase
	Reason string `json:"reason,omitempty"`

	// CurrentWindowStart indicates the start of the current scaling window (for cron-based scheduling)
	CurrentWindowStart *metav1.Time `json:"currentWindowStart,omitempty"`

	// CurrentWindowEnd indicates the end of the current scaling window (for cron-based scheduling)
	CurrentWindowEnd *metav1.Time `json:"currentWindowEnd,omitempty"`

	// NextWindowStart indicates when the next scaling window will start (for cron-based scheduling)
	NextWindowStart *metav1.Time `json:"nextWindowStart,omitempty"`

	// HumanReadableSchedule provides a human-readable description of the schedule
	HumanReadableSchedule string `json:"humanReadableSchedule,omitempty"`

	// FormattedStartTime provides a human-readable formatted start time
	FormattedStartTime string `json:"formattedStartTime,omitempty"`

	// FormattedEndTime provides a human-readable formatted end time
	FormattedEndTime string `json:"formattedEndTime,omitempty"`

	// FormattedNextStartTime provides a human-readable formatted next start time
	FormattedNextStartTime string `json:"formattedNextStartTime,omitempty"`
}

// ScheduledScalingPhase represents the phase of a scheduled scaling operation
type ScheduledScalingPhase string

const (
	// ScheduledScalingPhasePending means the scheduled scaling is waiting to start
	ScheduledScalingPhasePending ScheduledScalingPhase = "Pending"
	// ScheduledScalingPhaseActive means the scheduled scaling is currently active
	ScheduledScalingPhaseActive ScheduledScalingPhase = "Active"
	// ScheduledScalingPhaseCompleted means the scheduled scaling has completed successfully
	ScheduledScalingPhaseCompleted ScheduledScalingPhase = "Completed"
	// ScheduledScalingPhaseFailed means the scheduled scaling has failed
	ScheduledScalingPhaseFailed ScheduledScalingPhase = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.schedule.type"
//+kubebuilder:printcolumn:name="Start Time",type="string",JSONPath=".status.formattedStartTime"
//+kubebuilder:printcolumn:name="End Time",type="string",JSONPath=".status.formattedEndTime"
//+kubebuilder:printcolumn:name="Next Start",type="string",JSONPath=".status.formattedNextStartTime"
//+kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".status.humanReadableSchedule"
//+kubebuilder:printcolumn:name="Target Tortoise",type="string",JSONPath=".spec.targetRefs.tortoiseName"
//+kubebuilder:printcolumn:name="Warnings",type="string",JSONPath=".status.message"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScheduledScaling is the Schema for the scheduledscalings API
// +kubebuilder:object:generate=true
type ScheduledScaling struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledScalingSpec   `json:"spec,omitempty"`
	Status ScheduledScalingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScheduledScalingList contains a list of ScheduledScaling
type ScheduledScalingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledScaling `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledScaling{}, &ScheduledScalingList{})
}

// GetHumanReadableSchedule returns a human-readable description of the schedule
func (s *ScheduledScaling) GetHumanReadableSchedule() string {
	if s.Spec.Schedule.Type == ScheduleTypeCron {
		return s.getHumanReadableCronSchedule()
	}
	return s.getHumanReadableTimeSchedule()
}

// getHumanReadableCronSchedule returns a human-readable description of cron schedule
func (s *ScheduledScaling) getHumanReadableCronSchedule() string {
	if s.Spec.Schedule.CronExpression == "" {
		return "Invalid cron schedule"
	}

	// Parse cron expression and provide human-readable description
	desc := s.parseCronExpression(s.Spec.Schedule.CronExpression)

	// Add duration information
	if s.Spec.Schedule.Duration != "" {
		duration, err := time.ParseDuration(s.Spec.Schedule.Duration)
		if err == nil {
			desc += s.formatDuration(duration)
		} else {
			desc += " for " + s.Spec.Schedule.Duration
		}
	}

	// Add timezone information if different from default
	if s.Spec.Schedule.TimeZone != "" && s.Spec.Schedule.TimeZone != "Asia/Tokyo" {
		desc += " (" + s.Spec.Schedule.TimeZone + ")"
	}

	return desc
}

// formatDuration formats duration in a human-readable way
func (s *ScheduledScaling) formatDuration(duration time.Duration) string {
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		return fmt.Sprintf(" for %d minute%s", minutes, pluralSuffix(minutes))
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf(" for %d hour%s", hours, pluralSuffix(hours))
		} else {
			return fmt.Sprintf(" for %dh %dm", hours, minutes)
		}
	} else {
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		if hours == 0 {
			return fmt.Sprintf(" for %d day%s", days, pluralSuffix(days))
		} else {
			return fmt.Sprintf(" for %d day%s %d hour%s", days, pluralSuffix(days), hours, pluralSuffix(hours))
		}
	}
}

// getHumanReadableTimeSchedule returns a human-readable description of time-based schedule
func (s *ScheduledScaling) getHumanReadableTimeSchedule() string {
	if s.Spec.Schedule.StartAt == "" || s.Spec.Schedule.FinishAt == "" {
		return "Invalid time schedule"
	}

	startTime, err := time.Parse(time.RFC3339, s.Spec.Schedule.StartAt)
	if err != nil {
		return "Invalid start time format"
	}

	finishTime, err := time.Parse(time.RFC3339, s.Spec.Schedule.FinishAt)
	if err != nil {
		return "Invalid finish time format"
	}

	// Calculate duration
	duration := finishTime.Sub(startTime)

	// Create a descriptive schedule name
	scheduleName := s.generateScheduleName(startTime, finishTime, duration)

	// Build the description - shortened version
	if duration < 24*time.Hour {
		// Less than 24 hours
		if duration < time.Hour {
			minutes := int(duration.Minutes())
			return fmt.Sprintf("%s (%d min) - One-time", scheduleName, minutes)
		} else {
			hours := int(duration.Hours())
			minutes := int(duration.Minutes()) % 60
			if minutes == 0 {
				return fmt.Sprintf("%s (%d hr) - One-time", scheduleName, hours)
			} else {
				return fmt.Sprintf("%s (%dh %dm) - One-time", scheduleName, hours, minutes)
			}
		}
	} else {
		// 24 hours or more
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		if hours == 0 {
			return fmt.Sprintf("%s (%d day) - One-time", scheduleName, days)
		} else {
			return fmt.Sprintf("%s (%d day %dh) - One-time", scheduleName, days, hours)
		}
	}
}

// generateScheduleName creates a descriptive name for the schedule
func (s *ScheduledScaling) generateScheduleName(startTime, finishTime time.Time, duration time.Duration) string {
	// Check for common patterns
	startHour := startTime.Hour()
	finishHour := finishTime.Hour()

	// Business hours pattern
	if startHour >= 8 && startHour <= 10 && finishHour >= 17 && finishHour <= 19 {
		if duration >= 8*time.Hour && duration <= 10*time.Hour {
			return "Business Hours"
		}
	}

	// Night shift pattern
	if (startHour >= 22 || startHour <= 6) && (finishHour >= 22 || finishHour <= 6) {
		return "Night Shift"
	}

	// Weekend pattern
	if startTime.Weekday() == time.Saturday || startTime.Weekday() == time.Sunday ||
		finishTime.Weekday() == time.Saturday || finishTime.Weekday() == time.Sunday {
		return "Weekend"
	}

	// Campaign/Event pattern (longer duration)
	if duration >= 24*time.Hour {
		return "Campaign"
	}

	// Peak hours pattern
	if (startHour >= 7 && startHour <= 9) || (startHour >= 17 && startHour <= 19) {
		return "Peak Hours"
	}

	// Default based on duration
	if duration < time.Hour {
		return "Quick Scaling"
	} else if duration < 24*time.Hour {
		return "Daily Scaling"
	} else {
		return "Extended Scaling"
	}
}

// parseCronExpression converts cron expression to human-readable text
func (s *ScheduledScaling) parseCronExpression(cronExpr string) string {
	// Enhanced mapping for common cron patterns
	patterns := map[string]string{
		"0 9 * * 1-5":  "Weekdays at 9:00 AM",
		"0 8,20 * * *": "Daily at 8:00 AM and 8:00 PM",
		"0 0 1 * *":    "Monthly on 1st at midnight",
		"0 2 * * 6":    "Weekly on Saturday at 2:00 AM",
		"0 8 * * 1-5":  "Weekdays at 8:00 AM",
		"0 10 * * 1-5": "Weekdays at 10:00 AM",
		"0 17 * * 1-5": "Weekdays at 5:00 PM",
		"0 18 * * 1-5": "Weekdays at 6:00 PM",
		"*/5 * * * *":  "Every 5 minutes",
		"*/10 * * * *": "Every 10 minutes",
		"*/15 * * * *": "Every 15 minutes",
		"*/30 * * * *": "Every 30 minutes",
		"0 */2 * * *":  "Every 2 hours",
		"0 */3 * * *":  "Every 3 hours",
		"0 */4 * * *":  "Every 4 hours",
		"0 */6 * * *":  "Every 6 hours",
		"0 */8 * * *":  "Every 8 hours",
		"0 */12 * * *": "Every 12 hours",
		"0 0 * * *":    "Daily at midnight",
		"0 6 * * *":    "Daily at 6:00 AM",
		"0 12 * * *":   "Daily at noon",
		"0 18 * * *":   "Daily at 6:00 PM",
		"0 0 * * 1":    "Weekly on Monday at midnight",
		"0 0 * * 6":    "Weekly on Saturday at midnight",
		"0 0 * * 0":    "Weekly on Sunday at midnight",
		"0 0 15 * *":   "Monthly on 15th at midnight",
		"0 0 1 1 *":    "Yearly on January 1st at midnight",
	}

	if desc, exists := patterns[cronExpr]; exists {
		return desc
	}

	// For unknown patterns, provide intelligent parsing
	parts := strings.Fields(cronExpr)
	if len(parts) >= 5 {
		minute := parts[0]
		hour := parts[1]
		day := parts[2]
		month := parts[3]
		weekday := parts[4]

		desc := s.buildCronDescription(minute, hour, day, month, weekday)
		return desc
	}

	return "Cron: " + cronExpr
}

// buildCronDescription builds a human-readable description from cron parts
func (s *ScheduledScaling) buildCronDescription(minute, hour, day, month, weekday string) string {
	var desc strings.Builder

	// Handle minutes
	if minute != "*" && minute != "0" {
		if minute == "*/5" {
			desc.WriteString("Every 5 minutes ")
		} else if minute == "*/10" {
			desc.WriteString("Every 10 minutes ")
		} else if minute == "*/15" {
			desc.WriteString("Every 15 minutes ")
		} else if minute == "*/30" {
			desc.WriteString("Every 30 minutes ")
		} else {
			desc.WriteString(fmt.Sprintf("At %s minutes past ", minute))
		}
	}

	// Handle hours
	if hour != "*" {
		if strings.Contains(hour, ",") {
			hours := strings.Split(hour, ",")
			if len(hours) == 2 {
				desc.WriteString(fmt.Sprintf("at %s:00 and %s:00 ", hours[0], hours[1]))
			} else {
				desc.WriteString(fmt.Sprintf("at %s ", hour))
			}
		} else if strings.Contains(hour, "-") {
			hourRange := strings.Split(hour, "-")
			if len(hourRange) == 2 {
				desc.WriteString(fmt.Sprintf("between %s:00 and %s:00 ", hourRange[0], hourRange[1]))
			} else {
				desc.WriteString(fmt.Sprintf("at %s:00 ", hour))
			}
		} else if strings.Contains(hour, "/") {
			hourStep := strings.Split(hour, "/")
			if len(hourStep) == 2 {
				desc.WriteString(fmt.Sprintf("every %s hours starting at %s:00 ", hourStep[1], hourStep[0]))
			} else {
				desc.WriteString(fmt.Sprintf("at %s:00 ", hour))
			}
		} else {
			desc.WriteString(fmt.Sprintf("at %s:00 ", hour))
		}
	}

	// Handle days
	if day != "*" {
		if day == "1" {
			desc.WriteString("on the 1st ")
		} else if day == "15" {
			desc.WriteString("on the 15th ")
		} else if day == "31" {
			desc.WriteString("on the 31st ")
		} else {
			desc.WriteString(fmt.Sprintf("on day %s ", day))
		}
	}

	// Handle months
	if month != "*" {
		monthNames := []string{"", "January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"}
		if monthNum, err := strconv.Atoi(month); err == nil && monthNum >= 1 && monthNum <= 12 {
			desc.WriteString(fmt.Sprintf("in %s ", monthNames[monthNum]))
		} else {
			desc.WriteString(fmt.Sprintf("in month %s ", month))
		}
	}

	// Handle weekdays
	if weekday != "*" {
		if weekday == "1-5" {
			desc.WriteString("on weekdays")
		} else if weekday == "6-7" || weekday == "0,6" {
			desc.WriteString("on weekends")
		} else if weekday == "1" {
			desc.WriteString("on Mondays")
		} else if weekday == "2" {
			desc.WriteString("on Tuesdays")
		} else if weekday == "3" {
			desc.WriteString("on Wednesdays")
		} else if weekday == "4" {
			desc.WriteString("on Thursdays")
		} else if weekday == "5" {
			desc.WriteString("on Fridays")
		} else if weekday == "6" {
			desc.WriteString("on Saturdays")
		} else if weekday == "0" || weekday == "7" {
			desc.WriteString("on Sundays")
		} else if strings.Contains(weekday, ",") {
			days := strings.Split(weekday, ",")
			if len(days) == 2 {
				desc.WriteString(fmt.Sprintf("on %s and %s", s.getWeekdayName(days[0]), s.getWeekdayName(days[1])))
			} else {
				desc.WriteString(fmt.Sprintf("on weekdays %s", weekday))
			}
		} else {
			desc.WriteString(fmt.Sprintf("on weekday %s", weekday))
		}
	}

	result := strings.TrimSpace(desc.String())
	if result == "" {
		return "Every minute"
	}

	return result
}

// getWeekdayName converts weekday number to name
func (s *ScheduledScaling) getWeekdayName(weekday string) string {
	switch weekday {
	case "0", "7":
		return "Sunday"
	case "1":
		return "Monday"
	case "2":
		return "Tuesday"
	case "3":
		return "Wednesday"
	case "4":
		return "Thursday"
	case "5":
		return "Friday"
	case "6":
		return "Saturday"
	default:
		return weekday
	}
}

// GetTimezone returns the timezone to use for formatting times
// Defaults to Asia/Tokyo if not specified
func (s *ScheduledScaling) GetTimezone() *time.Location {
	timezone := s.Spec.Schedule.TimeZone
	if timezone == "" {
		timezone = "Asia/Tokyo" // Default timezone
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Fallback to Asia/Tokyo if the specified timezone is invalid
		loc, _ = time.LoadLocation("Asia/Tokyo")
	}
	return loc
}

// GetHumanReadableTime formats a time in a human-readable way
func (s *ScheduledScaling) GetHumanReadableTime(t *metav1.Time) string {
	if t == nil {
		return "-"
	}

	loc := s.GetTimezone()
	now := time.Now().In(loc)
	timeDiff := t.Time.In(loc).Sub(now)

	// Format based on how far in the past/future the time is
	if timeDiff < 0 {
		// Past time
		absDiff := -timeDiff
		if absDiff < time.Minute {
			return "just now"
		} else if absDiff < time.Hour {
			minutes := int(absDiff.Minutes())
			return fmt.Sprintf("%d minute%s ago", minutes, pluralSuffix(minutes))
		} else if absDiff < 24*time.Hour {
			hours := int(absDiff.Hours())
			return fmt.Sprintf("%d hour%s ago", hours, pluralSuffix(hours))
		} else {
			days := int(absDiff.Hours() / 24)
			return fmt.Sprintf("%d day%s ago", days, pluralSuffix(days))
		}
	} else {
		// Future time
		if timeDiff < time.Minute {
			return "starting now"
		} else if timeDiff < time.Hour {
			minutes := int(timeDiff.Minutes())
			return fmt.Sprintf("in %d minute%s", minutes, pluralSuffix(minutes))
		} else if timeDiff < 24*time.Hour {
			hours := int(timeDiff.Hours())
			return fmt.Sprintf("in %d hour%s", hours, pluralSuffix(hours))
		} else {
			days := int(timeDiff.Hours() / 24)
			return fmt.Sprintf("in %d day%s", days, pluralSuffix(days))
		}
	}
}

// GetFormattedTime returns a human-readable formatted time with date information
func (s *ScheduledScaling) GetFormattedTime(t *metav1.Time) string {
	if t == nil {
		return "-"
	}

	loc := s.GetTimezone()
	now := time.Now().In(loc)
	targetTime := t.Time.In(loc)

	// If it's today, show "Today at time"
	if targetTime.Year() == now.Year() && targetTime.YearDay() == now.YearDay() {
		return fmt.Sprintf("Today at %s", targetTime.Format("3:04 PM"))
	}

	// If it's tomorrow, show "Tomorrow at time"
	if targetTime.Year() == now.Year() && targetTime.YearDay() == now.YearDay()+1 {
		return fmt.Sprintf("Tomorrow at %s", targetTime.Format("3:04 PM"))
	}

	// If it's within a week, show day and time
	if targetTime.Sub(now) < 7*24*time.Hour {
		return targetTime.Format("Mon 3:04 PM")
	}

	// Otherwise show date and time
	return targetTime.Format("Jan 2, 3:04 PM")
}

// pluralSuffix returns the appropriate plural suffix
func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
