import { useState, type FormEvent } from "react";
import { login } from "../../api/identity";
import { message } from "../../request";

export function Login({ onLogin }: { onLogin: () => void }) {
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    setPending(true);
    setError("");
    try {
      await login({
        username: String(values.get("username")),
        password: String(values.get("password")),
      });
      onLogin();
    } catch (error) {
      setError(message(error));
    } finally {
      setPending(false);
    }
  }
  return (
    <main className="login">
      <p className="eyebrow">HOTKEY / PERSONAL LAB</p>
      <h1>让关注有迹可循。</h1>
      <p className="muted">登录你的热点监控工作台。</p>
      <form onSubmit={submit} className="panel">
        <label>
          用户名
          <input
            name="username"
            autoComplete="username"
            required
            minLength={3}
            maxLength={50}
          />
        </label>
        <label>
          密码
          <input
            name="password"
            type="password"
            autoComplete="current-password"
            required
            minLength={12}
            maxLength={128}
          />
        </label>
        {error && <p role="alert">{error}</p>}
        <button disabled={pending}>
          {pending ? "正在登录…" : "登录工作台"}
        </button>
      </form>
      <p className="muted small">
        首次使用请由本机所有者按照运行文档初始化账号。
      </p>
    </main>
  );
}
