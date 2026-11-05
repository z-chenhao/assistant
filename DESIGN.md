---
name: Steward
description: "Steward Web 的视觉与交互规范"
colors:
  light-background: "#f9f9f9"
  light-surface: "#fcfcfc"
  light-text: "#212121"
  light-muted-text: "#666666"
  light-divider: "#dfdfdf"
  light-control-border: "#858585"
  light-focus: "#767676"
  dark-background: "#212121"
  dark-surface: "#282828"
  dark-text: "#ededed"
  dark-muted-text: "#afafaf"
  dark-divider: "#3e3e3e"
  dark-control-border: "#777777"
  dark-focus: "#9f9f9f"
typography:
  page-title:
    fontFamily: "Inter, PingFang SC, Source Han Sans SC, Microsoft YaHei, sans-serif"
    fontSize: "1.375rem"
    fontWeight: 600
    lineHeight: 1.35
  section-title:
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
  ui:
    fontSize: "0.875rem"
    lineHeight: 1.6
  metadata:
    fontSize: "0.8125rem"
    lineHeight: 1.65
  reply:
    fontFamily: "Source Serif 4, Georgia, PingFang SC, Source Han Sans SC, Microsoft YaHei, sans-serif"
    fontSize: "16px"
    lineHeight: 1.75
rounded:
  button: "9px"
  surface: "12px"
spacing:
  workspace-gutter: "40px"
  compact-gutter: "20px"
  section: "20px"
  compact-section: "16px"
  action-gap: "10px"
---

# Steward Web 设计规范

## Overview

Steward 采用克制的黑白工作界面。Chat 给持续对话留出阅读空间。Tasks、Settings 和 Developer 优先让用户扫读状态、找到控件并完成操作。Ghost 是 Chat 的固定视觉锚点，悬停问候时头像和点击区域尺寸保持稳定。

## Colors

浅色与深色共用相同的语义。主操作使用高对比填充，普通操作使用透明底和细边框。状态同时使用文字，颜色提供辅助识别。内容分隔线、输入边界与键盘焦点各自使用对应的颜色。实际色值维护在 [共享样式](web/src/foundation.css)。

## Typography

操作页依次使用页面标题、区块标题、常规界面和辅助文字四档。在根字号为 16px 时分别为 **22、16、14 和 13px**。表单标签、状态、帮助文案应按角色选档。窄屏普通文本输入和下拉控件使用 16px。Chat 欢迎语和回复采用阅读排版。字标使用 Michroma，代码使用 JetBrains Mono。

## Layout

- 普通工作页共用内容列和左右留白。Trace 工作台为并排的树和详情使用更宽的容器。
- 面包屑负责上级导航。当前标签已经显示页面名称时，保留语义标题供辅助技术使用，视觉上只出现一次。实体名称与标识相同时只显示一次。
- 同组操作放进同一容器，对齐控件中心并使用统一间距。每个按钮即使各自包在表单里，也遵循同一行的对齐规则。带标签的筛选字段和提交按钮以控件底边对齐。
- 窄屏允许字段和操作按组换行，保持阅读顺序与触控空间。工作区页头保留直接上级，为状态文字留出空间。宽表格在自己的容器内滚动。

## Elevation & Depth

常规详情用留白和分隔线划分区块。独立边界用于审批、反馈和抽屉等有明确交互范围的内容。颜色层次和边线承担日常分组。

## Components

### 设置行与帮助

一项配置一行。字段名称、控件和保存操作保持同组，常规保存使用紧凑的次要按钮。可选解释放在字段旁，短提示可用鼠标悬停和键盘聚焦访问。影响选择的内容直接可见，例如提供方同意、重启要求和共享账户操作范围。

### 语言偏好

语言偏好在增强界面中确认不同值后自动保存，正常界面不显示 Save 或自动保存说明文案。保存中、成功及错误反馈靠近该设置。只有无 JavaScript 或增强初始化失败时显示原生保存按钮。

### 下拉选择器

- 收起时显示当前值和下拉箭头，使用细边框和稳定的控件宽度。展开时，面板贴近触发器，优先对齐右缘，宽度与触发器协调，并适应视口边缘。
- 选项逐行显示。当前选项用背景和右侧勾号共同标记，键盘焦点与已选状态可区分。较长的列表在面板内滚动，面板与焦点环要避开分组容器的裁剪。
- 支持键盘和触摸打开与选择。键盘可用方向键浏览、确认键选择、Escape 关闭，关闭后焦点回到触发器。辅助技术能读到控件名称、展开状态和当前选项。用脚本增强原生设置表单时，保留无脚本时可用的选择和保存路径。

### 行内帮助

短解释用名称旁的小问号。悬停、键盘聚焦和触摸都能查看，Escape 可以关闭。提示不抢焦点、不改变设置行高度。保存错误、不可逆后果或决定前必读的信息直接显示在设置旁。

### 状态与账户操作

标题旁展示必要状态。Resources 的模型列表默认收起配置，当前模型及连接状态在行内标明。展开一行即可查看连接标识。行尾保留连接和选用两个位置，未验证时显示「测试连接」，通过后显示「连接已验证」。候选模型的主操作「用于新工作」在选用后变为「当前模型」。切换会复用有效验证，缺少验证时先测试。「测试连接」使用次要按钮。已有有效验证的当前模型可从连接位置选择「恢复使用当前模型」。共享账户作为分组标题，模型直接列在对应账户下，直连模型独立分组。账户操作单独展开，重连和退出在同一操作行。登录进度归发起页面，首次渲染与实时更新采用同一规则。授权窗口成功打开后，原页面保留简短的等待状态和取消入口。弹窗被拦截时，原页面提供手动继续链接。

## Do's and Don'ts

- 复用共享字号、工作区间距、字段行和操作行。普通详情保持单一可见标题。
- 让成功反馈对应当前状态。保留需要用户理解或处理的错误和风险，合并重复说明。
- 用实际渲染核对同组按钮的垂直位置、筛选字段的底边、标题去重，以及 320px 下页头元素是否重叠。检查明暗主题与键盘焦点。已有回归入口见 [品牌测试方案](docs/test-plans/branding.md)。
