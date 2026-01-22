package event

const (
	VPACreated = "VPACreated"

	VerticalRecommendationUpdated = "VerticalRecommendationUpdated"

	HPACreated  = "HPACreated"
	HPAUpdated  = "HPAUpdated"
	HPADeleted  = "HPADeleted"
	HPADisabled = "HPADisabled"

	RecommendationUpdated = "RecommendationUpdated"

	Initialized          = "Initialized"
	Working              = "Working"
	PartlyWorking        = "PartlyWorking"
	EmergencyModeEnabled = "EmergencyModeEnabled"
	EmergencyModeFailed  = "EmergencyModeFailed"
	RestartDeployment    = "RestartDeployment"

	WarningHittingHardMaxReplicaLimit = "HitHardMaxReplicaLimit"
	WarningReplicaValidation          = "ReplicaValidationWarning"
)
