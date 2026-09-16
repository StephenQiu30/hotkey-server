import { useState } from "react";
import {
  createEventTrendAlertRule,
  evaluateEventTrendAlerts,
  getEventTrends,
  listEventTrendAlertRules,
  updateEventTrendAlertRule,
} from "../../api/events";
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

const sourceLabels: Record<API.TrendAlertRuleInput["source"], string> = {
  x: "X",
  bilibili: "B站",
  weibo: "微博",
  xiaohongshu: "小红书",
  douyin: "抖音",
  bluesky: "Bluesky",
};

const metricLabels: Record<API.TrendAlertRuleInput["metric"], string> = {
  new_posts: "新增根帖",
  new_discussions: "新增评论/回复",
  observed_reply_delta: "已观察回复增量",
};

function windowParams() {
  const until = new Date();
  const since = new Date(until.getTime() - 7 * 24 * 60 * 60 * 1000);
  return { since: since.toISOString(), until: until.toISOString() };
}

export function EventTrends({
  eventId,
  active,
  onNotificationsChanged,
}: {
  eventId: string;
  active: boolean;
  onNotificationsChanged: () => Promise<void>;
}) {
  const [trend, setTrend] = useState<API.EventTrendView | null>(null);
  const [rules, setRules] = useState<API.TrendAlertRuleView[]>([]);
  const [source, setSource] =
    useState<API.TrendAlertRuleInput["source"]>("bilibili");
  const [metric, setMetric] =
    useState<API.TrendAlertRuleInput["metric"]>("new_posts");
  const [bucketHours, setBucketHours] =
    useState<API.TrendAlertRuleInput["bucket_hours"]>(24);
  const [threshold, setThreshold] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [nextTrend, nextRules] = await Promise.all([
        getEventTrends({
          identity: eventId,
          ...windowParams(),
          bucket_hours: 24,
        }),
        listEventTrendAlertRules({ identity: eventId }),
      ]);
      setTrend(nextTrend);
      setRules(nextRules);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setLoading(false);
    }
  }

  async function createRule() {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      await createEventTrendAlertRule(
        { identity: eventId },
        {
          source,
          metric,
          bucket_hours: bucketHours,
          threshold_count: threshold,
        },
      );
      setNotice("趋势提醒规则已创建");
      setRules(await listEventTrendAlertRules({ identity: eventId }));
    } catch (cause) {
      setError(message(cause));
    } finally {
      setLoading(false);
    }
  }

  async function toggleRule(rule: API.TrendAlertRuleView) {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      await updateEventTrendAlertRule(
        { identity: eventId, rule_id: rule.id },
        {
          expected_version: rule.version,
          threshold_count: rule.threshold_count,
          enabled: !rule.enabled,
        },
      );
      setRules(await listEventTrendAlertRules({ identity: eventId }));
    } catch (cause) {
      setError(message(cause));
    } finally {
      setLoading(false);
    }
  }

  async function evaluate() {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const result = await evaluateEventTrendAlerts({ identity: eventId });
      setNotice(
        result.created_notifications > 0
          ? `新增 ${result.created_notifications} 条趋势提醒`
          : "最近可比较时间桶没有新增提醒",
      );
      await onNotificationsChanged();
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
      {notice && <p role="status">{notice}</p>}
      {trend.sources.length === 0 ? (
        <p className="muted">该事件还没有带原始证据的平台内容。</p>
      ) : (
        trend.sources.map((trendSource) => (
          <div className="trend-source" key={trendSource.source}>
            <h5>{sourceLabels[trendSource.source]}</h5>
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
                  {trendSource.buckets.map((bucket) => (
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

      <div className="trend-alerts">
        <div className="section-title">
          <div>
            <h5>趋势提醒</h5>
            <p className="muted small">只对结算后的可比较平台时间桶提醒。</p>
          </div>
          {active && rules.some((rule) => rule.enabled) && (
            <button
              className="secondary"
              disabled={loading}
              onClick={() => void evaluate()}
            >
              检查提醒
            </button>
          )}
        </div>
        {rules.map((rule) => (
          <div className="trend-alert-rule" key={rule.id}>
            <span>
              {sourceLabels[rule.source]} · {rule.bucket_hours} 小时 ·{" "}
              {metricLabels[rule.metric]} ≥ {rule.threshold_count} · v
              {rule.version}
            </span>
            <button
              className="secondary"
              disabled={loading || !active}
              onClick={() => void toggleRule(rule)}
            >
              {rule.enabled ? "停用" : "启用"}
            </button>
          </div>
        ))}
        {active && (
          <form
            className="compact-form trend-alert-form"
            onSubmit={(event) => {
              event.preventDefault();
              void createRule();
            }}
          >
            <label>
              平台
              <select
                value={source}
                onChange={(event) =>
                  setSource(
                    event.target.value as API.TrendAlertRuleInput["source"],
                  )
                }
              >
                {Object.entries(sourceLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              指标
              <select
                value={metric}
                onChange={(event) =>
                  setMetric(
                    event.target.value as API.TrendAlertRuleInput["metric"],
                  )
                }
              >
                {Object.entries(metricLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              时间桶
              <select
                value={bucketHours}
                onChange={(event) =>
                  setBucketHours(
                    Number(
                      event.target.value,
                    ) as API.TrendAlertRuleInput["bucket_hours"],
                  )
                }
              >
                <option value={1}>1 小时</option>
                <option value={6}>6 小时</option>
                <option value={24}>24 小时</option>
              </select>
            </label>
            <label>
              阈值
              <input
                aria-label="趋势提醒阈值"
                type="number"
                min={1}
                max={1000000}
                value={threshold}
                onChange={(event) => setThreshold(Number(event.target.value))}
              />
            </label>
            <button disabled={loading || threshold < 1}>创建提醒</button>
          </form>
        )}
      </div>
    </div>
  );
}
