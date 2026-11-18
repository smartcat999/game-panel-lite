package authorization

import (
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

// bootstrapNodePrincipal is the identity assigned to the shared node bootstrap token.
const bootstrapNodePrincipal = iamv1.PrincipalNodeAgent

func isReadVerb(v string) bool {
	return v == iamv1.VerbGet || v == iamv1.VerbList || v == iamv1.VerbWatch
}

// nodeRestriction returns a non-empty denial reason when a node principal steps outside what a
// node-agent legitimately needs: register/heartbeat its own Node, read RuntimeClasses, list the
// instances scheduled to it, report their status, and release the cleanup finalizer of its own
// instances once they are being deleted. Everything else is denied.
func nodeRestriction(a AttributesRecord, target *corev1.Instance) string {
	self := a.User.Name
	if a.APIGroup == registryv1.GroupName {
		return nodeRegistryRestriction(a)
	}
	if a.APIGroup != corev1.GroupName {
		return "NodeRestriction: nodes may only access " + corev1.GroupName + " and " + registryv1.GroupName + " resources"
	}
	switch a.Resource {
	case "nodes":
		if a.Subresource != "" && a.Subresource != "status" {
			return "NodeRestriction: nodes cannot access node subresource " + a.Subresource
		}
		switch a.Verb {
		case iamv1.VerbCreate:
			if a.Subresource != "" {
				return "NodeRestriction: nodes cannot create node subresource " + a.Subresource
			}
			if a.Name != "" && a.Name != self {
				return "NodeRestriction: node " + self + " cannot create node " + a.Name
			}
			return ""
		case iamv1.VerbGet, iamv1.VerbUpdate, iamv1.VerbPatch:
			if a.Name != self {
				return "NodeRestriction: node " + self + " cannot access node " + a.Name
			}
			return ""
		case iamv1.VerbList, iamv1.VerbWatch:
			return "NodeRestriction: nodes cannot list or watch Node objects"
		case iamv1.VerbDelete:
			return "NodeRestriction: nodes cannot delete Node objects"
		default:
			return "NodeRestriction: nodes cannot perform " + a.Verb + " on Node objects"
		}
	case "runtimeclasses", "apps":
		if a.Subresource == "" && isReadVerb(a.Verb) {
			return ""
		}
		return "NodeRestriction: nodes cannot modify " + a.Resource
	case "prewarmjobs":
		if a.Subresource == "" && isReadVerb(a.Verb) {
			return ""
		}
		if a.Subresource == "status" && a.Name != "" && (a.Verb == iamv1.VerbGet || a.Verb == iamv1.VerbUpdate || a.Verb == iamv1.VerbPatch) {
			return "" // handler enforces that a node only reports its own status.nodes entry
		}
		return "NodeRestriction: nodes cannot " + a.Verb + " prewarmjobs/" + a.Subresource
	case "instances":
		return nodeInstanceRestriction(a, self, target)
	}
	return "NodeRestriction: nodes cannot access " + a.Resource
}

func nodeInstanceRestriction(a AttributesRecord, self string, target *corev1.Instance) string {
	if a.Name == "" && (a.Verb == iamv1.VerbList || a.Verb == iamv1.VerbWatch) {
		return ""
	}
	if a.Name == "" {
		return "NodeRestriction: nodes cannot " + a.Verb + " instances"
	}
	if target == nil || target.Spec.NodeName != self {
		return "NodeRestriction: node " + self + " may only access instances scheduled onto itself"
	}
	switch a.Subresource {
	case "":
		if a.Verb == iamv1.VerbGet {
			return ""
		}
		if (a.Verb == iamv1.VerbUpdate || a.Verb == iamv1.VerbPatch) && target.Metadata.DeletionTimestamp != nil {
			return "" // releasing the cleanup finalizer of an instance being deleted
		}
		return "NodeRestriction: nodes may not change instance spec"
	case "status":
		if a.Verb == iamv1.VerbGet || a.Verb == iamv1.VerbUpdate || a.Verb == iamv1.VerbPatch {
			return ""
		}
	}
	return "NodeRestriction: nodes cannot " + a.Verb + " instances/" + a.Subresource
}

func nodeRegistryRestriction(a AttributesRecord) string {
	switch a.Resource {
	case "repositories", "packages", "artifacts", "blobs":
		if a.Subresource == "" && isReadVerb(a.Verb) {
			return ""
		}
		return "NodeRestriction: nodes may only read " + a.Resource
	default:
		return "NodeRestriction: nodes cannot access " + a.Resource
	}
}
