# redis-cluster

## redis-cluster 局限性

1. key 批量操作支持有限。（mget mset等只支持具有相同slot 值的key）
2. 事务支持有限 （若 key 被分配到同一节点上支持事务操作，否则不支持）
3. 只支持0号数据库
4. 复制结构只有一层，即不存在 A<-B<-C 这种模式，仅支持 A<-B ,A<-C。

```text
局限性 1，2 可以通过为 key 设置相同的 slot 来解决
```

## redis-cluster 改进

1. 基础组件有 statefulSet 改为 statefulPod
2. 数据备份由 RDB 机制改为 AOF
3. 删除集群式，若需要保留 pv(即集群数据),仅保留 master 的数据，slave 数据被清除。

## 部署注意事项

1. 若集群内 statefulpod 已被部署，若没有跟新，请重新部署。（建议重新发布 statefulpod-operator yaml 文件）
2. 请仔细产看 05-redis-cluster.yaml 文件中各个字段的含义。
3. 若第一次部署或者部署时不需要加载数据可以不指定 pvNames 字段
4. pv 数量请与 masterSize 数量保持一致，若不一致 masterSize 的数量与 pv 的数量保持一致。
5. 请确保 DistributedRedisCluster 的状态为 Healthy 后在进行使用。

## 测试注意事项

1. 请确保 slot 分配正确且合理
2. 部署时若加载旧数据，请查看数据是否成功加载
3. 扩缩容后删除集群，然后再部署集群加载旧数据时确保 slot 分配合理正确，以及集群正常启动。