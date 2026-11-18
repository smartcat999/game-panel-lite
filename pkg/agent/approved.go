package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

// approvedInstance detaches execution from the server's desired resource fields.
// Both requests and limits use the complete admitted budget.
func approvedInstance(in *corev1.Instance, nodeName, nodeUID string) (*corev1.Instance, error) {
	if in == nil || in.Metadata.UID == "" || nodeUID == "" {
		return nil, fmt.Errorf("execution requires instance and node UIDs")
	}
	c := in.Status.ResourceControl
	if c == nil || c.ApprovedRevision <= 0 || c.CurrentAttempt == nil {
		return nil, fmt.Errorf("instance has no approved binding")
	}
	if _, err := frozenRuntimeClass(in); err != nil {
		return nil, err
	}
	a := c.CurrentAttempt
	if a.ID == "" || a.Epoch <= 0 || a.NodeUID != nodeUID || a.NodeName != nodeName || in.Spec.NodeName != nodeName {
		return nil, fmt.Errorf("instance binding does not match this node")
	}
	deleting := in.Metadata.DeletionTimestamp != nil && c.Stage == corev1.InstanceStageDeleting
	if a.Phase != corev1.AttemptPhaseBound && (!deleting || a.Phase != corev1.AttemptPhaseClosing) {
		return nil, fmt.Errorf("instance attempt is not executable")
	}
	if c.Stage != corev1.InstanceStageActive && !deleting || in.Metadata.DeletionTimestamp != nil && !deleting {
		return nil, fmt.Errorf("instance lifecycle is not executable")
	}
	if len(c.ApprovedResources) == 0 {
		return nil, fmt.Errorf("approved resources are required")
	}
	for _, port := range c.ApprovedPorts {
		if port.PublishedPort <= 0 || port.PublishedPort > 65535 || port.TargetPort <= 0 || port.TargetPort > 65535 {
			return nil, fmt.Errorf("approved ports must contain concrete valid assignments")
		}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out corev1.Instance
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	resources, err := corev1.ParseResources(c.ApprovedResources)
	if err != nil {
		return nil, err
	}
	if err := resources.ValidateInstance(); err != nil {
		return nil, err
	}
	out.Spec.Resources = corev1.InstanceRequirements(resources)
	out.Spec.Ports = append([]corev1.InstancePort(nil), c.ApprovedPorts...)
	out.Spec.State = corev1.EffectiveInstanceState(in)
	return &out, nil
}

func sameExecution(a, b *corev1.Instance) bool {
	if a == nil || b == nil || a.Metadata.UID == "" || a.Metadata.UID != b.Metadata.UID || a.Metadata.Generation != b.Metadata.Generation {
		return false
	}
	x, y := a.Status.ResourceControl, b.Status.ResourceControl
	if x == nil || y == nil || x.CurrentAttempt == nil || y.CurrentAttempt == nil {
		return false
	}
	p, q := x.CurrentAttempt, y.CurrentAttempt
	return x.ApprovedRevision == y.ApprovedRevision && x.Suspended == y.Suspended && p.ID == q.ID && p.Epoch == q.Epoch && p.NodeUID == q.NodeUID && p.NodeName == q.NodeName && p.Phase == q.Phase && x.Stage == y.Stage && reflect.DeepEqual(x.RuntimeClass, y.RuntimeClass) && reflect.DeepEqual(x.ApprovedResources, y.ApprovedResources) && reflect.DeepEqual(x.ApprovedPorts, y.ApprovedPorts) && (a.Metadata.DeletionTimestamp == nil) == (b.Metadata.DeletionTimestamp == nil)
}

func sameBinding(a, b *corev1.Instance) bool {
	if a == nil || b == nil || a.Metadata.UID == "" || a.Metadata.UID != b.Metadata.UID {
		return false
	}
	x, y := a.Status.ResourceControl, b.Status.ResourceControl
	if x == nil || y == nil || x.CurrentAttempt == nil || y.CurrentAttempt == nil {
		return false
	}
	p, q := x.CurrentAttempt, y.CurrentAttempt
	return p.ID == q.ID && p.Epoch == q.Epoch && p.NodeUID == q.NodeUID && p.NodeName == q.NodeName && reflect.DeepEqual(x.RuntimeClass, y.RuntimeClass)
}

func canonicalObservedPorts(ports []corev1.InstancePort) []corev1.InstancePort {
	out := append([]corev1.InstancePort{}, ports...)
	for i := range out {
		out[i].Protocol = strings.ToUpper(out[i].Protocol)
		if out[i].Protocol == "" {
			out[i].Protocol = corev1.ProtocolTCP
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := json.Marshal(out[i])
		b, _ := json.Marshal(out[j])
		return string(a) < string(b)
	})
	return out
}

func verifiedObservation(inst *corev1.Instance, obs *runtime.RuntimeObservation, started time.Time) error {
	if obs == nil || obs.ObservedAt == nil || obs.ObservedAt.IsZero() || obs.ObservedAt.Before(started) || obs.ObservedAt.After(time.Now().Add(time.Second)) {
		return fmt.Errorf("runtime returned no fresh execution receipt")
	}
	wanted := corev1.InstancePhaseRunning
	if corev1.EffectiveInstanceState(inst) == corev1.InstanceStateStopped {
		wanted = corev1.InstancePhaseStopped
	}
	if obs.Phase != wanted {
		return fmt.Errorf("runtime phase %s does not confirm desired state %s", obs.Phase, wanted)
	}
	applied, err := corev1.ParseResources(obs.AppliedResources)
	if err != nil {
		return fmt.Errorf("invalid applied resources: %w", err)
	}
	if err := applied.ValidateInstance(); err != nil {
		return fmt.Errorf("incomplete applied resources: %w", err)
	}
	approved, err := corev1.ParseResources(inst.Status.ResourceControl.ApprovedResources)
	if err != nil || applied != approved {
		return fmt.Errorf("runtime applied resources do not match the approved budget")
	}
	if !reflect.DeepEqual(canonicalObservedPorts(obs.AppliedPorts), canonicalObservedPorts(inst.Status.ResourceControl.ApprovedPorts)) {
		return fmt.Errorf("runtime applied ports do not match every approved assignment")
	}
	return nil
}
