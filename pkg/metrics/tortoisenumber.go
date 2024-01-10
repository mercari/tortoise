package metrics

import (
	"github.com/mercari/tortoise/api/v1beta3"
)

func RecordTortoise(t *v1beta3.Tortoise, deleted bool) {
	value := 1.0
	if deleted {
		value = 0
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

func ShouldRerecordTortoise(old, new *v1beta3.Tortoise) bool {
	return old.Status.TortoisePhase != new.Status.TortoisePhase ||
		old.Spec.UpdateMode != new.Spec.UpdateMode
}
