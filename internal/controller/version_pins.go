package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	databasev1alpha1 "github.com/sudhir-u/my-mysql-operator/api/v1alpha1"
)

const (
	defaultPinsConfigMap = "my-mysql-operator-mysql-channel-pins"
	pinsConfigMapKey     = "channels.yaml"
)

// channelPinsFile is the Offering-owned YAML shape inside the ConfigMap.
type channelPinsFile struct {
	Channels map[string]string `json:"channels"`
}

// IsChannel reports whether version is a major.minor channel (e.g. "8.4"), not a patch pin.
func IsChannel(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// ResolveImageTag returns the concrete MySQL image tag for Spec.Version using channel pins.
// Explicit pins (e.g. "8.4.13") are returned unchanged. Missing pin entries keep the channel as-is.
func ResolveImageTag(specVersion string, pins map[string]string) string {
	if IsChannel(specVersion) {
		if pin, ok := pins[specVersion]; ok && pin != "" {
			return pin
		}
	}
	return specVersion
}

// IsPinDrivenUpgrade is true when Spec is a channel and the desired tag comes from the pin map.
func IsPinDrivenUpgrade(specVersion string, pins map[string]string, desiredTag string) bool {
	if !IsChannel(specVersion) {
		return false
	}
	pin, ok := pins[specVersion]
	return ok && pin != "" && pin == desiredTag
}

func (r *MySQLReconciler) pinsConfigMapName() string {
	if v := os.Getenv("MYSQL_CHANNEL_PINS_CONFIGMAP"); v != "" {
		return v
	}
	return defaultPinsConfigMap
}

func (r *MySQLReconciler) pinsNamespace() string {
	if v := os.Getenv("POD_NAMESPACE"); v != "" {
		return v
	}
	return "my-mysql-operator-system"
}

// loadChannelPins reads Offering pins from the ConfigMap. Missing ConfigMap yields an empty map.
func (r *MySQLReconciler) loadChannelPins(ctx context.Context) (map[string]string, error) {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: r.pinsConfigMapName(), Namespace: r.pinsNamespace()}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	raw, ok := cm.Data[pinsConfigMapKey]
	if !ok || raw == "" {
		return map[string]string{}, nil
	}
	var parsed channelPinsFile
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse channel pins ConfigMap: %w", err)
	}
	if parsed.Channels == nil {
		return map[string]string{}, nil
	}
	return parsed.Channels, nil
}

// InMaintenanceWindow reports whether nowUTC falls in the daily [start, end) window (UTC).
func InMaintenanceWindow(nowUTC time.Time, w *databasev1alpha1.MaintenanceWindowSpec) bool {
	if w == nil {
		return false
	}
	startMin, err1 := parseHHMM(w.Start)
	endMin, err2 := parseHHMM(w.End)
	if err1 != nil || err2 != nil {
		return false
	}
	nowMin := nowUTC.Hour()*60 + nowUTC.Minute()
	if startMin == endMin {
		return true // full day
	}
	if startMin < endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	// Crosses midnight
	return nowMin >= startMin || nowMin < endMin
}

// TimeUntilMaintenanceWindow returns a positive duration until the next window start (UTC).
func TimeUntilMaintenanceWindow(nowUTC time.Time, w *databasev1alpha1.MaintenanceWindowSpec) time.Duration {
	if w == nil {
		return time.Hour
	}
	startMin, err := parseHHMM(w.Start)
	if err != nil {
		return time.Hour
	}
	nowMin := nowUTC.Hour()*60 + nowUTC.Minute()
	var deltaMin int
	if nowMin < startMin {
		deltaMin = startMin - nowMin
	} else {
		deltaMin = 24*60 - nowMin + startMin
	}
	if deltaMin <= 0 {
		deltaMin = 1
	}
	return time.Duration(deltaMin) * time.Minute
}

func parseHHMM(s string) (int, error) {
	var h, m int
	_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid HH:MM %q", s)
	}
	return h*60 + m, nil
}
