package manager

import (
	"fmt"

	iapetosapiv1 "github.com/q8s-io/iapetos/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulpods"
)

type ICheck interface {
	CheckRedisNodeNum(*redisv1alpha1.DistributedRedisCluster) error
	//CheckRedisMasterNum(*redisv1alpha1.DistributedRedisCluster) error
}

type realCheck struct {
	//statefulSetClient k8sutil.IStatefulSetControl
	statefulPodClient k8sutil.IStatefulPodControl
}

func NewCheck(client client.Client) ICheck {
	return &realCheck{
		statefulPodClient: k8sutil.NewStatefulPodController(client),
	}
}

func (c *realCheck) CheckRedisNodeNum(cluster *redisv1alpha1.DistributedRedisCluster) error {
	for i := 0; i < int(cluster.Spec.MasterSize); i++ {
		name := statefulpods.GetStatefulPodName(cluster.Name, i)
		expectNodeNum := cluster.Spec.ClusterReplicas + 1
		ss, err := c.statefulPodClient.GetStatefulPod(cluster.Namespace, name)
		if err != nil {
			return err
		}
		if err := c.checkRedisNodeNum(expectNodeNum, ss); err != nil {
			return err
		}
	}

	return nil
}

func (c *realCheck) checkRedisNodeNum(expectNodeNum int32, sp *iapetosapiv1.StatefulPod) error {
	if expectNodeNum != *sp.Spec.Size {
		return fmt.Errorf("number of redis pods is different from specification")
	}
	if expectNodeNum != int32(len(sp.Status.PodStatusMes)) {
		return fmt.Errorf("redis pods are not all ready")
	}
	/*if expectNodeNum != ss.Status.CurrentReplicas {
		return fmt.Errorf("redis pods need to be updated")
	}*/

	return nil
}

func (c *realCheck) CheckRedisMasterNum(cluster *redisv1alpha1.DistributedRedisCluster) error {
	if cluster.Spec.MasterSize != cluster.Status.NumberOfMaster {
		return fmt.Errorf("number of redis master different from specification")
	}
	return nil
}

//
//func (c *realCheck) CheckRedisClusterIsEmpty(cluster *redisv1alpha1.DistributedRedisCluster, admin redisutil.IAdmin) (bool, error) {
//
//}
