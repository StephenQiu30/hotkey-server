export default function AdminConnectorsPage() {
  return (
    <main>
      <h1>连接器状态</h1>
      <table>
        <thead>
          <tr>
            <th>连接器</th>
            <th>状态</th>
            <th>最后同步</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>X (Twitter)</td>
            <td>healthy</td>
            <td>2026-06-12 23:00</td>
          </tr>
        </tbody>
      </table>
    </main>
  );
}
