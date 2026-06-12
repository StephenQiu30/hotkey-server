import Link from "next/link";

export default function AdminPage() {
  return (
    <main>
      <h1>管理后台</h1>
      <nav>
        <ul>
          <li>
            <Link href="/admin/runs">任务运行</Link>
          </li>
          <li>
            <Link href="/admin/connectors">连接器状态</Link>
          </li>
        </ul>
      </nav>
    </main>
  );
}
