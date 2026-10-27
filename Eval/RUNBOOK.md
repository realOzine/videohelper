# 固定语料与历史结果的独立工具

当前分支是 SQLite + sqlite-vec 的个人桌面 App。这里保留固定语料、历史结果与离线工具；原 PostgreSQL/Redis/S3 索引、检索桥接和快照 runner 已移除。本次桌面迁移没有改动素材、视频、标签、哈希、复核图、缺陷 fixtures、基线或实验报告。

旧服务器基线不能证明桌面版检索质量。桌面版还需独立完成真实模型索引、同语料检索、逐题评分和重复性验证；小型 fake ML 接线测试只验证流程。

## 核对语料与离线测试

在仓库根目录执行。校验哈希不依赖服务或 Python 环境：

```sh
(cd Eval && shasum -a 256 -c checksums.sha256)
```

离线工具使用 Python 3.11+；复用已安装依赖的 `ml/.venv`，或在 `Eval/.venv` 安装 `Eval/requirements.txt`，然后将下文解释器路径替换为相应环境：

```sh
ml/.venv/bin/python -m unittest discover -s Eval/tools -p 'test_*.py' -v
```

这会检查评分、历史报告比较、参数选择与标注结构。`score.py` 和 `ablate.py` 不启动模型、数据库或后台队列。

保留的 ASR 缺陷回归需要安装了项目及本地 ML 插件的开发环境，无需模型推理或旧服务器资源：

```sh
PYTHONPATH=ml/plugins/funasr ml/.venv/bin/python -m pytest Eval/defects/asr-timestamps/test_repro.py
```

`tools/diagnose_asr.py` 现在只保留该回归使用的纯 `violations` helper；历史数据库和 S3 抓取入口已移除。`defects/` 中的说明、原始时间戳和测试输入保持原样，其旧抓取命令应在历史版本查看。

## 媒体复核

```sh
ml/.venv/bin/python Eval/tools/serve.py
```

打开 `http://127.0.0.1:18766/review/index.html`，完成后 Ctrl-C。预览只绑定 localhost，支持 HTTP Range。

完整媒体重验需要 PATH 中的 FFmpeg/FFprobe 和 NumPy：

```sh
ml/.venv/bin/python Eval/tools/validate.py
```

它会检查文件哈希、完整解码、流、帧数、时长、音频信号及标注结构，并重新写入 `review/validation.json`。若要保留当前冻结报告，先在独立副本运行。结构与信号通过不等于独立人工金标或真实模型检索质量通过。

`tools/build.py` 是合成语料的历史生成器，检测到现有 manifest 会拒绝覆盖；正常回归不重建视频。

## 比较已有结果

标准库评分器可以比较已经保存的两个报告：

```sh
python3 Eval/tools/score.py compare --help
python3 Eval/tools/ablate.py summarize --help
```

接口和指标定义见 [CONTRACT.md](CONTRACT.md)。消融工具读取预先声明的计划和已保存报告；不执行检索，不改变模型、标签或选择完成后的核验结论。旧实验中的运行命令是当时服务器版本的操作记录，详见 [实验报告](experiments/ablation-v1/REPORT.md)。

## 复现旧服务器版本

在独立工作树检出目标报告中的 `code_revision` 或 `bridge_build_provenance.build_revision`，再使用该版本的 `Eval/RUNBOOK.md`、依赖与配置。不要在当前桌面工作树恢复旧数据库、对象桶或队列，也不要向桌面 SQLite 导入旧 PG 快照。

原始 baseline-v2 报告可只读查看：

```sh
git show 11a6668:Eval/runs/baseline-v2-a/report.md
git show 11a6668:Eval/snapshots/baseline-v2/snapshot.json
```

这些是历史证据；当时的运行资源和快照已经清理，当前文件不承诺仍可恢复相同索引。当前分支的代码删除没有执行任何云资源、数据库、对象存储或用户媒体删除。
