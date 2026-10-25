# baseline-v2 索引验收报告

结果：**通过**。新数据库 `vh_eval_baseline_v2`、新桶 `vh-eval-baseline-v2`；原失败基线保持不变。

| 指标 | baseline-v1 | baseline-v2 |
| --- | ---: | ---: |
| 上传视频 | 5 | 5 |
| ready 视频 | 3 | 5 |
| 必需阶段完成 | 29 | 35 |
| visual 向量 | 251 | 251 |
| speech 向量 | 45 | 83 |
| audio 向量 | 251 | 251 |

新轮次耗时 **337.48 秒**，含环境准备、上传、索引检查及归档；真实模型预先加载。旧轮次因可复现缺陷中止，不能将两轮耗时当性能改善比较。

| 视频 | ASR 尝试次数 | visual / speech / audio |
| --- | ---: | ---: |
| study_zh | 1 | 84 / 15 / 84 |
| blockchain_mix | 1 | 48 / 18 / 48 |
| story_en | 1 | 44 / 16 / 44 |
| vlog_zh | 1 | 39 / 22 / 39 |
| landscape_en | 1 | 36 / 12 / 36 |

所有网格、语音切片覆盖、向量有效性、HNSW、模型空间及索引阶段检查通过。源视频字节与冻结素材一致，归一化视频音画流和时长已检查；完整快照归档 20 个对象。

- [机器可读报告](report.json)、[原始索引检查](inspection.json)、[运行版本](software.json)
- [冻结快照清单](../../snapshots/baseline-v2/snapshot.json)、[历史失败报告](../baseline-v1-index/report.md)
- [独立检索与恢复评估](../../BASELINE.md)

本报告只验收索引，不将成功处理等同于检索质量达标。
