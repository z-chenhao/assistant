来源：ERII-250 [PR #72](https://github.com/z-chenhao/erii/pull/72)，组合源码 `561421ab8fd96a4e6a44ba2c1efe95398b057451`。这是固定 security 分片的有限确定性维护前置，按注册 Coordinator 政策准入，不请求新的 Owner Block，不降低 ERII-250 原验收。提案保持 Backlog、无 assignee，等待原生 Workflow 准入；不手动启动另一个 Builder。

## 已观察到的容量缺口

ERII-375 已 Done 并合入 main，ERII-250 正常合并后注册实际 GC-04 库存，修正组合回归并优先启动已测量较长的 GC-03/04；所有门禁、十分钟和每 job 两 lane 不变。

[Ubuntu run 40257156127 attempt 1](https://github.com/z-chenhao/erii/actions/runs/40257156127)、Security(cases) job `121361354367` 再次超过十分钟。GitHub annotation 明确记录 job 超时；4 CPU AMD EPYC 7763、Ubuntu 24。verification 542.255 秒、exit 130；取消清理 1.805 秒，`owned_group_drained=true`。

- GC-03 完整 gate：295.76 秒、exit 0。
- GC-04 完整 gate：370.12 秒、exit 0。
- 随后启动的 GC-05 运行 246.32 秒后被取消，exit 130，不能计为通过。
- protection 五轮系列通过，job 5 分钟；其余 29 个 job 成功，Complete security evidence 与 Ubuntu checks 随 cases 不通过而失败。
- 同 SHA 本地 build、lint、658 Python 回归通过；GC-03/04 各 30 份原生收据通过，205.77/225.89 秒，GC-04 复算和零 Judge preview 通过。仅证明本地/当前观察，不能替代完整 Ubuntu CI，也不证明稳定提速。

原始日志、runner/源码/二进制/命令和收据留在注册 evidence_root：`ERII-250/561421ab8fd96a4e6a44ba2c1efe95398b057451/combined-5aea8321d59f/`。当前消费者没有已知 GC-04 断言失败；阻塞为三个长 Case 不能在现有两 lane cases job 中完成。

## 最小有限机制

将固定 `SECURITY_PARTS` 从两部分调整为三部分：

- `finance`：仅完整 GC-03 gate。
- `cases`：完整 GC-04（注册时）与 GC-05 gate，两条 lane，保留每 Case 原始变体并发和六变体各五轮。
- `protection`：memory-maintenance、recall、tools、artifacts，仍为每组原五轮串行、最多两条 lane。

保持现有 fail-closed join，恢复当前 `SHARDS['security']` 规范顺序、每个 gate 的原命令/参数及全部轮次；三部分与 join 全部进入 required Ubuntu checks。拒绝 missing、extra、duplicate、reordered、failed、cancelled、focused、dirty、身份漂移、未清理和不正确 run/attempt 的结果。不得缩减语义/机械门禁或把取消当通过。

新增一个 Ubuntu execution job、一次 build/setup，join 原数不变。单 job 十分钟、两 lane、runner class/资源规格、Case deadline/变体并发、断言、轮次、cleanup 以及任何已配置成本上限保持原值；记录这个有限资源增量。没有产品、协议、权限、存储、Durable Replay 或发布阈值变化，按当前 agent-delivery 的 routine deterministic maintenance 政策处理，不请求人工 CI 预算决定。

仅修改固定 part/matrix、严格 join 的库存选择、必需 aggregate/ci_compare 期望、对应拒绝回归和 FEATURES/CI 文档，以及受影响审计引用身份。一个独立有限 PR，不导入消费者业务源码、不扩展为动态分片或缓存/性能平台。

## 两阶段验收及停止条件

1. 本项基于 current main 证明机制和当前完整库存（GC-03/05 + 原保护系列）；合成已声明 GC-04 库存证明未来消费者的分片/顺序/拒绝边界。不得要求等待本项的未合并消费者源码先进入本项，以免形成循环依赖。真实 subprocess 回归证明 finance 与 cases 的成员、最多两 lane、原五轮串行、完整 join 与取消清理。所有 required exact-SHA Ubuntu CI 通过，再经独立 acceptance Review/helper 合并。
2. ERII-250 继续 PR #72/原分支，正常 merge current main、产生新 SHA，独立证明实际 GC-03/04/05 和保护系列的完整 Ubuntu 组合、全部 required CI、新的独立 acceptance Review。修复机制 PASS 不替代消费者或发布验收。
3. 保留同环境时长、输入、runner、source/binary SHA、run/attempt 和失败/清理证据，不从单次通过推导稳定加速。不提高限制、不减少轮次、不跳过任何 Case。若仍不足，记录实际测量并保留源码/证据，走必要的下一项有限确定性维护，不做盲目重跑或不受限扩张。
4. `semantic_score=null`、`release_ready=false`。模型、账号/VM/当前 L2 与本项无关，仍由 ERII-223 负责。
