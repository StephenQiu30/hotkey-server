import { useState } from "react";
import { getEventTrends } from "../../api/events";
import { message } from "../../request";

const coverageLabels: Record<API.EventTrendBucket["coverage_status"], string> =
  {
    comparable: "可比较",
    interrupted: "覆盖中断",
    missing: "缺少覆盖",
  };

const reasonLabels: Record<
  API.EventTrendBucket["interruption_reasons"][number],
  string
> = {
  policy_changed: "采集策略变化",
  run_failed: "采集失败",
  run_partial: "仅部分采集",
  run_incomplete: "采集尚未完成",
  no_live_coverage: "没有实时采集",
};

function windowParams() {
  const until = new Date();
  const since = new Date(until.getTime() - 7 * 24 * 60 * 60 * 1000);
  return { since: since.toISOString(), until: until.toISOString() };
}

export function EventTrends({ eventId }: { eventId: string }) {
  const [trend, setTrend] = useState<API.EventTrendView | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      setTrend(
        await getEventTrends({
          identity: eventId,
          ...windowParams(),
          bucket_hours: 24,
        }),
      );
    } catch (cause) {
      setError(message(cause));
    } finally {
      setLoading(false);
    }
  }

  if (trend === null) {
    return (
      <div className="event-trends">
        <button
          className="secondary"
          disabled={loading}
          onClick={() => void load()}
        >
          {loading ? "读取趋势中…" : "查看近 7 天趋势"}
        </button>
        {error && <p role="alert">{error}</p>}
      </div>
    );
  }

  return (
    <div className="event-trends">
      <div className="section-title">
        <div>
          <h4>分平台趋势</h4>
          <p className="muted small">
            UTC 24 小时桶 · 回填已从新增量和互动增量中排除
          </p>
        </div>
        <button
          className="secondary"
          disabled={loading}
          onClick={() => void load()}
        >
          刷新趋势
        </button>
      </div>
      {error && <p role="alert">{error}</p>}
      {trend.sources.length === 0 ? (
        <p className="muted">该事件还没有带原始证据的平台内容。</p>
      ) : (
        trend.sources.map((source) => (
          <div className="trend-source" key={source.source}>
            <h5>{source.source}</h5>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>时间桶</th>
                    <th>根帖</th>
                    <th>评论/回复</th>
                    <th>回复增量</th>
                    <th>覆盖</th>
                    <th>排除回填</th>
                  </tr>
                </thead>
                <tbody>
                  {source.buckets.map((bucket) => (
                    <tr key={bucket.starts_at}>
                      <td>
                        {new Date(bucket.starts_at).toLocaleDateString("zh-CN")}
                      </td>
                      <td>{bucket.new_posts}</td>
                      <td>{bucket.new_discussions}</td>
                      <td>{bucket.observed_reply_delta}</td>
                      <td>
                        {coverageLabels[bucket.coverage_status]}
                        {bucket.interruption_reasons.length > 0 && (
                          <small className="trend-reasons">
                            {bucket.interruption_reasons
                              .map((reason) => reasonLabels[reason])
                              .join("、")}
                          </small>
                        )}
                      </td>
                      <td>
                        {bucket.excluded_backfill_items} 条内容 /{" "}
                        {bucket.excluded_backfill_observations} 次观察
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ))
      )}
    </div>
  );
}
