# Developer Memory：测试方案

[功能与验收目标](../features/memory.md) · [技术方案](../designs/memory.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 验收

- 真实自动提取、memory_manage 和本机控制记忆进入同一列表；正确展示加密正文、Core／Recall 和可验证来源。
- Unicode 字面搜索、层级／来源／状态／时间过滤、完整无重复分页；游标不能跨条件、所有者或隐私边界复用。
- 原始消息／Task 正文只沿该记忆的当前可见引用读取；来源工具只能关联真实的当前 Agent memory_manage 调用。
- 外部／未知／遗忘记录一致不可访问；被替代正文不可见。缺失密钥不泄露密文，搜索不把解密失败伪装成无结果。
- 输出前发生 Forget、替代或表示变动时拒绝旧快照；只读查询不持有 WAL 写锁。
- remember／forget 的 pending／applied／rejected 保持维护回执语义，不与当前有效记忆混合。
- Web 认证、作用域、无效参数、只读写请求拒绝、HTML 转义、中文、延迟脚本、无脚本和移动端行为；旧 Database 路由及导航已移除。
- desktop/mobile、浅色/深色的真实截图验证；Developer 一级／二级导航为 176px／168px，过滤栏为 176px；半屏详情、分栏调宽、来源导航、返回、前后条目、独立页保持可用。

## 入口与证据

```sh
go test ./internal/agent/sqlite ./internal/interaction/web -run 'MemoryInspection|MemoryRoutes|MemoryFailures|Chinese' -count=1
python3 tests/system/verify_channels.py --binary bin/steward --memory-only --evidence evidence/CHOOSE_FRESH_MEMORY_RUN/report.json
make test lint
```

Memory 独立场景也属于既有 channels 门禁。Harness 使用临时数据根、实际记忆管理命令与工具、当前加密机制和真实 Chromium，检查原始持久记录指纹，验证浏览前后只读、重启和遗忘。原始报告和截图保留在忽略的 evidence/ 中。已验证的确定性 UI／数据边界不代表真实模型的语义记忆质量或大规模查询性能。

共享列表几何断言覆盖 320／390／760／761／1024／1400px，检查导航留白、compact 密度、不同模块图标、窄屏溢出和折叠筛选与视图标签不重叠。组件状态 UT 与所属 Go 回归分别由既有 frontend／go 通道运行，真实 Memory 场景由 channels-web 通道运行；统一设计依据见 [design.md](../../design.md#数据列表页契约)。

增强抽屉场景等待 Memory 自身的初始化标记，再断言非模态详情；延迟 app.js 时验证标记不会提前出现，原生链接始终保留。不能把脚本加载前合法的完整详情回退误判成抽屉故障。
