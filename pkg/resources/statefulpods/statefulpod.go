package statefulpods

import (
	iapetosapiv1 "github.com/q8s-io/iapetos/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

func NewStatefulPodForCR(cluster *redisv1alpha1.DistributedRedisCluster, name, svcName string, labels map[string]string) (*iapetosapiv1.StatefulPod, error) {
	password := redisPassword(cluster)
	volumes := redisVolumes(cluster)
	//fmt.Println("-------------------------------")
	//for _, v := range volumes {
	//	fmt.Printf("%+v\n", v)
	//}
	namespace := cluster.Namespace
	spec := cluster.Spec
	size := spec.ClusterReplicas + 1
	//mode:=corev1.PersistentVolumeFilesystem
	sp := &iapetosapiv1.StatefulPod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     cluster.Spec.Annotations,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
		},
		Spec: iapetosapiv1.StatefulPodSpec{
			Size: &size,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			PodTemplate: corev1.PodSpec{
				ImagePullSecrets: cluster.Spec.ImagePullSecrets,
				Affinity:         getAffinity(cluster, labels),
				Tolerations:      spec.ToleRations,
				SecurityContext:  spec.SecurityContext,
				NodeSelector:     cluster.Spec.NodeSelector,
				Subdomain:        svcName,
				Containers: []corev1.Container{
					redisServerContainer(cluster, password),
				},
				Volumes: volumes,
			},
			ServiceTemplate: nil,
		},
	}
	// set pvc template
	if volumes[0].PersistentVolumeClaim != nil {
		sp.Spec.PVCTemplate = &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: cluster.Spec.Storage.Size,
				},
			},
			StorageClassName: &cluster.Spec.Storage.Class,
			//VolumeMode: &mode,
		}
	}
	index := GetIndex(name)
	if len(cluster.Spec.PVNames) > 0 && len(cluster.Spec.PVNames) > index {
		sp.Spec.PVNames = append(sp.Spec.PVNames, cluster.Spec.PVNames[index])
	}
	if spec.Storage.DeleteClaim {
		sp.Spec.PVRecyclePolicy = delete
	} else {
		sp.Spec.PVRecyclePolicy = retain
	}
	if spec.Monitor != nil {
		sp.Spec.PodTemplate.Containers = append(sp.Spec.PodTemplate.Containers, redisExporterContainer(cluster, password))
	}
	if cluster.IsRestoreFromBackup() && cluster.IsRestoreRunning() && cluster.Status.Restore.Backup != nil {
		initContainer, err := redisInitContainer(cluster, password)
		if err != nil {
			return nil, err
		}
		sp.Spec.PodTemplate.Containers = append(sp.Spec.PodTemplate.Containers, initContainer)
	}
	return sp, nil
}
