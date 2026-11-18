// 显示用的小工具。

const MAP_NAMES: Record<string, string> = {
  theisland: "孤岛",
  thecenter: "中心岛",
  scorchedearth: "焦土",
  ragnarok: "仙境",
  aberration: "畸变",
  extinction: "灭绝",
  valguero: "瓦尔盖罗",
  genesis: "创世纪",
  gen2: "创世纪 2",
  crystalisles: "水晶岛",
  lostisland: "失落岛",
  fjordur: "峡湾",
};

/** 地图显示名：TheIsland → 孤岛（TheIsland）；未知地图原样显示。 */
export function mapName(map: string): string {
  if (!map) return "未知地图";
  const key = map.toLowerCase().replace(/_p$/, "");
  const zh = MAP_NAMES[key];
  return zh ? `${zh}` : map;
}

export function rate(v: number): string {
  if (!v) return "-";
  return `${Number.isInteger(v) ? v : v.toFixed(1)}x`;
}

export function region(country: string, province: string): string {
  if (province && country === "CN") return province;
  return [country, province].filter(Boolean).join(" · ");
}

/** Unix 秒 → 「3 分钟前」。 */
export function ago(unix: number): string {
  if (!unix) return "";
  const s = Math.max(0, Date.now() / 1000 - unix);
  if (s < 60) return "刚刚";
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`;
  return `${Math.floor(s / 86400)} 天前`;
}

export function errText(e: unknown): string {
  if (e && typeof e === "object" && "message" in e) return String((e as { message: unknown }).message);
  return String(e);
}
