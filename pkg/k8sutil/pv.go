package k8sutil

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IPVController interface {
	GetPV(string) (*corev1.PersistentVolume, error)
	UpdatePV(pv *corev1.PersistentVolume) error
}

type pvController struct {
	client.Client
}

func NewPVController(client client.Client) IPVController {
	return &pvController{client}
}

func (pv *pvController) GetPV(name string) (*corev1.PersistentVolume, error) {
	var pvObj corev1.PersistentVolume
	if err := pv.Get(context.TODO(), types.NamespacedName{
		Namespace: corev1.NamespaceAll,
		Name:      name,
	}, &pvObj); err != nil {
		return nil, err
	}
	return &pvObj, nil
}

func (pv *pvController) UpdatePV(pvObj *corev1.PersistentVolume) error {
	if err := pv.Update(context.TODO(), pvObj); err != nil {
		return err
	}
	return nil
}
