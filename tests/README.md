# 测试套件

所有测试源码均位于 `tests/`，与业务实现分离。

- `architecture/`：独立编译的架构、Schema 与 OpenAPI 契约门禁。
- `postgresfixture/`：可复用的 PostgreSQL 测试数据库 fixture。
- `_suite/`：按仓库目录镜像保存需要与被测 Go 包同编译的单元与集成测试。

执行完整测试使用 `make test`；执行某个业务包使用：

```bash
sh scripts/with-test-suite.sh test ./internal/modules/event/... -count=1
```

`with-test-suite.sh` 仅在命令运行期间创建未跟踪符号链接，结束时自动清理。不要将 `*_test.go` 放回 `internal/` 或其他业务目录。
