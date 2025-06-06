/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package patch

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

// Helper is a utility for ensuring the proper patching of objects.
type Helper struct {
	client       client.Client
	gvk          schema.GroupVersionKind
	beforeObject client.Object
	before       *unstructured.Unstructured
	after        *unstructured.Unstructured
	changes      sets.Set[string]

	metav1ConditionsFieldPath    []string
	clusterv1ConditionsFieldPath []string
}

// NewHelper returns an initialized Helper. Use NewHelper before changing
// obj. After changing obj use Helper.Patch to persist your changes.
//
// Please note that patch helper implements a custom handling for objects implementing
// the condition.Setter interface or the conditions.Setter interface.
//
// It is also possible to implement wrappers for object not implementing those interfaces;
// in case those objects have custom conditions types the wrapper should take care of conversions.
// Additionally, if the conditions are not in the canonical place defined by the proposal for
// improving status in Cluster API conditions, locations of the condition field must be
// provided explicitly by using Metav1ConditionsFieldPath and Clusterv1ConditionsFieldPath options
// during the Patch call.
func NewHelper(obj client.Object, crClient client.Client) (*Helper, error) {
	// Return early if the object is nil.
	if util.IsNil(obj) {
		return nil, errors.New("failed to create patch helper: object is nil")
	}

	// Get the GroupVersionKind of the object,
	// used to validate against later on.
	gvk, err := apiutil.GVKForObject(obj, crClient.Scheme())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create patch helper for object %s", klog.KObj(obj))
	}

	// Identify location of the condition fields according to the canonical place defined by the proposal for
	// improving status in Cluster API conditions.
	metav1ConditionsFieldPath, clusterv1ConditionsFieldPath, err := identifyConditionsFieldsPath(obj)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to identify condition fields for object %s", klog.KObj(obj))
	}

	return &Helper{
		client:                       crClient,
		gvk:                          gvk,
		beforeObject:                 obj.DeepCopyObject().(client.Object),
		metav1ConditionsFieldPath:    metav1ConditionsFieldPath,
		clusterv1ConditionsFieldPath: clusterv1ConditionsFieldPath,
	}, nil
}

// Patch will attempt to patch the given object, including its status.
func (h *Helper) Patch(ctx context.Context, obj client.Object, opts ...Option) error {
	// Return early if the object is nil.
	if util.IsNil(obj) {
		return errors.Errorf("failed to patch %s %s: modified object is nil", h.gvk.Kind, klog.KObj(h.beforeObject))
	}

	// Get the GroupVersionKind of the object that we want to patch.
	gvk, err := apiutil.GVKForObject(obj, h.client.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to patch %s %s", h.gvk.Kind, klog.KObj(h.beforeObject))
	}
	if gvk != h.gvk {
		return errors.Errorf("failed to patch %s %s: unmatched GroupVersionKind, expected %q got %q", h.gvk.Kind, klog.KObj(h.beforeObject), h.gvk, gvk)
	}

	// Calculate the options.
	options := &HelperOptions{}
	for _, opt := range opts {
		opt.ApplyToHelper(options)
	}

	// If condition field path override have been provided, propagate them to the helper for usage in various places of this func.
	if len(options.Clusterv1ConditionsFieldPath) > 0 {
		h.clusterv1ConditionsFieldPath = options.Clusterv1ConditionsFieldPath
	}
	if len(options.Metav1ConditionsFieldPath) > 0 {
		h.metav1ConditionsFieldPath = options.Metav1ConditionsFieldPath
	}

	// Check if the object satisfies the Cluster API contract setter interfaces; if not, ignore condition field path entirely.
	if _, canInterfaceConditions := obj.(v1beta1conditions.Setter); !canInterfaceConditions {
		h.clusterv1ConditionsFieldPath = nil
	}
	if _, canInterfaceV1Beta2Conditions := obj.(conditions.Setter); !canInterfaceV1Beta2Conditions {
		h.metav1ConditionsFieldPath = nil
	}

	// Convert the before object to unstructured.
	h.before, err = toUnstructured(h.beforeObject, gvk)
	if err != nil {
		return errors.Wrapf(err, "failed to patch %s %s: failed to convert before object to Unstructured", h.gvk.Kind, klog.KObj(h.beforeObject))
	}

	// Convert the after object to unstructured.
	h.after, err = toUnstructured(obj, gvk)
	if err != nil {
		return errors.Wrapf(err, "failed to patch %s %s: failed to convert after object to Unstructured", h.gvk.Kind, klog.KObj(h.beforeObject))
	}

	// Determine if the object has status.
	if unstructuredHasStatus(h.after) {
		if options.IncludeStatusObservedGeneration {
			// Set status.observedGeneration if we're asked to do so.
			if err := unstructured.SetNestedField(h.after.Object, h.after.GetGeneration(), "status", "observedGeneration"); err != nil {
				return errors.Wrapf(err, "failed to patch %s %s: failed to set .status.observedGeneration", h.gvk.Kind, klog.KObj(h.beforeObject))
			}

			// Restore the changes back to the original object.
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(h.after.Object, obj); err != nil {
				return errors.Wrapf(err, "failed to patch %s %s: failed to converted object from Unstructured", h.gvk.Kind, klog.KObj(h.beforeObject))
			}
		}
	}

	// Calculate and store the top-level field changes (e.g. "metadata", "spec", "status") we have before/after.
	h.changes, err = h.calculateChanges(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to patch %s %s", h.gvk.Kind, klog.KObj(h.beforeObject))
	}

	// Issue patches and return errors in an aggregate.
	var errs []error
	// Patch the conditions first.
	//
	// Given that we pass in metadata.resourceVersion to perform a 3-way-merge conflict resolution,
	// patching conditions first avoids an extra loop if spec or status patch succeeds first
	// given that causes the resourceVersion to mutate.
	if err := h.patchStatusConditions(ctx, obj, options.ForceOverwriteConditions, options.OwnedConditions, options.OwnedV1Beta2Conditions); err != nil {
		errs = append(errs, err)
	}
	// Then proceed to patch the rest of the object.
	if err := h.patch(ctx, obj); err != nil {
		errs = append(errs, err)
	}

	if err := h.patchStatus(ctx, obj); err != nil {
		//nolint:staticcheck
		if !(apierrors.IsNotFound(err) && !obj.GetDeletionTimestamp().IsZero() && len(obj.GetFinalizers()) == 0) {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Wrapf(kerrors.NewAggregate(errs), "failed to patch %s %s", h.gvk.Kind, klog.KObj(h.beforeObject))
	}
	return nil
}

// patch issues a patch for metadata and spec.
func (h *Helper) patch(ctx context.Context, obj client.Object) error {
	if !h.shouldPatch(specPatch) {
		return nil
	}
	beforeObject, afterObject, err := h.calculatePatch(obj, specPatch)
	if err != nil {
		return err
	}
	return h.client.Patch(ctx, afterObject, client.MergeFrom(beforeObject))
}

// patchStatus issues a patch if the status has changed.
func (h *Helper) patchStatus(ctx context.Context, obj client.Object) error {
	if !h.shouldPatch(statusPatch) {
		return nil
	}
	beforeObject, afterObject, err := h.calculatePatch(obj, statusPatch)
	if err != nil {
		return err
	}
	return h.client.Status().Patch(ctx, afterObject, client.MergeFrom(beforeObject))
}

// patchStatusConditions issues a patch if there are any changes to the conditions slice under
// the status subresource. This is a special case and it's handled separately given that
// we allow different controllers to act on conditions of the same object.
//
// This method has an internal backoff loop. When a conflict is detected, the method
// asks the Client for the a new version of the object we're trying to patch.
//
// Condition changes are then applied to the latest version of the object, and if there are
// no unresolvable conflicts, the patch is sent again.
func (h *Helper) patchStatusConditions(ctx context.Context, obj client.Object, forceOverwrite bool, ownedConditions []clusterv1.ConditionType, ownedV1beta2Conditions []string) error {
	// Nothing to do if the object doesn't have conditions (doesn't have conditions identified as needing a special treatment).
	if len(h.clusterv1ConditionsFieldPath) == 0 && len(h.metav1ConditionsFieldPath) == 0 {
		return nil
	}

	// If the object has clusterv1 conditions, create a function applying corresponding changes if any.
	var clusterv1ApplyPatch func(client.Object) error
	if len(h.clusterv1ConditionsFieldPath) > 0 {
		// Make sure our before/after objects satisfy the proper interface before continuing.
		//
		// NOTE: The checks and error below are done so that we don't panic if any of the objects don't satisfy the
		// interface any longer, although this shouldn't happen because we already check when creating the patcher.
		before, ok := h.beforeObject.(v1beta1conditions.Getter)
		if !ok {
			return errors.Errorf("%s %s doesn't satisfy conditions.Getter, cannot patch", h.gvk.Kind, klog.KObj(h.beforeObject))
		}
		after, ok := obj.(v1beta1conditions.Getter)
		if !ok {
			return errors.Errorf("%s %s doesn't satisfy conditions.Getter, cannot compute patch", h.gvk.Kind, klog.KObj(obj))
		}

		diff, err := v1beta1conditions.NewPatch(
			before,
			after,
		)
		if err != nil {
			return errors.Wrapf(err, "%s %s can not be patched", h.gvk.Kind, klog.KObj(before))
		}
		if !diff.IsZero() {
			clusterv1ApplyPatch = func(latest client.Object) error {
				latestSetter, ok := latest.(v1beta1conditions.Setter)
				if !ok {
					return errors.Errorf("%s %s doesn't satisfy conditions.Setter, cannot apply patch", h.gvk.Kind, klog.KObj(latest))
				}

				return diff.Apply(latestSetter, v1beta1conditions.WithForceOverwrite(forceOverwrite), v1beta1conditions.WithOwnedConditions(ownedConditions...))
			}
		}
	}

	// If the object has metav1 conditions, create a function applying corresponding changes if any.
	var metav1ApplyPatch func(client.Object) error
	if len(h.metav1ConditionsFieldPath) > 0 {
		// Make sure our before/after objects satisfy the proper interface before continuing.
		//
		// NOTE: The checks and error below are done so that we don't panic if any of the objects don't satisfy the
		// interface any longer, although this shouldn't happen because we already check when creating the patcher.
		before, ok := h.beforeObject.(conditions.Getter)
		if !ok {
			return errors.Errorf("%s %s doesn't satisfy conditions.Getter, cannot patch", h.gvk.Kind, klog.KObj(h.beforeObject))
		}
		after, ok := obj.(conditions.Getter)
		if !ok {
			return errors.Errorf("%s %s doesn't satisfy conditions.Getter, cannot compute patch", h.gvk.Kind, klog.KObj(obj))
		}

		diff, err := conditions.NewPatch(
			before,
			after,
		)
		if err != nil {
			return errors.Wrapf(err, "%s %s can not be patched", h.gvk.Kind, klog.KObj(h.beforeObject))
		}

		if !diff.IsZero() {
			metav1ApplyPatch = func(latest client.Object) error {
				latestSetter, ok := latest.(conditions.Setter)
				if !ok {
					return errors.Errorf("%s %s doesn't satisfy conditions.Setter, cannot apply patch", h.gvk.Kind, klog.KObj(latest))
				}

				return diff.Apply(latestSetter, conditions.ForceOverwrite(forceOverwrite), conditions.OwnedConditionTypes(ownedV1beta2Conditions))
			}
		}
	}

	// No changes to apply, return early.
	if clusterv1ApplyPatch == nil && metav1ApplyPatch == nil {
		return nil
	}

	// Make a copy of the object and store the key used if we have conflicts.
	key := client.ObjectKeyFromObject(obj)

	// Define and start a backoff loop to handle conflicts
	// between controllers working on the same object.
	//
	// This has been copied from https://github.com/kubernetes/kubernetes/blob/release-1.16/pkg/controller/controller_utils.go#L86-L88.
	backoff := wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Jitter:   1.0,
	}

	// Start the backoff loop and return errors if any.
	return wait.ExponentialBackoff(backoff, func() (bool, error) {
		latest, ok := h.beforeObject.DeepCopyObject().(client.Object)
		if !ok {
			return false, errors.Errorf("%s %s doesn't satisfy client.Object, cannot patch", h.gvk.Kind, klog.KObj(h.beforeObject))
		}

		// Get a new copy of the object.
		if err := h.client.Get(ctx, key, latest); err != nil {
			return false, err
		}

		// Create the condition patch before merging conditions.
		conditionsPatch := client.MergeFromWithOptions(latest.DeepCopyObject().(client.Object), client.MergeFromWithOptimisticLock{})

		// Set the condition patch previously created on the new object.
		if clusterv1ApplyPatch != nil {
			if err := clusterv1ApplyPatch(latest); err != nil {
				return false, err
			}
		}
		if metav1ApplyPatch != nil {
			if err := metav1ApplyPatch(latest); err != nil {
				return false, err
			}
		}

		// Issue the patch.
		err := h.client.Status().Patch(ctx, latest, conditionsPatch)
		switch {
		case apierrors.IsConflict(err):
			// Requeue.
			return false, nil
		case err != nil:
			return false, err
		default:
			return true, nil
		}
	})
}

// calculatePatch returns the before/after objects to be given in a controller-runtime patch, scoped down to the absolute necessary.
func (h *Helper) calculatePatch(afterObj client.Object, focus patchType) (client.Object, client.Object, error) {
	// Get a shallow unsafe copy of the before/after object in unstructured form.
	before := unsafeUnstructuredCopy(h.before, focus, h.clusterv1ConditionsFieldPath, h.metav1ConditionsFieldPath)
	after := unsafeUnstructuredCopy(h.after, focus, h.clusterv1ConditionsFieldPath, h.metav1ConditionsFieldPath)

	// We've now applied all modifications to local unstructured objects,
	// make copies of the original objects and convert them back.
	beforeObj := h.beforeObject.DeepCopyObject().(client.Object)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(before.Object, beforeObj); err != nil {
		return nil, nil, err
	}
	afterObj = afterObj.DeepCopyObject().(client.Object)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(after.Object, afterObj); err != nil {
		return nil, nil, err
	}
	return beforeObj, afterObj, nil
}

func (h *Helper) shouldPatch(focus patchType) bool {
	if focus == specPatch {
		// If we're looking to patch anything other than status,
		// return true if the changes map has any fields after removing `status`.
		return h.changes.Clone().Delete("status").Len() > 0
	}
	return h.changes.Has(string(focus))
}

// calculate changes tries to build a patch from the before/after objects we have
// and store in a map which top-level fields (e.g. `metadata`, `spec`, `status`, etc.) have changed.
func (h *Helper) calculateChanges(after client.Object) (sets.Set[string], error) {
	// Calculate patch data.
	patch := client.MergeFrom(h.beforeObject)
	diff, err := patch.Data(after)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to calculate patch data")
	}

	// Unmarshal patch data into a local map.
	patchDiff := map[string]interface{}{}
	if err := json.Unmarshal(diff, &patchDiff); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal patch data into a map")
	}

	// Return the map.
	res := sets.New[string]()
	for key := range patchDiff {
		res.Insert(key)
	}
	return res, nil
}
