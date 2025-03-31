package metrics

import (
	"github.com/mercari/tortoise/api/v1beta3"
)

func RecordTortoise(t *v1beta3.Tortoise, deleted bool) {
	value := 1.0
	if deleted {
		value = 0
	}

	for _, um := range []v1beta3.UpdateMode{v1beta3.UpdateModeOff, v1beta3.UpdateModeAuto, v1beta3.UpdateModeEmergency} {
		if t.Spec.UpdateMode != um {
			// Set 0 to reset.
			TortoiseNumber.WithLabelValues(
				t.Name,
				t.Namespace,
				t.Spec.TargetRefs.ScaleTargetRef.Name,
				t.Spec.TargetRefs.ScaleTargetRef.Kind,
				string(um),
				string(t.Status.TortoisePhase),
			).Set(0.0)
			continue
		}

		TortoiseNumber.WithLabelValues(
			t.Name,
			t.Namespace,
			t.Spec.TargetRefs.ScaleTargetRef.Name,
			t.Spec.TargetRefs.ScaleTargetRef.Kind,
			string(t.Spec.UpdateMode),
			string(t.Status.TortoisePhase),
		).Set(value)
	}
}

func ShouldRerecordTortoise(old, new *v1beta3.Tortoise) bool {
	return old.Status.TortoisePhase != new.Status.TortoisePhase ||
		old.Spec.UpdateMode != new.Spec.UpdateMode
}
