# 开发、验证与交付：测试方案

[功能说明](../features/engineering.md) · [技术方案](../designs/engineering.md) · [产品测试方案](product.md)

## 验收场景

F-038：固定依赖和构建入口可用；文档改动只运行适用检查；代码/未知变更运行完整 PR 路径；失败和取消保留失败状态与清理；原生打包不自动发布正式版本。

## 已有入口

[工程辅助回归](../../tests/harness/test_project_layout.py)、[Makefile](../../Makefile)、[PR CI](../../.github/workflows/ci.yml) 与 [扩展回归](../../.github/workflows/regression.yml)。运行方式及边界见产品测试方案。本次文档迁移不宣称这些流水线已重新执行。
