# Ripplesight Web

架构见 [PROJECT](../PROJECT.md)，视觉与组件规范见 [编号设计](../docs/index.md#设计主题)，工程规范见 [AGENTS](../AGENTS.md)。

## 本地运行

环境变量统一写在仓库根目录的 `.env`。在 `frontend/` 下执行：

```bash
pnpm install
```

```bash
pnpm dev
```

打开 <http://127.0.0.1:8666/>。后端 API 需要同时运行在 8667 端口。

## 目录

| 路径                     | 内容                                                                           |
| ------------------------ | ------------------------------------------------------------------------------ |
| `src/app/`               | 路由；页面专属组件放在路由下的 `components/`，首页的放在 `src/app/components/` |
| `src/components/ui/`     | shadcn/Radix 基础组件，以及语义排版、表单、媒体组件（`content.tsx`）           |
| `src/components/<功能>/` | 至少被两个页面复用的组件                                                       |
| `src/layout/`            | 全站外壳：BasicLayout、侧栏、登录页脚、正文滚动、阅读布局                      |
| `src/api/`               | 由 OpenAPI 生成的客户端，不要手改                                              |
| `src/request.ts`         | 唯一的 HTTP 传输层                                                             |
| `src/proxy.ts`           | 会话门禁与 CSP                                                                 |
| `tests/`                 | 全部前端测试                                                                   |

## 生成 API 客户端

后端运行后执行：

```bash
pnpm openapi:generate
```

默认读取 `http://127.0.0.1:8667/openapi.json`，可以用 `HOTKEY_OPENAPI_URL` 改成其他地址。生成器只从 200/201 响应中取返回模型，配置里会把只有 202 的响应在内存中映射过去，这不改变真实的 HTTP 语义。

## 检查

```bash
pnpm lint
```

```bash
pnpm typecheck
```

```bash
pnpm format:check
```

```bash
pnpm test
```

```bash
pnpm build
```

修改接口后，还要执行 `pnpm openapi:check`，确认生成结果没有漂移。

## 根元素 hydration 报错

浏览器翻译扩展可能在 React 接管前给 `<html>` 注入 `data-immersive-translate-page-theme`。如果报错差异仅有该根属性，而同一URL的服务端HTML没有该属性，原因是扩展修改了DOM，不是数据接口失效。根布局仅在 DocumentRoot 使用 `suppressHydrationWarning`；这是单层例外，正文差异仍必须报错。不要全局过滤console、对body/全部组件加抑制，或关闭SSR来隐藏真实问题。参见 [Next.js说明](https://nextjs.org/docs/messages/react-hydration-error)，回归在 `tests/app/layout.test.tsx`。

当前页面需要的字段、读取数量和存储取舍见[页面需求](../docs/index.md#需求目录)。新增接口/表应先证明当前具体控件无法用既有数据满足。
