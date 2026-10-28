# 安装、启动与部署：测试方案

[功能说明](../features/installation.md) · [技术方案](../designs/installation.md) · [产品测试方案](product.md)

## 验收场景

F-037：从真实安装入口完成源码/原生包安装、其他工作目录启动、包含空格的安装路径、首条对话和工具调用、退出、SIGKILL 后重启、重装与数据保留。检查无模型时保持设置状态，显式配置不会被标准启动默认值覆盖。

## 已有入口

[startup 系统测试](../../tests/system/verify_startup.py) 和 [启动组件测试](../../internal/startup/local_test.go)。本次只迁移文档，未重跑应用功能验证。
