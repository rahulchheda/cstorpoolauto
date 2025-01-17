/*
Copyright 2019 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterconfigplan

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/controller/generic"

	"cstorpoolauto/k8s"
	"cstorpoolauto/types"
)

type reconcileErrHandler struct {
	clusterPlan  *unstructured.Unstructured
	hookResponse *generic.SyncHookResponse
}

func (h *reconcileErrHandler) handle(err error) {
	// Error has been handled elaborately. This logic ensures
	// error message is propagated to the resource & hence seen via
	// 'kubectl get CStorClusterPlan -oyaml'.
	//
	// In addition, errors are logged as well.
	glog.Errorf(
		"Failed to reconcile CStorClusterPlan %s: %v", h.clusterPlan.GetName(), err,
	)

	conds, mergeErr :=
		k8s.MergeStatusConditions(
			h.clusterPlan,
			types.MakeCStorClusterPlanReconcileErrCond(err),
		)
	if mergeErr != nil {
		glog.Errorf(
			"Failed to reconcile CStorClusterPlan %s: Can't set status conditions: %v",
			h.clusterPlan.GetName(), mergeErr,
		)
		// Note: Merge error will reset the conditions which will make
		// things worse since various controllers will be reconciling
		// based on these conditions.
		//
		// Hence it is better to set response status as nil to let metac
		// preserve old status conditions if any.
		h.hookResponse.Status = nil
	} else {
		// response status will be set against the watch's status by metac
		h.hookResponse.Status = map[string]interface{}{}
		h.hookResponse.Status["phase"] = types.CStorClusterPlanStatusPhaseError
		h.hookResponse.Status["conditions"] = conds
	}

	// stop further reconciliation since there was an error
	h.hookResponse.SkipReconcile = true
}

// Sync implements the idempotent logic reconcile CStorClusterPlan
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest uses CStorClusterPlan as the watched resource.
// SyncHookResponse has the resources that forms the desired state
// w.r.t the watched resource.
//
// NOTE:
//	Returning error will panic this process. We would rather want this
// controller to run continuously. Hence, the errors are logged and at
// the same time, these errors are posted against CStorClusterPlan's
// status.
func Sync(request *generic.SyncHookRequest, response *generic.SyncHookResponse) error {
	response = &generic.SyncHookResponse{}

	// construct the error handler
	errHandler := &reconcileErrHandler{
		clusterPlan:  request.Watch,
		hookResponse: response,
	}

	var observedStorageSet []*unstructured.Unstructured
	var cstorClusterConfig *unstructured.Unstructured
	for _, attachment := range request.Attachments.List() {
		if attachment.GetKind() == string(types.KindCStorClusterStorageSet) {
			// verify further if CStorClusterStorageSet belongs to current watch
			uid, _ := k8s.GetAnnotationForKey(
				attachment.GetAnnotations(), types.AnnKeyCStorClusterPlanUID,
			)
			if string(request.Watch.GetUID()) == uid {
				// this is a desired CStorClusterStorageSet
				observedStorageSet = append(observedStorageSet, attachment)
				// we don't want to add this CStorClusterStorageSet now
				// but later after reconciliation
				continue
			}
		}
		if attachment.GetKind() == string(types.KindCStorClusterConfig) {
			// verify further if CStorClusterConfig belongs to current watch
			uid, _ := k8s.GetAnnotationForKey(
				request.Watch.GetAnnotations(), types.AnnKeyCStorClusterConfigUID,
			)
			if string(attachment.GetUID()) == uid {
				// this is a desired CStorClusterConfig
				cstorClusterConfig = attachment
			}
		}
		// add attachments as-is if they are not of kind CStorClusterStorageSet
		response.Attachments = append(response.Attachments, attachment)
	}
	if cstorClusterConfig == nil {
		errHandler.handle(errors.Errorf("Missing CStorClusterConfig attachment"))
		return nil
	}

	reconciler, err := NewReconciler(request.Watch, cstorClusterConfig, observedStorageSet)
	if err != nil {
		errHandler.handle(err)
		return nil
	}
	op, err := reconciler.Reconcile()
	if err != nil {
		errHandler.handle(err)
		return nil
	}
	response.Attachments = append(response.Attachments, op.DesiredStorageSet...)
	response.Status = op.Status
	return nil
}

// Reconciler enables reconciliation of CStorClusterPlan instance
type Reconciler struct {
	CStorClusterPlan   *types.CStorClusterPlan
	CStorClusterConfig *types.CStorClusterConfig
	ObservedStorageSet []*unstructured.Unstructured
}

// ReconcileResponse forms the response due to reconciliation of
// CStorClusterPlan
type ReconcileResponse struct {
	DesiredStorageSet []*unstructured.Unstructured
	Status            map[string]interface{}
}

// NewReconciler returns a new instance of reconciler
func NewReconciler(
	plan *unstructured.Unstructured,
	clusterConfig *unstructured.Unstructured,
	observedStorageSet []*unstructured.Unstructured,
) (*Reconciler, error) {
	// transforms cluster plan from unstructured to typed
	var cstorClusterPlanTyped *types.CStorClusterPlan
	cstorClusterPlanRaw, err := plan.MarshalJSON()
	if err != nil {
		return nil, errors.Wrapf(err, "Can't marshal CStorClusterPlan")
	}
	err = json.Unmarshal(cstorClusterPlanRaw, cstorClusterPlanTyped)
	if err != nil {
		return nil, errors.Wrapf(err, "Can't unmarshal CStorClusterPlan")
	}
	// transforms cluster config from unstructured to typed
	var cstorClusterConfigTyped *types.CStorClusterConfig
	cstorClusterConfigRaw, err := clusterConfig.MarshalJSON()
	if err != nil {
		return nil, errors.Wrapf(err, "Can't marshal CStorClusterConfig")
	}
	err = json.Unmarshal(cstorClusterConfigRaw, cstorClusterConfigTyped)
	if err != nil {
		return nil, errors.Wrapf(err, "Can't unmarshal CStorClusterConfig")
	}
	// use above constructed objects to build Reconciler instance
	return &Reconciler{
		CStorClusterPlan:   cstorClusterPlanTyped,
		CStorClusterConfig: cstorClusterConfigTyped,
		ObservedStorageSet: observedStorageSet,
	}, nil
}

// Reconcile observed state of CStorClusterPlan to its desired
// state
func (r *Reconciler) Reconcile() (ReconcileResponse, error) {
	planner, err := NewStorageSetPlanner(
		r.CStorClusterPlan,
		r.ObservedStorageSet,
	)
	if err != nil {
		return ReconcileResponse{}, err
	}
	desiredStorageSet, err := planner.Plan(r.CStorClusterConfig)
	if err != nil {
		return ReconcileResponse{}, err
	}
	return ReconcileResponse{
		DesiredStorageSet: desiredStorageSet,
		Status:            types.MakeCStorClusterPlanStatusToOnline(r.CStorClusterPlan),
	}, nil
}

// StorageSetPlanner ensures if any CStorClusterStorageSet instance
// need to be created, deleted, updated or perhaps does not require
// any changes at all.
type StorageSetPlanner struct {
	ClusterPlan *types.CStorClusterPlan

	// NOTE:
	// All the maps in this structure have node UID as their keys
	ObservedStorageSetObjs map[string]*unstructured.Unstructured
	ObservedStorageSets    map[string]bool

	IsCreate map[string]bool // map of newly desired nodes
	IsRemove map[string]bool // map of nodes that are no more needed
	IsNoop   map[string]bool // map of nodes that are desired & is already in-use

	PlannedNodeNames map[string]string // map of desired node names
	Updates          map[string]string // map of not needed to newly desired nodes
}

// NewStorageSetPlanner returns a new instance of StorageSetPlanner
func NewStorageSetPlanner(
	plan *types.CStorClusterPlan,
	observedStorageSet []*unstructured.Unstructured,
) (*StorageSetPlanner, error) {
	// initialize the planner
	planner := &StorageSetPlanner{
		ClusterPlan:            plan,
		ObservedStorageSetObjs: map[string]*unstructured.Unstructured{},
		ObservedStorageSets:    map[string]bool{},
		IsCreate:               map[string]bool{},
		IsRemove:               map[string]bool{},
		IsNoop:                 map[string]bool{},
		PlannedNodeNames:       map[string]string{},
		Updates:                map[string]string{},
	}
	for _, storageSet := range observedStorageSet {
		// verify further if this belongs to the current watch
		uid, found, err := unstructured.NestedString(
			storageSet.UnstructuredContent(), "spec", "node", "uid",
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"Failed to get spec.node.uid: StorageSet %s %s",
				storageSet.GetNamespace(), storageSet.GetName(),
			)
		}
		if !found {
			return nil, errors.Errorf(
				"Invalid StorageSet %s %s: Missing spec.node.uid",
				storageSet.GetNamespace(), storageSet.GetName(),
			)
		}
		planner.ObservedStorageSets[uid] = true
		planner.ObservedStorageSetObjs[uid] = storageSet
	}
	for _, plannedNode := range plan.Spec.Nodes {
		// store node uid to name mapping
		planner.PlannedNodeNames[string(plannedNode.UID)] = plannedNode.Name
		// planned nodes need to get into some bucket
		if planner.ObservedStorageSets[string(plannedNode.UID)] {
			// this node is desired and is observed
			planner.IsNoop[string(plannedNode.UID)] = true
		} else {
			// this node is desired and is not observed
			planner.IsCreate[string(plannedNode.UID)] = true
		}
	}
	// there may be more observed nodes than what is
	// planned currently
	for observedNode := range planner.ObservedStorageSets {
		if planner.IsNoop[observedNode] || planner.IsCreate[observedNode] {
			continue
		}
		planner.IsRemove[observedNode] = true
	}
	// build update inventory i.e. move observed storageset's
	// to a new planned node based on create & remove inventory
	for removeNode := range planner.IsRemove {
		for createNode := range planner.IsCreate {
			planner.Updates[removeNode] = createNode
			// nullify create & remove inventory since
			// they are accomodated by update inventory
			planner.IsRemove[removeNode] = false
			planner.IsCreate[createNode] = false
		}
	}
	return planner, nil
}

// Plan provides the list of desired StorageSets
func (p *StorageSetPlanner) Plan(config *types.CStorClusterConfig) ([]*unstructured.Unstructured, error) {
	var finalStorageSets []*unstructured.Unstructured
	noopObjs := p.noop()
	createObjs := p.create(config)
	p.remove()
	updateObjs, err := p.update()
	if err != nil {
		return nil, err
	}
	finalStorageSets = append(finalStorageSets, noopObjs...)
	finalStorageSets = append(finalStorageSets, createObjs...)
	finalStorageSets = append(finalStorageSets, updateObjs...)
	return finalStorageSets, nil
}

func (p *StorageSetPlanner) noop() []*unstructured.Unstructured {
	var storageSets []*unstructured.Unstructured
	for uid, isnoop := range p.IsNoop {
		if !isnoop {
			continue
		}

		storageSets = append(storageSets, p.ObservedStorageSetObjs[uid])
	}
	return storageSets
}

func (p *StorageSetPlanner) create(config *types.CStorClusterConfig) []*unstructured.Unstructured {
	var storageSets []*unstructured.Unstructured
	for uid, iscreate := range p.IsCreate {
		if !iscreate {
			continue
		}

		storageSet := &unstructured.Unstructured{}
		storageSet.SetUnstructuredContent(map[string]interface{}{
			"metadata": map[string]interface{}{
				"apiVersion":   "dao.mayadata.io/v1alpha1",
				"kind":         "CStorClusterStorageSet",
				"generateName": "ccplan-", // ccplan -> CStorClusterPlan
				"namespace":    p.ClusterPlan.GetNamespace(),
				"annotations": map[string]interface{}{
					string(types.AnnKeyCStorClusterPlanUID): p.ClusterPlan.GetUID(),
				},
			},
			"spec": map[string]interface{}{
				"node": map[string]interface{}{
					"name": p.PlannedNodeNames[uid],
					"uid":  uid,
				},
				"disk": map[string]interface{}{
					"capacity": config.Spec.DiskConfig.MinCapacity,
					"count":    config.Spec.DiskConfig.MinCount,
				},
				"externalProvisioner": map[string]interface{}{
					"csiAttacherName":  config.Spec.DiskConfig.ExternalProvisioner.CSIAttacherName,
					"storageClassName": config.Spec.DiskConfig.ExternalProvisioner.StorageClassName,
				},
			},
		})
		storageSets = append(storageSets, storageSet)
	}
	return storageSets
}

func (p *StorageSetPlanner) remove() {
	// this is a noop
	// metac will remove the resources if they were
	// available in the request but were not sent in the response
	for uid, isremove := range p.IsRemove {
		if !isremove {
			continue
		}
		// log it for debuggability purposes
		glog.V(3).Infof(
			"Will remove CStorClusterStorageSet %s/%s having node uid %s",
			p.ObservedStorageSetObjs[uid].GetNamespace(),
			p.ObservedStorageSetObjs[uid].GetName(),
			uid,
		)
	}
}

func (p *StorageSetPlanner) update() ([]*unstructured.Unstructured, error) {
	var updatedStorageSets []*unstructured.Unstructured
	for oldNode, newNode := range p.Updates {
		storageSet := p.ObservedStorageSetObjs[oldNode]
		copy := storageSet.DeepCopy()
		// set new node details
		node := map[string]string{
			"name": p.PlannedNodeNames[newNode],
			"uid":  newNode,
		}
		err := unstructured.SetNestedField(copy.Object, node, "spec", "node")
		if err != nil {
			return nil, err
		}
		updatedStorageSets = append(updatedStorageSets, copy)
	}
	return updatedStorageSets, nil
}
