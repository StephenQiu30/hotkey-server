export default function LoginPage() {
  return (
    <main>
      <h1>登录</h1>
      <form>
        <div>
          <label htmlFor="email">邮箱</label>
          <input id="email" type="email" placeholder="请输入邮箱" />
        </div>
        <div>
          <label htmlFor="password">密码</label>
          <input id="password" type="password" placeholder="请输入密码" />
        </div>
        <button type="submit">登录</button>
      </form>
    </main>
  );
}
