package k8sutil

import (
	"context"

	iapetosapiv1 "github.com/q8s-io/iapetos/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IStatefulPodControl interface {
	// CreateStatefulSet creates a StatefulSet in a DistributedRedisCluster.
	CreateStatefulPod(*iapetosapiv1.StatefulPod) error
	// UpdateStatefulSet updates a StatefulSet in a DistributedRedisCluster.
	UpdateStatefulPod(*iapetosapiv1.StatefulPod) error
	// DeleteStatefulSet deletes a StatefulSet in a DistributedRedisCluster.
	DeleteStatefulPod(*iapetosapiv1.StatefulPod) error
	DeleteStatefulPodByName(namespace, name string) error
	// GetStatefulSet get StatefulSet in a DistributedRedisCluster.
	GetStatefulPod(namespace, name string) (*iapetosapiv1.StatefulPod, error)
	ListStatefulPodByLabels(namespace string, labels map[string]string) (*iapetosapiv1.StatefulPodList, error)
	// GetStatefulSetPods will retrieve the pods managed by a given StatefulSet.
	GetStatefulPodPods(namespace, name string) (*corev1.PodList, error)
	GetStatefulPodPodsByLabels(namespace string, labels map[string]string) (*corev1.PodList, error)
}

type statefulPodController struct {
	client client.Client
}

func NewStatefulPodController(client client.Client) IStatefulPodControl {
	return &statefulPodController{client: client}
}

func (s *statefulPodController) CreateStatefulPod(sp *iapetosapiv1.StatefulPod) error {
	return s.client.Create(context.TODO(), sp)
}

func (s *statefulPodController) UpdateStatefulPod(sp *iapetosapiv1.StatefulPod) error {
	return s.client.Update(context.TODO(), sp)
}

func (s *statefulPodController) DeleteStatefulPod(sp *iapetosapiv1.StatefulPod) error {
	return s.client.Delete(context.TODO(), sp)
}

func (s *statefulPodController) ListStatefulPodByLabels(namespace string, labels map[string]string) (*iapetosapiv1.StatefulPodList, error) {
	sp := &iapetosapiv1.StatefulPodList{}
	err := s.client.List(context.TODO(), sp, client.InNamespace(namespace), client.MatchingLabels(labels))
	return sp, err
}

func (s *statefulPodController) DeleteStatefulPodByName(namespace, name string) error {
	sp, err := s.GetStatefulPod(namespace, name)
	if err != nil {
		return err
	}
	return s.DeleteStatefulPod(sp)
}

func (s *statefulPodController) GetStatefulPodPods(namespace, name string) (*corev1.PodList, error) {
	sp, err := s.GetStatefulPod(namespace, name)
	if err != nil {
		return nil, err
	}
	match := client.MatchingLabels{}
	for k, v := range sp.Spec.Selector.MatchLabels {
		match[k] = v
	}
	pods := &corev1.PodList{}
	err = s.client.List(context.TODO(), pods, client.InNamespace(namespace), match)
	return pods, err
}

func (s *statefulPodController) GetStatefulPodPodsByLabels(namespace string, labels map[string]string) (*corev1.PodList, error) {
	foundPods := &corev1.PodList{}
	err := s.client.List(context.TODO(), foundPods, client.InNamespace(namespace), client.MatchingLabels(labels))
	return foundPods, err
}

func (s *statefulPodController) GetStatefulPod(namespace, name string) (*iapetosapiv1.StatefulPod, error) {
	sp := &iapetosapiv1.StatefulPod{}
	err := s.client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, sp)
	return sp, err
}
