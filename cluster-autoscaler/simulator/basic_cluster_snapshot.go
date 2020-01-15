/*
Copyright 2016 The Kubernetes Authors.

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

package simulator

import (
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	schedulerlisters "k8s.io/kubernetes/pkg/scheduler/listers"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

// BasicClusterSnapshot is simple, reference implementation of ClusterSnapshot.
// It is inefficient. But hopefully bug-free and good for initial testing.
type BasicClusterSnapshot struct {
	baseData   *internalBasicSnapshotData
	forkedData *internalBasicSnapshotData
}

type internalBasicSnapshotDataNodeLister internalBasicSnapshotData
type internalBasicSnapshotDataPodLister internalBasicSnapshotData

type internalBasicSnapshotData struct {
	nodeInfoMap map[string]*schedulernodeinfo.NodeInfo
}

func (data *internalBasicSnapshotDataNodeLister) List() ([]*schedulernodeinfo.NodeInfo, error) {
	nodeInfoList := make([]*schedulernodeinfo.NodeInfo, 0, len(data.nodeInfoMap))
	for _, v := range data.nodeInfoMap {
		nodeInfoList = append(nodeInfoList, v)
	}
	return nodeInfoList, nil
}

func (data *internalBasicSnapshotDataNodeLister) HavePodsWithAffinityList() ([]*schedulernodeinfo.NodeInfo, error) {
	havePodsWithAffinityList := make([]*schedulernodeinfo.NodeInfo, 0, len(data.nodeInfoMap))
	for _, v := range data.nodeInfoMap {
		if len(v.PodsWithAffinity()) > 0 {
			havePodsWithAffinityList = append(havePodsWithAffinityList, v)
		}
	}
	return havePodsWithAffinityList, nil
}

func (data *internalBasicSnapshotDataNodeLister) Get(nodeName string) (*schedulernodeinfo.NodeInfo, error) {
	if v, ok := data.nodeInfoMap[nodeName]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("node %s not in snapshot", nodeName)
}

func (data *internalBasicSnapshotDataPodLister) List(selector labels.Selector) ([]*apiv1.Pod, error) {
	alwaysTrue := func(p *apiv1.Pod) bool { return true }
	return data.FilteredList(alwaysTrue, selector)
}

func (data *internalBasicSnapshotDataPodLister) FilteredList(podFilter schedulerlisters.PodFilter, selector labels.Selector) ([]*apiv1.Pod, error) {
	pods := make([]*apiv1.Pod, 0)
	for _, n := range data.nodeInfoMap {
		for _, pod := range n.Pods() {
			if podFilter(pod) && selector.Matches(labels.Set(pod.Labels)) {
				pods = append(pods, pod)
			}
		}
	}
	return pods, nil
}

func (data *internalBasicSnapshotData) Pods() schedulerlisters.PodLister {
	return (*internalBasicSnapshotDataPodLister)(data)
}

func (data *internalBasicSnapshotData) NodeInfos() schedulerlisters.NodeInfoLister {
	return (*internalBasicSnapshotDataNodeLister)(data)
}

// NewEmptySnapshot initializes a Snapshot struct and returns it.
func newInternalBasicSnapshotData() *internalBasicSnapshotData {
	return &internalBasicSnapshotData{
		nodeInfoMap: make(map[string]*schedulernodeinfo.NodeInfo),
	}
}

func (data *internalBasicSnapshotData) clone() *internalBasicSnapshotData {
	clonedNodeInforMap := make(map[string]*schedulernodeinfo.NodeInfo)
	for k, v := range data.nodeInfoMap {
		clonedNodeInforMap[k] = v.Clone()
	}
	return &internalBasicSnapshotData{
		nodeInfoMap: clonedNodeInforMap,
	}
}

func (data *internalBasicSnapshotData) addNode(node *apiv1.Node) error {
	if _, found := data.nodeInfoMap[node.Name]; found {
		return fmt.Errorf("node %s already in snapshot", node.Name)
	}
	nodeInfo := schedulernodeinfo.NewNodeInfo()
	err := nodeInfo.SetNode(node)
	if err != nil {
		return fmt.Errorf("cannot set node in NodeInfo; %v", err)
	}
	data.nodeInfoMap[node.Name] = nodeInfo
	return nil
}

func (data *internalBasicSnapshotData) removeNode(nodeName string) error {
	if _, found := data.nodeInfoMap[nodeName]; !found {
		return fmt.Errorf("node %s not in snapshot", nodeName)
	}
	delete(data.nodeInfoMap, nodeName)
	return nil
}

func (data *internalBasicSnapshotData) addPod(pod *apiv1.Pod, nodeName string) error {
	if _, found := data.nodeInfoMap[nodeName]; !found {
		return fmt.Errorf("node %s not in snapshot", nodeName)
	}
	data.nodeInfoMap[nodeName].AddPod(pod)
	return nil
}

func (data *internalBasicSnapshotData) removePod(namespace string, podName string) error {
	for _, nodeInfo := range data.nodeInfoMap {
		for _, pod := range nodeInfo.Pods() {
			if pod.Namespace == namespace && pod.Name == podName {
				err := nodeInfo.RemovePod(pod)
				if err != nil {
					return fmt.Errorf("cannot remove pod; %v", err)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("pod %s/%s not in snapshot", namespace, podName)
}

func (data *internalBasicSnapshotData) getAllPods() ([]*apiv1.Pod, error) {
	var pods []*apiv1.Pod
	for _, nodeInfo := range data.nodeInfoMap {
		pods = append(pods, nodeInfo.Pods()...)
	}
	return pods, nil
}

func (data *internalBasicSnapshotData) getAllNodes() ([]*apiv1.Node, error) {
	var nodes []*apiv1.Node
	for _, nodeInfo := range data.nodeInfoMap {
		nodes = append(nodes, nodeInfo.Node())
	}
	return nodes, nil
}

// NewBasicClusterSnapshot creates instances of BasicClusterSnapshot.
func NewBasicClusterSnapshot() *BasicClusterSnapshot {
	snapshot := &BasicClusterSnapshot{}
	_ = snapshot.Clear()
	return snapshot
}

func (snapshot *BasicClusterSnapshot) getInternalData() *internalBasicSnapshotData {
	if snapshot.forkedData != nil {
		return snapshot.forkedData
	}
	return snapshot.baseData
}

// AddNode adds node to the snapshot.
func (snapshot *BasicClusterSnapshot) AddNode(node *apiv1.Node) error {
	return snapshot.getInternalData().addNode(node)
}

// RemoveNode removes nodes (and pods scheduled to it) from the snapshot.
func (snapshot *BasicClusterSnapshot) RemoveNode(nodeName string) error {
	return snapshot.getInternalData().removeNode(nodeName)
}

// AddPod adds pod to the snapshot and schedules it to given node.
func (snapshot *BasicClusterSnapshot) AddPod(pod *apiv1.Pod, nodeName string) error {
	return snapshot.getInternalData().addPod(pod, nodeName)
}

// RemovePod removes pod from the snapshot.
func (snapshot *BasicClusterSnapshot) RemovePod(namespace string, podName string) error {
	return snapshot.getInternalData().removePod(namespace, podName)
}

// GetAllPods returns list of all the pods in snapshot
func (snapshot *BasicClusterSnapshot) GetAllPods() ([]*apiv1.Pod, error) {
	return snapshot.getInternalData().getAllPods()
}

// GetAllNodes returns list of ll the nodes in snapshot
func (snapshot *BasicClusterSnapshot) GetAllNodes() ([]*apiv1.Node, error) {
	return snapshot.getInternalData().getAllNodes()
}

// Fork creates a fork of snapshot state. All modifications can later be reverted to moment of forking via Revert()
// Forking already forked snapshot is not allowed and will result with an error.
func (snapshot *BasicClusterSnapshot) Fork() error {
	if snapshot.forkedData != nil {
		return fmt.Errorf("snapshot already forked")
	}
	snapshot.forkedData = snapshot.baseData.clone()
	return nil
}

// Revert reverts snapshot state to moment of forking.
func (snapshot *BasicClusterSnapshot) Revert() error {
	snapshot.forkedData = nil
	return nil
}

// Commit commits changes done after forking.
func (snapshot *BasicClusterSnapshot) Commit() error {
	if snapshot.forkedData == nil {
		// do nothing
		return nil
	}
	snapshot.baseData = snapshot.forkedData
	snapshot.forkedData = nil
	return nil
}

// Clear reset cluster snapshot to empty, unforked state
func (snapshot *BasicClusterSnapshot) Clear() error {
	snapshot.baseData = newInternalBasicSnapshotData()
	snapshot.forkedData = nil
	return nil
}

// GetSchedulerLister exposes snapshot state as scheduler's SharedLister.
func (snapshot *BasicClusterSnapshot) GetSchedulerLister() (schedulerlisters.SharedLister, error) {
	return snapshot.getInternalData(), nil
}