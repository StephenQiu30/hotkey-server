# HotKey 文档中心

本目录只保留 Go 后端全面重构后的长期事实源文档。旧 FastAPI 实现、旧编号计划、中间验收包、静态 OpenAPI 产物和一次性审计记录不再保留在当前文档树中，避免污染后续实现上下文。

## 当前目录

- `product/prd/`：按产品能力维护 PRD，编号从 `1` 开始。
- `plans/`：按全局连续编号维护实现计划；一个 PRD 可以通过 `downstream` 拆分到多个 Plan。
- `engineering/`：长期架构与工程治理设计。
- `acceptance/`：后续存放可复测验收证据。
- `operations/`：后续存放部署、运行和回滚手册。
- `archive/`：后续只存放仍有长期价值的归档说明。

## 编号规则

- `1-13`：P0 开源核心闭环。
- `14-16`：P1 平台化能力。
- `17-19`：P2 商业化与规模化能力。
- `20-22`：P3 高级实时与事件图谱。
- `23-25`：n8n 外部自动化编排、热点内容采集和 AI 日报邮件工作流。
- `26`：系统端到端可运行与基础设施对接（后端接 PG/Redis、Web 接真实 API、Docker 部署）。

Plan 编号独立连续。`1-22` 保持既有 PRD 与 Plan 同号结构；`23+` 允许一个 PRD 通过 frontmatter `downstream` 指向多个后续 Plan，用于进一步拆解执行内容。

## 里程碑规则

- 一个 Epic 对应一个里程碑，GitHub 与 Linear 必须保持同名或等价命名。
- Epic issue 和其下属任务 issue 必须分配到同一个里程碑。
- P0、P1、P2、P3 里程碑关闭前，必须确认对应 PRD、Plan、实现、测试、验收和发布说明已经闭环。
- 里程碑推进过程中的临时记录、过程清单和一次性排查材料不进入 `docs/`。

每个 PRD 必须有：

```text
docs/product/prd/N-能力名称PRD.md
```

每个 Plan 必须有：

```text
docs/plans/N-能力名称实现计划.md
```

PRD 与 Plan 的关联以 PRD frontmatter `downstream` 为事实源。
