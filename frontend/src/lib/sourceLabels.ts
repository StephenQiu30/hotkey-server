export const sourceTypeLabels: Readonly<Record<string, string>> = {
  x: "X / Twitter",
  bing_grounding: "Bing 网页搜索",
  google_agent_search: "Google 网页搜索",
  duckduckgo: "DuckDuckGo Instant Answer",
  hacker_news: "Hacker News",
  sogou: "搜狗授权搜索",
  bilibili: "Bilibili",
  weibo: "微博",
  rss: "RSS / Atom",
};

export const instantSearchSourceOptions = Object.entries(sourceTypeLabels).map(
  ([value, label]) => ({ value, label })
);

export function sourceTypeLabel(sourceType: string | undefined) {
  if (!sourceType) return "未知来源";
  return sourceTypeLabels[sourceType] ?? sourceType;
}
