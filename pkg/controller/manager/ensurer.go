package manager

import (
	"strings"

	"github.com/go-logr/logr"
	iapetosapiv1 "github.com/q8s-io/iapetos/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/configmaps"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/poddisruptionbudgets"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/services"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulpods"
)

type IEnsureResource interface {
	//EnsureRedisStatefulsets(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) (bool, error)
	EnsureRedisStatefulPods(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) (bool, error)
	EnsureRedisHeadLessSvcs(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	EnsureRedisSvc(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	EnsureRedisConfigMap(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	EnsureRedisRCloneSecret(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	//UpdateRedisStatefulsets(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	UpdateRedisStatefulPods(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
}

type realEnsureResource struct {
	statefulPodClient k8sutil.IStatefulPodControl
	//statefulSetClient k8sutil.IStatefulSetControl
	svcClient       k8sutil.IServiceControl
	configMapClient k8sutil.IConfigMapControl
	pdbClient       k8sutil.IPodDisruptionBudgetControl
	crClient        k8sutil.ICustomResource
	client          client.Client
	logger          logr.Logger
}

func NewEnsureResource(client client.Client, logger logr.Logger) IEnsureResource {
	return &realEnsureResource{
		statefulPodClient: k8sutil.NewStatefulPodController(client),
		//	statefulSetClient: k8sutil.NewStatefulSetController(client),
		svcClient:       k8sutil.NewServiceController(client),
		configMapClient: k8sutil.NewConfigMapController(client),
		pdbClient:       k8sutil.NewPodDisruptionBudgetController(client),
		crClient:        k8sutil.NewCRControl(client),
		client:          client,
		logger:          logger,
	}
}

func (r *realEnsureResource) EnsureRedisStatefulPods(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) (bool, error) {
	updated := false
	for i := 0; i < int((cluster.Spec.MasterSize)); i++ {
		name := statefulpods.GetStatefulPodName(cluster.Name, i)
		svcName := statefulpods.GetServiceName(cluster.Spec.ServiceName, i)
		labels[redisv1alpha1.StatefulPodLabel] = name
		if spUpdated, err := r.ensureRedisStatefulPod(cluster, name, svcName, labels); err != nil {
			return false, err
		} else if spUpdated {
			updated = spUpdated
		}
	}
	return updated, nil
}

/*func (r *realEnsureResource) EnsureRedisStatefulsets(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) (bool, error) {
	updated := false
	for i := 0; i < int(cluster.Spec.MasterSize); i++ {
	//	name := statefulsets.ClusterStatefulSetName(cluster.Name, i)
	//	svcName := statefulsets.ClusterHeadlessSvcName(cluster.Spec.ServiceName, i)
		name := statefulpods.GetStatefulPodName(cluster.Name,i)
		svcName:=statefulpods.GetServiceName(cluster.Spec.ServiceName,i)
		// assign label
		labels[redisv1alpha1.StatefulPodLabel] = name
		if stsUpdated, err := r.ensureRedisStatefulset(cluster, name, svcName, labels); err != nil {
			return false, err
		} else if stsUpdated {
			updated = stsUpdated
		}
	}
	return updated, nil
}*/

func (r *realEnsureResource) ensureRedisStatefulPod(cluster *redisv1alpha1.DistributedRedisCluster, spName, svcName string, labels map[string]string) (bool, error) {
	if err := r.ensureRedisPDB(cluster, spName, labels); err != nil {
		return false, err
	}
	sp, err := r.statefulPodClient.GetStatefulPod(cluster.Namespace, spName)
	if err == nil { // update
		if shouldUpdateRedis2(cluster, sp) {
			r.logger.WithValues("StatefulPod.Namespace", cluster.Namespace, "StatefulPod.Name", spName).
				Info("updating statefulPod")
			newSP, err := statefulpods.NewStatefulPodForCR(cluster, spName, svcName, labels)
			sp.Spec = *newSP.Spec.DeepCopy()
			if err != nil {
				return false, err
			}
			//newSP.ResourceVersion=sp.ResourceVersion
			return true, r.statefulPodClient.UpdateStatefulPod(sp)
		}
	} else if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("StatefulPod.Namespace", cluster.Namespace, "StatefulPod.Name", spName).
			Info("creating a new statefulPod")
		newSP, err := statefulpods.NewStatefulPodForCR(cluster, spName, svcName, labels)
		if err != nil {
			return false, err
		}
		return false, r.statefulPodClient.CreateStatefulPod(newSP)
	}
	return false, err
}

/*func (r *realEnsureResource) ensureRedisStatefulset(cluster *redisv1alpha1.DistributedRedisCluster, ssName, svcName string,
	labels map[string]string) (bool, error) {
	if err := r.ensureRedisPDB(cluster, ssName, labels); err != nil {
		return false, err
	}

	ss, err := r.statefulSetClient.GetStatefulSet(cluster.Namespace, ssName)
	if err == nil {
		if shouldUpdateRedis(cluster, ss) {
			r.logger.WithValues("StatefulSet.Namespace", cluster.Namespace, "StatefulSet.Name", ssName).
				Info("updating statefulSet")
			newSS, err := statefulsets.NewStatefulSetForCR(cluster, ssName, svcName, labels)
			if err != nil {
				return false, err
			}
			return true, r.statefulSetClient.UpdateStatefulSet(newSS)
		}
	} else if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("StatefulSet.Namespace", cluster.Namespace, "StatefulSet.Name", ssName).
			Info("creating a new statefulSet")
		newSS, err := statefulsets.NewStatefulSetForCR(cluster, ssName, svcName, labels)
		if err != nil {
			return false, err
		}
		return false, r.statefulSetClient.CreateStatefulSet(newSS)
	}
	return false, err
}*/

func shouldUpdateRedis2(cluster *redisv1alpha1.DistributedRedisCluster, sp *iapetosapiv1.StatefulPod) bool {
	if (cluster.Spec.ClusterReplicas + 1) != *sp.Spec.Size {
		return true
	}
	if cluster.Spec.Image != sp.Spec.PodTemplate.Containers[0].Image {
		return true
	}

	if statefulpods.IsPasswordChanged(cluster, sp) {
		return true
	}

	expectResource := cluster.Spec.Resources
	currentResource := sp.Spec.PodTemplate.Containers[0].Resources
	if result := expectResource.Requests.Memory().Cmp(*currentResource.Requests.Memory()); result != 0 {
		return true
	}
	if result := expectResource.Requests.Cpu().Cmp(*currentResource.Requests.Cpu()); result != 0 {
		return true
	}
	if result := expectResource.Limits.Memory().Cmp(*currentResource.Limits.Memory()); result != 0 {
		return true
	}
	if result := expectResource.Limits.Cpu().Cmp(*currentResource.Limits.Cpu()); result != 0 {
		return true
	}
	return monitorChanged2(cluster, sp)
}

func monitorChanged2(cluster *redisv1alpha1.DistributedRedisCluster, sp *iapetosapiv1.StatefulPod) bool {
	if cluster.Spec.Monitor != nil {
		for _, container := range sp.Spec.PodTemplate.Containers {
			if container.Name == statefulpods.ExporterContainerName {
				return false
			}
		}
		return true
	} else {
		for _, container := range sp.Spec.PodTemplate.Containers {
			if container.Name == statefulpods.ExporterContainerName {
				return true
			}
		}
		return false
	}
}

/*func shouldUpdateRedis(cluster *redisv1alpha1.DistributedRedisCluster, sts *appsv1.StatefulSet) bool {
	if (cluster.Spec.ClusterReplicas + 1) != *sts.Spec.Replicas {
		return true
	}
	if cluster.Spec.Image != sts.Spec.Template.Spec.Containers[0].Image {
		return true
	}

	if statefulsets.IsPasswordChanged(cluster, sts) {
		return true
	}

	expectResource := cluster.Spec.Resources
	currentResource := sts.Spec.Template.Spec.Containers[0].Resources
	if result := expectResource.Requests.Memory().Cmp(*currentResource.Requests.Memory()); result != 0 {
		return true
	}
	if result := expectResource.Requests.Cpu().Cmp(*currentResource.Requests.Cpu()); result != 0 {
		return true
	}
	if result := expectResource.Limits.Memory().Cmp(*currentResource.Limits.Memory()); result != 0 {
		return true
	}
	if result := expectResource.Limits.Cpu().Cmp(*currentResource.Limits.Cpu()); result != 0 {
		return true
	}
	return monitorChanged(cluster, sts)
}*/

func monitorChanged(cluster *redisv1alpha1.DistributedRedisCluster, sts *appsv1.StatefulSet) bool {
	if cluster.Spec.Monitor != nil {
		for _, container := range sts.Spec.Template.Spec.Containers {
			if container.Name == statefulpods.ExporterContainerName {
				return false
			}
		}
		return true
	} else {
		for _, container := range sts.Spec.Template.Spec.Containers {
			if container.Name == statefulpods.ExporterContainerName {
				return true
			}
		}
		return false
	}
}

func (r *realEnsureResource) ensureRedisPDB(cluster *redisv1alpha1.DistributedRedisCluster, name string, labels map[string]string) error {
	_, err := r.pdbClient.GetPodDisruptionBudget(cluster.Namespace, name)
	if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("PDB.Namespace", cluster.Namespace, "PDB.Name", name).
			Info("creating a new PodDisruptionBudget")
		pdb := poddisruptionbudgets.NewPodDisruptionBudgetForCR(cluster, name, labels)
		return r.pdbClient.CreatePodDisruptionBudget(pdb)
	}
	return err
}

func (r *realEnsureResource) EnsureRedisHeadLessSvcs(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	for i := 0; i < int(cluster.Spec.MasterSize); i++ {
		svcName := statefulpods.GetServiceName(cluster.Spec.ServiceName, i)
		name := statefulpods.GetStatefulPodName(cluster.Name, i)
		// assign label
		labels[redisv1alpha1.StatefulPodLabel] = name
		if err := r.ensureRedisHeadLessSvc(cluster, svcName, labels); err != nil {
			return err
		}
	}
	return nil
}

func (r *realEnsureResource) ensureRedisHeadLessSvc(cluster *redisv1alpha1.DistributedRedisCluster, name string, labels map[string]string) error {
	_, err := r.svcClient.GetService(cluster.Namespace, name)
	if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("Service.Namespace", cluster.Namespace, "Service.Name", cluster.Spec.ServiceName).
			Info("creating a new headless service")
		svc := services.NewHeadLessSvcForCR(cluster, name, labels)
		return r.svcClient.CreateService(svc)
	}
	return err
}

func (r *realEnsureResource) EnsureRedisSvc(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	name := cluster.Spec.ServiceName
	delete(labels, redisv1alpha1.StatefulPodLabel)
	_, err := r.svcClient.GetService(cluster.Namespace, name)
	if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("Service.Namespace", cluster.Namespace, "Service.Name", cluster.Spec.ServiceName).
			Info("creating a new service")
		svc := services.NewSvcForCR(cluster, name, labels)
		return r.svcClient.CreateService(svc)
	}
	return err
}

func (r *realEnsureResource) EnsureRedisConfigMap(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	cmName := configmaps.RedisConfigMapName(cluster.Name)
	drcCm, err := r.configMapClient.GetConfigMap(cluster.Namespace, cmName)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.WithValues("ConfigMap.Namespace", cluster.Namespace, "ConfigMap.Name", cmName).
				Info("creating a new configMap")
			cm := configmaps.NewConfigMapForCR(cluster, labels)
			if err2 := r.configMapClient.CreateConfigMap(cm); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	} else {
		if isRedisConfChanged(drcCm.Data[configmaps.RedisConfKey], cluster.Spec.Config, r.logger) {
			cm := configmaps.NewConfigMapForCR(cluster, labels)
			if err2 := r.configMapClient.UpdateConfigMap(cm); err2 != nil {
				return err2
			}
		}
	}

	if cluster.IsRestoreFromBackup() {
		restoreCmName := configmaps.RestoreConfigMapName(cluster.Name)
		restoreCm, err := r.configMapClient.GetConfigMap(cluster.Namespace, restoreCmName)
		if err != nil {
			if errors.IsNotFound(err) {
				r.logger.WithValues("ConfigMap.Namespace", cluster.Namespace, "ConfigMap.Name", restoreCmName).
					Info("creating a new restore configMap")
				cm := configmaps.NewConfigMapForRestore(cluster, labels)
				return r.configMapClient.CreateConfigMap(cm)
			}
			return err
		}
		if cluster.Status.Restore.Phase == redisv1alpha1.RestorePhaseRestart && restoreCm.Data[configmaps.RestoreSucceeded] == "0" {
			cm := configmaps.NewConfigMapForRestore(cluster, labels)
			return r.configMapClient.UpdateConfigMap(cm)
		}
	}
	return nil
}

func (r *realEnsureResource) EnsureRedisRCloneSecret(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	if !cluster.IsRestoreFromBackup() || cluster.IsRestored() {
		return nil
	}
	backup := cluster.Status.Restore.Backup
	secret, err := osm.NewRcloneSecret(r.client, backup.RCloneSecretName(), cluster.Namespace, backup.Spec.Backend, redisv1alpha1.DefaultOwnerReferences(cluster))
	if err != nil {
		return err
	}
	if err := k8sutil.CreateSecret(r.client, secret, r.logger); err != nil {
		return err
	}
	return nil
}

func isRedisConfChanged(confInCm string, currentConf map[string]string, log logr.Logger) bool {
	lines := strings.Split(strings.TrimSuffix(confInCm, "\n"), "\n")
	if len(lines) != len(currentConf) {
		return true
	}
	for _, line := range lines {
		line = strings.TrimSuffix(line, " ")
		confLine := strings.SplitN(line, " ", 2)
		if len(confLine) == 2 {
			if valueInCurrentConf, ok := currentConf[confLine[0]]; !ok {
				return true
			} else {
				if valueInCurrentConf != confLine[1] {
					return true
				}
			}
		} else {
			log.Info("custom config is invalid", "raw", line, "split", confLine)
		}
	}
	return false
}

func (r *realEnsureResource) UpdateRedisStatefulPods(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	for i := 0; i < int(cluster.Spec.MasterSize); i++ {
		name := statefulpods.GetStatefulPodName(cluster.Name, i)
		svcName := statefulpods.GetServiceName(cluster.Spec.ServiceName, i)
		// assign label
		labels[redisv1alpha1.StatefulPodLabel] = name
		if err := r.updateRedisStatefulPod(cluster, name, svcName, labels); err != nil {
			return err
		}
	}
	return nil
}

func (r *realEnsureResource) updateRedisStatefulPod(cluster *redisv1alpha1.DistributedRedisCluster, spName, svcName string,
	labels map[string]string) error {
	r.logger.WithValues("StatefulPod.Namespace", cluster.Namespace, "StatefulPod.Name", spName).
		Info("updating statefulPod immediately")
	newSS, err := statefulpods.NewStatefulPodForCR(cluster, spName, svcName, labels)
	if err != nil {
		return err
	}
	return r.statefulPodClient.UpdateStatefulPod(newSS)
}

/*func (r *realEnsureResource) UpdateRedisStatefulsets(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	for i := 0; i < int(cluster.Spec.MasterSize); i++ {
		name := statefulsets.ClusterStatefulSetName(cluster.Name, i)
		svcName := statefulsets.ClusterHeadlessSvcName(cluster.Spec.ServiceName, i)
		// assign label
		labels[redisv1alpha1.StatefulSetLabel] = name
		if err := r.updateRedisStatefulset(cluster, name, svcName, labels); err != nil {
			return err
		}
	}
	return nil
}*/

/*func (r *realEnsureResource) updateRedisStatefulset(cluster *redisv1alpha1.DistributedRedisCluster, ssName, svcName string,
	labels map[string]string) error {
	r.logger.WithValues("StatefulSet.Namespace", cluster.Namespace, "StatefulSet.Name", ssName).
		Info("updating statefulSet immediately")
	newSS, err := statefulsets.NewStatefulSetForCR(cluster, ssName, svcName, labels)
	if err != nil {
		return err
	}
	return r.statefulSetClient.UpdateStatefulSet(newSS)
}*/
