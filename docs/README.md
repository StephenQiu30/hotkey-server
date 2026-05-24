# docs

本目录仅承载 HotKey 后端长期文档，不收录过程性备忘。  
文档分层如下：

1. `product/`：PRD、范围与目标定义（只写决策）
2. `plans/`：执行计划、任务拆解、里程碑领取与领取规则
3. `engineering/`：技术方案、架构决策、验收与监控准则
4. `acceptance/`：验收标准、验证记录、回放报告
5. `research/`：研究结论与调研对比
6. `archive/`：历史归档与版本快照

## 写作约束（强制）

1. PRD、Plan、验收都需 `docs/TEMPLATE.md` 头部模板（YAML frontmatter）。
2. 题目与文件名必须能表达领域与序号（如 `24-热点事件判定与热度引擎PRD.md`）。
3. 文件中不能出现 `TBD/TODO/待补充` 占位。
4. 过程性工作不在 `docs/`，必须先放到 OpenSpec 的 `tasks`。
5. 变更必须可追溯：每篇文档要有 `inputs`、`outputs`、`triggers`、`downstream`。

## 本项目文档执行流

1. 先补 PRD（目标/边界/验收）  
2. 再写 Plan（文件任务拆分、TDD 清单、依赖、回滚）  
3. 按 Plan 拆 Issue，并挂 `superpowers-tdd` 与里程碑  
4. 每次任务执行须先补测试并在本地执行（至少覆盖相关测试，再进入代码变更与提交流程）  
5. PRD/Plan 完整后，才进入实现与 PR

## 里程碑与任务领取

- 里程碑文件：`docs/plans/28-里程碑与任务领取总控计划.md`
- 总控文件维护四象限状态：待办 / 进行中 / 完成 / 阻塞。
- 任何 Issue 超过 3 天无状态更新需要重启风险会审。

## 执行约束补充

- `server` 为后端服务入口，且不维护独立 `apps` 运行时目录；非归档 `apps` 文件不得作为运行时入口。
- 如存在历史遗留的 `apps` 运行时文件，可在本执行周期作为清理项直接删除；清理完成后在 PR 中说明扫描与删除结果。
- 每个执行单元（Issue/PR）须按 test-first 先后顺序执行并提交：
  - `test:`：先补充/更新相关测试并让测试先失败（红）
  - `impl:`：补齐实现使测试通过（绿）
  - `refactor:`/`chore:`：仅做非行为变更整理（可选）
- 每个执行单元完成前后都必须执行：
  - `python3 -m unittest discover -s tests -p 'test_repository_governance.py'`
  - `find . -type d -name apps -print`
  - `find . -type f | rg '(^|/)apps/'`
  - `git status --short`（执行前确认无未计划中间产物；测试后/提交前后都要求输出为空）

## 推荐阅读路径

- 新成员：先读 `docs/product/03-分阶段功能需求.md`、`docs/plans/10-AI热点监控MVP计划.md`
- 本次热点监测专项：先读 `docs/product/prd/24-热点事件判定与热度引擎PRD.md` 到 `docs/plans/27-接入源适配器改造与可替换实现计划.md`
- 验收复盘：对应查看 `docs/acceptance/README.md`
