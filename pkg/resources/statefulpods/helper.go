package statefulpods

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	iapetosapiv1 "github.com/q8s-io/iapetos/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/configmaps"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

var log = logf.Log.WithName("resource_statefulset")

func getRedisCommand(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) []string {
	cmd := []string{
		"/conf/fix-ip.sh",
		"redis-server",
		"/conf/redis.conf",
		"--cluster-enabled yes",
		"--cluster-config-file /data/nodes.conf",
	}
	if password != nil {
		cmd = append(cmd, fmt.Sprintf("--requirepass '$(%s)'", redisv1alpha1.PasswordENV),
			fmt.Sprintf("--masterauth '$(%s)'", redisv1alpha1.PasswordENV))
	}

	renameCmdMap := utils.BuildCommandReplaceMapping(config.RedisConf().GetRenameCommandsFile(), log)
	mergedCmd := mergeRenameCmds(cluster.Spec.Command, renameCmdMap)

	if len(mergedCmd) > 0 {
		cmd = append(cmd, mergedCmd...)
	}

	return cmd
}

func mergeRenameCmds(userCmds []string, systemRenameCmdMap map[string]string) []string {
	cmds := make([]string, 0)
	for _, cmd := range userCmds {
		splitedCmd := strings.Fields(cmd)
		if len(splitedCmd) == 3 && strings.ToLower(splitedCmd[0]) == "--rename-command" {
			if _, ok := systemRenameCmdMap[splitedCmd[1]]; !ok {
				cmds = append(cmds, cmd)
			}
		} else {
			cmds = append(cmds, cmd)
		}
	}

	renameCmdSlice := make([]string, len(systemRenameCmdMap))
	i := 0
	for key, value := range systemRenameCmdMap {
		c := fmt.Sprintf("--rename-command %s %s", key, value)
		renameCmdSlice[i] = c
		i++
	}
	sort.Strings(renameCmdSlice)
	for _, renameCmd := range renameCmdSlice {
		cmds = append(cmds, renameCmd)
	}

	return cmds
}

func volumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      redisStorageVolumeName,
			MountPath: "/data",
		},
		{
			Name:      configMapVolumeName,
			MountPath: "/conf",
		},
	}
}

func customContainerEnv(env []corev1.EnvVar, customEnv []corev1.EnvVar) []corev1.EnvVar {
	env = append(env, customEnv...)
	return env
}

func redisServerContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) corev1.Container {
	probeArg := "redis-cli -h $(hostname) ping"

	container := corev1.Container{
		Name:            redisServerName,
		Image:           cluster.Spec.Image,
		ImagePullPolicy: cluster.Spec.ImagePullPolicy,
		SecurityContext: cluster.Spec.ContainerSecurityContext,
		Ports: []corev1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: 6379,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "gossip",
				ContainerPort: 16379,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts(),
		Command:      getRedisCommand(cluster, password),
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						probeArg,
					},
				},
			},
		},
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						probeArg,
					},
				},
			},
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		Resources: *cluster.Spec.Resources,
		// TODO store redis data when pod stop
		Lifecycle: &corev1.Lifecycle{
			PostStart: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "echo ${REDIS_PASSWORD} > /data/redis_password"},
				},
			},
			PreStop: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "/conf/shutdown.sh"},
				},
			},
		},
	}

	if password != nil {
		container.Env = append(container.Env, *password)
	}

	container.Env = customContainerEnv(container.Env, cluster.Spec.Env)
	container.Lifecycle = &corev1.Lifecycle{PreStop: &corev1.Handler{Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", "sed -i '/myself/!d' /data/nodes.conf"}}}}
	return container
}

func redisExporterContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) corev1.Container {
	container := corev1.Container{
		Name: ExporterContainerName,
		Args: append([]string{
			fmt.Sprintf("--web.listen-address=:%v", cluster.Spec.Monitor.Prometheus.Port),
			fmt.Sprintf("--web.telemetry-path=%v", redisv1alpha1.PrometheusExporterTelemetryPath),
		}, cluster.Spec.Monitor.Args...),
		Image:           cluster.Spec.Monitor.Image,
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-http",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: cluster.Spec.Monitor.Prometheus.Port,
			},
		},
		Env:             cluster.Spec.Monitor.Env,
		Resources:       cluster.Spec.Monitor.Resources,
		SecurityContext: cluster.Spec.Monitor.SecurityContext,
	}
	if password != nil {
		container.Env = append(container.Env, *password)
	}

	container.Env = customContainerEnv(container.Env, cluster.Spec.Env)

	return container
}

func redisInitContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) (corev1.Container, error) {
	backup := cluster.Status.Restore.Backup
	backupSpec := backup.Spec.Backend
	location, err := backupSpec.Location()
	if err != nil {
		return corev1.Container{}, err
	}
	folderName, err := backup.RemotePath()
	if err != nil {
		return corev1.Container{}, err
	}
	log.V(3).Info("restore", "namespaces", cluster.Namespace, "name", cluster.Name, "folderName", folderName)
	container := corev1.Container{
		Name:            redisv1alpha1.JobTypeRestore,
		Image:           backup.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Args: []string{
			redisv1alpha1.JobTypeRestore,
			fmt.Sprintf(`--data-dir=%s`, redisv1alpha1.BackupDumpDir),
			fmt.Sprintf(`--location=%s`, location),
			fmt.Sprintf(`--folder=%s`, folderName),
			fmt.Sprintf(`--snapshot=%s`, backup.Name),
			"--",
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "REDIS_RESTORE_SUCCEEDED",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configmaps.RestoreConfigMapName(cluster.Name),
						},
						Key: configmaps.RestoreSucceeded,
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      redisStorageVolumeName,
				MountPath: redisv1alpha1.BackupDumpDir,
			},
			{
				Name:      "rcloneconfig",
				ReadOnly:  true,
				MountPath: osm.SecretMountPath,
			},
		},
	}

	if backup.IsRefLocalPVC() {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      redisRestoreLocalVolumeName,
			MountPath: backup.Spec.Backend.Local.MountPath,
			SubPath:   backup.Spec.Backend.Local.SubPath,
			ReadOnly:  true,
		})
	}

	if password != nil {
		container.Env = append(container.Env, *password)
	}

	if backup.Spec.PodSpec != nil {
		container.Resources = backup.Spec.PodSpec.Resources
		container.LivenessProbe = backup.Spec.PodSpec.LivenessProbe
		container.ReadinessProbe = backup.Spec.PodSpec.ReadinessProbe
		container.Lifecycle = backup.Spec.PodSpec.Lifecycle
	}

	container.Env = customContainerEnv(container.Env, cluster.Spec.Env)

	return container, nil
}

func getAffinity(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *corev1.Affinity {
	affinity := cluster.Spec.Affinity
	if affinity != nil {
		return affinity
	}

	if cluster.Spec.RequiredAntiAffinity {
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							TopologyKey: hostnameTopologyKey,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{redisv1alpha1.LabelClusterName: cluster.Name},
							},
						},
					},
				},
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		}
	}
	// return a SOFT anti-affinity by default
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{redisv1alpha1.LabelClusterName: cluster.Name},
						},
					},
				},
			},
		},
	}
}

func redisPassword(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.EnvVar {
	if cluster.Spec.PasswordSecret == nil {
		return nil
	}
	secretName := cluster.Spec.PasswordSecret.Name

	return &corev1.EnvVar{
		Name: redisv1alpha1.PasswordENV,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: "password",
			},
		},
	}
}

func redisVolumes(cluster *redisv1alpha1.DistributedRedisCluster) []corev1.Volume {
	var volumes []corev1.Volume
	volumes = append(volumes, *redisDataVolume(cluster))
	volumes = append(volumes, *configMapVolume(cluster))
	if !cluster.IsRestoreFromBackup() || cluster.Status.Restore.Backup == nil || !cluster.IsRestoreRunning() {
		return volumes
	}
	volumes = append(volumes, corev1.Volume{
		Name: "rcloneconfig",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: cluster.Status.Restore.Backup.RCloneSecretName(),
			},
		},
	})
	if cluster.Status.Restore.Backup.IsRefLocalPVC() {
		volumes = append(volumes, corev1.Volume{
			Name:         redisRestoreLocalVolumeName,
			VolumeSource: cluster.Status.Restore.Backup.Spec.Local.VolumeSource,
		})
	}

	return volumes
}

func configMapVolume(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Volume {
	executeMode := int32(0755)
	return &corev1.Volume{
		Name: configMapVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configmaps.RedisConfigMapName(cluster.Name),
				},
				DefaultMode: &executeMode,
			},
		},
	}
}

func emptyVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: redisStorageVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func pvcVolume(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Volume {
	return &corev1.Volume{
		Name: redisStorageVolumeName,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: redisStorageVolumeName,
				//ReadOnly: true,
			},
		},
	}
}

func redisDataVolume(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Volume {
	if cluster.Spec.Storage == nil {
		return emptyVolume()
	}
	switch cluster.Spec.Storage.Type {
	case redisv1alpha1.Ephemeral:
		return emptyVolume()
	case redisv1alpha1.PersistentClaim:
		return pvcVolume(cluster)
	default:
		return emptyVolume()
	}
}

func GetStatefulPodName(clusterName string, i int) string {
	return fmt.Sprintf("drc-%s-%d", clusterName, i)
}

func GetIndex(name string) int {
	index := strings.Split(name, "-")
	i, _ := strconv.Atoi(index[len(index)-1])

	return i
}

func GetServiceName(name string, i int) string {
	return fmt.Sprintf("%s-%d", name, i)
}

func IsPasswordChanged(cluster *redisv1alpha1.DistributedRedisCluster, sp *iapetosapiv1.StatefulPod) bool {
	if cluster.Spec.PasswordSecret != nil {
		envSet := sp.Spec.PodTemplate.Containers[0].Env
		secretName := getSecretKeyRefByKey(redisv1alpha1.PasswordENV, envSet)
		if secretName == "" {
			return true
		}
		if secretName != cluster.Spec.PasswordSecret.Name {
			return true
		}
	}
	return false
}

func getSecretKeyRefByKey(key string, envSet []corev1.EnvVar) string {
	for _, value := range envSet {
		if key == value.Name {
			if value.ValueFrom != nil && value.ValueFrom.SecretKeyRef != nil {
				return value.ValueFrom.SecretKeyRef.Name
			}
		}
	}
	return ""
}

func GetClusterPassword(client client.Client, cluster *redisv1alpha1.DistributedRedisCluster) (string, error) {
	if cluster.Spec.PasswordSecret == nil {
		return "", nil
	}
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      cluster.Spec.PasswordSecret.Name,
		Namespace: cluster.Namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[passwordKey]), nil
}

func GetOldRedisClusterPassword(client client.Client, sp *iapetosapiv1.StatefulPod) (string, error) {
	envSet := sp.Spec.PodTemplate.Containers[0].Env
	secretName := getSecretKeyRefByKey(redisv1alpha1.PasswordENV, envSet)
	if secretName == "" {
		return "", nil
	}
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      secretName,
		Namespace: sp.Namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[passwordKey]), nil
}
