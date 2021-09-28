package statefulpods

const (
	redisStorageVolumeName      = "redis-data"
	redisRestoreLocalVolumeName = "redis-local"
	redisServerName             = "redis"
	hostnameTopologyKey         = "kubernetes.io/hostname"
	ExporterContainerName       = "exporter"

	graceTime = 30

	configMapVolumeName = "conf"
	retain              = "Retain"
	delete              = "Delete"
	passwordKey         = "password"
)
