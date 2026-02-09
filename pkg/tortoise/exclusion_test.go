package tortoise

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mercari/tortoise/api/v1beta3"
)

func TestService_IsChangeApplicationDisabled(t *testing.T) {
	tests := []struct {
		name               string
		globalDisableMode  bool
		excludedNamespaces []string
		tortoise           *v1beta3.Tortoise
		wantDisabled       bool
		wantReason         string
	}{
		{
			name:               "disabled by global mode",
			globalDisableMode:  true,
			excludedNamespaces: []string{},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
			},
			wantDisabled: true,
			wantReason:   "GlobalDisableMode",
		},
		{
			name:               "disabled by exclusion list",
			globalDisableMode:  false,
			excludedNamespaces: []string{"kube-system", "default"},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
			},
			wantDisabled: true,
			wantReason:   "NamespaceExclusion",
		},
		{
			name:               "enabled (namespace not excluded)",
			globalDisableMode:  false,
			excludedNamespaces: []string{"kube-system"},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
			},
			wantDisabled: false,
			wantReason:   "",
		},
		{
			name:               "global disable takes precedence",
			globalDisableMode:  true,
			excludedNamespaces: []string{"default"},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
			},
			wantDisabled: true,
			wantReason:   "GlobalDisableMode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				globalDisableMode:  tt.globalDisableMode,
				excludedNamespaces: sets.New(tt.excludedNamespaces...),
			}
			gotDisabled, gotReason := s.IsChangeApplicationDisabled(context.Background(), tt.tortoise)
			if gotDisabled != tt.wantDisabled {
				t.Errorf("IsChangeApplicationDisabled() gotDisabled = %v, want %v", gotDisabled, tt.wantDisabled)
			}
			if gotReason != tt.wantReason {
				t.Errorf("IsChangeApplicationDisabled() gotReason = %v, want %v", gotReason, tt.wantReason)
			}
		})
	}
}
