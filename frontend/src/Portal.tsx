// 玩家站：服主的 Commander 开的只读网页（公告、服务器状态、ArkShop 商店），助手用自己的界面显示。
// 内容来自服主的机器，只当纯文本显示，链接要玩家点了才在浏览器里打开（只接受 http(s)）。
// 数据格式见 ArkPlayerPortal/docs/snapshot.md
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Clipboard } from "@wailsio/runtime";
import { PortalService } from "../bindings/arkfgf.com/link";
import { errText, mapName, rate } from "./format";

// ---- 快照的结构（只声明用到的字段） ----

type PortalData = {
  site: { name: string; description: string; qq_group: string; discord: string; links: { title: string; url: string }[] };
  announcements: { id: string; title: string; body: string; pinned: boolean; published_at: string }[];
  servers: PortalServer[];
  pages?: Page[];
};

// 通用页面（规则、指南、服务器设置）：只认 heading、paragraph、list、kv 四种块，不认识的跳过
type Text = string | { zh?: string; en?: string };
type Block = { type: string; text?: Text; items?: unknown[] };
type Page = { id: string; kind: string; title: Text; server_key?: string; blocks: Block[] };

type MeData = {
  user: { steam_id: string; name: string } | null;
  link_login?: boolean;
  data?: Record<string, { points: number; total_spent: number; kits?: Record<string, number> }>;
};

function zhText(t: unknown): string {
  if (typeof t === "string") return t;
  if (t && typeof t === "object") {
    const o = t as Record<string, unknown>;
    const v = o.zh ?? o.en;
    return typeof v === "string" ? v : "";
  }
  return "";
}

type PortalServer = {
  key: string;
  game: string;
  name: string;
  map: string;
  status: "online" | "starting" | "offline";
  players: number;
  max_players: number;
  password: boolean;
  pvp: boolean;
  cluster_group: number;
  rates: { xp: number; taming: number; harvest: number; breeding: number };
  address: string;
  query_port: number;
  room_id: number;
  next_restart_at: string | null;
  shop: boolean;
  settings_page?: string;
  points?: boolean;
  mods?: { id: string; title: string }[];
};

type Entry = { blueprint: string; amount: number; quality: number; blueprint_only: boolean };
type Dino = { blueprint: string; level: number; neutered: boolean; gender: string; saddle_blueprint: string };
type Req = { min_level: number; max_level: number; permissions: string[] };
type ShopItem = Req & {
  id: string;
  type: string;
  description: string;
  price: number;
  items?: Entry[];
  dinos?: Dino[];
  beacon?: string;
  experience?: { amount: number; to_dino: boolean };
  engrams?: string[];
  commands?: string[];
};
type Kit = Req & {
  name: string;
  description: string;
  price: number | null;
  default_amount: number;
  only_from_spawn: boolean;
  items: Entry[];
  dinos: Dino[];
  commands: string[];
};
type Shop = {
  commands: { buy: string; kit: string; buy_kit: string; points: string; sell: string };
  items: ShopItem[];
  kits: Kit[];
  sell: { id: string; description: string; price: number; amount: number; blueprint: string }[];
};

const REFRESH_MS = 15000;
const RECENT_KEY = "portal-recent";

// ---- 最近打开的玩家站（公网服务器，存在本机） ----

export function recentPortals(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string").slice(0, 10) : [];
  } catch {
    return [];
  }
}

export function rememberPortal(url: string) {
  const list = [url, ...recentPortals().filter((u) => u !== url)].slice(0, 10);
  localStorage.setItem(RECENT_KEY, JSON.stringify(list));
}

function forgetPortal(url: string) {
  localStorage.setItem(RECENT_KEY, JSON.stringify(recentPortals().filter((u) => u !== url)));
}

// ---- 左栏 + 详情 ----

export function PortalPane({ roomPortal, roomName, selected, onSelect }: { roomPortal: string; roomName: string; selected: string; onSelect: (url: string) => void }) {
  const [recent, setRecent] = useState(recentPortals);
  const [input, setInput] = useState("");
  const [inputError, setInputError] = useState("");

  const open = async () => {
    setInputError("");
    try {
      const url = await PortalService.Normalize(input);
      rememberPortal(url);
      setRecent(recentPortals());
      setInput("");
      onSelect(url);
    } catch (e) {
      setInputError(errText(e));
    }
  };

  // 从链接打开时外面已经记下了，这里刷新列表
  useEffect(() => setRecent(recentPortals()), [selected]);

  const current = selected || roomPortal || recent[0] || "";
  return (
    <div className="body">
      <aside className="list tool-nav">
        <div className="list-head tool-nav-head">
          <span>玩家站</span>
        </div>
        <div className="rows">
          {roomPortal && (
            <button className={`row ${current === roomPortal ? "selected" : ""}`} onClick={() => onSelect(roomPortal)}>
              <span className="row-name">{roomName || "当前房间"}</span>
              <span className="row-meta">当前联机房的玩家站</span>
            </button>
          )}
          {recent.map((u) => (
            <div key={u} className={`row portal-row ${current === u ? "selected" : ""}`}>
              <button className="portal-row-main" onClick={() => onSelect(u)}>
                <span className="row-name">{hostOf(u)}</span>
                <span className="row-meta">{u}</span>
              </button>
              <button
                className="icon-btn"
                title="从列表里移除"
                onClick={() => {
                  forgetPortal(u);
                  setRecent(recentPortals());
                }}
              >
                ×
              </button>
            </div>
          ))}
        </div>
        <div className="portal-add">
          <input
            className="text portal-input"
            value={input}
            placeholder="粘贴玩家站地址"
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && input.trim() && open()}
          />
          <button className="ghost small" disabled={!input.trim()} onClick={open}>
            打开
          </button>
          {inputError && <p className="bad small-text">{inputError}</p>}
        </div>
      </aside>
      <main className="detail">
        {current ? (
          <PortalView key={current} base={current} inRoom={current === roomPortal} />
        ) : (
          <div className="detail-msg">
            <p>还没有打开过玩家站。</p>
            <p className="muted small-text">
              服主开启了玩家站时：联机房加入后会出现在这里；公网服务器可以在 arkfgf.com 的服务器详情页点「用联机助手打开」，或者粘贴服主给的地址。
            </p>
          </div>
        )}
      </main>
    </div>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

// ---- 一个玩家站 ----

type Tab = "news" | "servers" | "pages" | "shop";

function PortalView({ base, inRoom }: { base: string; inRoom: boolean }) {
  const [data, setData] = useState<PortalData | null>(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState<Tab>("news");
  const [shopKey, setShopKey] = useState("");
  const [pageId, setPageId] = useState("");
  const [me, setMe] = useState<MeData | null>(null);

  const load = useCallback(async () => {
    try {
      const raw = await PortalService.Portal(base);
      setData(JSON.parse(raw) as PortalData);
      setError("");
    } catch (e) {
      setError(errText(e));
    }
  }, [base]);

  useEffect(() => {
    load();
    const t = setInterval(() => !document.hidden && load(), REFRESH_MS);
    return () => clearInterval(t);
  }, [load]);

  // 自己在这个玩家站上的数据（点数）：用助手的 arkfgf.com 登录自动登录玩家站，失败时不显示
  const loadMe = useCallback(async () => {
    try {
      setMe(JSON.parse(await PortalService.Me(base)) as MeData);
    } catch {
      setMe(null);
    }
  }, [base]);
  useEffect(() => {
    loadMe();
    const t = setInterval(() => !document.hidden && loadMe(), 30000);
    return () => clearInterval(t);
  }, [loadMe]);

  if (!data) {
    return (
      <div className="detail-msg">
        {error ? (
          <>
            <p className="bad">{error}</p>
            {inRoom && <p className="muted small-text">联机房的玩家站要在显示「已直连」之后才能打开。</p>}
            <button className="ghost small" onClick={load}>
              重试
            </button>
          </>
        ) : (
          <p className="muted">
            <span className="spinner" /> 正在读取玩家站…
          </p>
        )}
      </div>
    );
  }

  const site = data.site ?? { name: "", description: "", qq_group: "", discord: "", links: [] };
  const servers = data.servers ?? [];
  const shops = servers.filter((s) => s.shop);
  const pages = data.pages ?? [];
  const customPages = pages.filter((pg) => pg.kind === "custom");
  const name = site.name || servers[0]?.name || "玩家站";
  const tabs: { id: Tab; label: string }[] = [
    { id: "news", label: `公告（${(data.announcements ?? []).length}）` },
    { id: "servers", label: `服务器（${servers.length}）` },
    ...(customPages.length ? [{ id: "pages" as Tab, label: "指南" }] : []),
    ...(shops.length ? [{ id: "shop" as Tab, label: "商店" }] : []),
  ];
  const openPage = (id: string) => {
    setPageId(id);
    setTab("pages");
  };

  return (
    <div className="room portal">
      <div className="room-head">
        <div>
          <h2>{name}</h2>
          {site.description && <p className="desc">{site.description}</p>}
        </div>
        <div className="spacer" />
        {!inRoom && (
          <button className="ghost small" onClick={() => PortalService.OpenInBrowser(base)}>
            在浏览器中打开
          </button>
        )}
      </div>
      <Contacts site={site} />
      {me?.user && <p className="muted small-text">已用 {me.user.name || me.user.steam_id} 登录这个玩家站</p>}
      {error && <p className="bad small-text">刷新失败：{error}（显示的是上一次的内容）</p>}

      <div className="seg portal-tabs">
        {tabs.map((t) => (
          <button key={t.id} className={tab === t.id ? "on" : ""} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === "news" && <News items={data.announcements ?? []} />}
      {tab === "servers" && <Servers servers={servers} onShop={(k) => (setShopKey(k), setTab("shop"))} onPage={openPage} />}
      {tab === "pages" && <Pages pages={pages} servers={servers} selected={pageId || customPages[0]?.id || ""} onSelect={setPageId} />}
      {tab === "shop" && shops.length > 0 && (
        <ShopTab base={base} servers={shops} selected={shopKey || shops[0].key} onSelect={setShopKey} me={me} />
      )}
    </div>
  );
}

function Contacts({ site }: { site: PortalData["site"] }) {
  const links = [...(site.discord ? [{ title: "Discord", url: site.discord }] : []), ...(site.links ?? [])].filter((l) => /^https?:\/\//i.test(l.url));
  if (!site.qq_group && links.length === 0) return null;
  return (
    <div className="contacts">
      {site.qq_group && <span className="tag">QQ 群 {site.qq_group}</span>}
      {links.map((l) => (
        <button key={l.url} className="ghost small" title={l.url} onClick={() => PortalService.OpenLink(l.url)}>
          {l.title || hostOf(l.url)}
        </button>
      ))}
    </div>
  );
}

function fmtTime(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function News({ items }: { items: PortalData["announcements"] }) {
  if (items.length === 0) return <p className="muted">还没有公告</p>;
  const sorted = [...items].sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.published_at.localeCompare(a.published_at));
  return (
    <div className="portal-list">
      {sorted.map((a) => (
        <article key={a.id} className={`room-server ${a.pinned ? "linked" : ""}`}>
          <div className="room-server-head">
            {a.pinned && <span className="tag">置顶</span>}
            <span className="name">{a.title}</span>
            <span className="sv-players">{fmtTime(a.published_at)}</span>
          </div>
          {a.body && <p className="desc">{a.body}</p>}
        </article>
      ))}
    </div>
  );
}

const statusText: Record<string, string> = { online: "在线", starting: "启动中", offline: "离线" };

function Servers({ servers, onShop, onPage }: { servers: PortalServer[]; onShop: (key: string) => void; onPage: (id: string) => void }) {
  const [copied, setCopied] = useState("");
  if (servers.length === 0) return <p className="muted">没有服务器</p>;
  const copy = async (k: string, text: string) => {
    await Clipboard.SetText(text);
    setCopied(k);
    setTimeout(() => setCopied((x) => (x === k ? "" : x)), 1500);
  };
  return (
    <div className="portal-list">
      {servers.map((s) => {
        const asa = s.game === "asa";
        const host = s.address ? s.address.slice(0, s.address.lastIndexOf(":")) : "";
        const steamAddr = !asa && s.query_port > 0 ? `${host}:${s.query_port}` : s.address;
        return (
          <div key={s.key} className="room-server">
            <div className="room-server-head">
              <span className={`status-dot ${s.status === "online" ? "ok" : s.status === "starting" ? "busy" : "idle"}`} />
              <span className="name">{s.name}</span>
              {asa && <span className="game-badge">ASA</span>}
              <span className="sv-players">{s.status === "online" ? `${s.players}/${s.max_players}` : statusText[s.status] ?? ""}</span>
            </div>
            <div className="sv-facts">
              <span>{mapName(s.map)}</span>
              <span>{s.pvp ? "PvP" : "PvE"}</span>
              {s.password && <span>有密码</span>}
              {s.cluster_group > 0 && <span>集群 {s.cluster_group}</span>}
              <span>
                经验 {rate(s.rates?.xp)} · 驯服 {rate(s.rates?.taming)} · 采集 {rate(s.rates?.harvest)} · 繁殖 {rate(s.rates?.breeding)}
              </span>
              {s.next_restart_at && <span>下次重启 {fmtTime(s.next_restart_at)}</span>}
            </div>
            {s.address ? (
              <div className="command">
                <code>{asa ? `open ${s.address}` : steamAddr}</code>
                <div className="btns">
                  <button className={copied === s.key ? "ok small" : "ghost small"} onClick={() => copy(s.key, asa ? `open ${s.address}` : steamAddr)}>
                    {copied === s.key ? "已复制" : asa ? "复制命令" : "复制地址"}
                  </button>
                </div>
              </div>
            ) : s.room_id > 0 ? (
              <p className="muted small-text">P2P 联机房 {s.room_id}，在「联机大厅」里加入进服</p>
            ) : null}
            {(s.mods ?? []).length > 0 && (
              <div className="sv-facts">
                <span>Mod：{(s.mods ?? []).map((m) => m.title || m.id).join("、")}</span>
              </div>
            )}
            <div className="btns">
              {s.settings_page && (
                <button className="ghost small" onClick={() => onPage(s.settings_page!)}>
                  服务器设置
                </button>
              )}
              {s.shop && (
                <button className="ghost small" onClick={() => onShop(s.key)}>
                  查看商店
                </button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ---- 页面 ----

function pageTitle(pg: Page, servers: PortalServer[]): string {
  const title = zhText(pg.title);
  const sv = pg.server_key ? servers.find((s) => s.key === pg.server_key) : undefined;
  return sv ? `${title} · ${sv.name}` : title;
}

function Pages({ pages, servers, selected, onSelect }: { pages: Page[]; servers: PortalServer[]; selected: string; onSelect: (id: string) => void }) {
  const custom = pages.filter((pg) => pg.kind === "custom");
  const page = pages.find((pg) => pg.id === selected);
  return (
    <div className="portal-shop">
      {custom.length > 1 && (
        <div className="seg portal-tabs">
          {custom.map((pg) => (
            <button key={pg.id} className={pg.id === selected ? "on" : ""} onClick={() => onSelect(pg.id)}>
              {zhText(pg.title)}
            </button>
          ))}
        </div>
      )}
      {!page ? (
        <p className="muted">页面不存在</p>
      ) : (
        <div className="room-server portal-page">
          <h3>{pageTitle(page, servers)}</h3>
          {page.blocks.map((b, i) => (
            <BlockView key={i} block={b} />
          ))}
        </div>
      )}
    </div>
  );
}

function BlockView({ block }: { block: Block }) {
  switch (block.type) {
    case "heading":
      return <h4 className="portal-h">{zhText(block.text)}</h4>;
    case "paragraph":
      return <p className="desc">{zhText(block.text)}</p>;
    case "list":
      return (
        <ul className="portal-ul">
          {(block.items ?? []).map((it, i) => (
            <li key={i}>{zhText(it)}</li>
          ))}
        </ul>
      );
    case "kv":
      return (
        <dl className="portal-kv">
          {((block.items ?? []) as { label: unknown; value: unknown }[]).map((it, i) => (
            <div key={i}>
              <dt>{zhText(it.label)}</dt>
              <dd>{typeof it.value === "boolean" ? (it.value ? "开" : "关") : String(it.value ?? "")}</dd>
            </div>
          ))}
        </dl>
      );
    default:
      return null;
  }
}

// ---- 商店 ----

const typeLabel: Record<string, string> = { item: "物品", dino: "恐龙", beacon: "补给箱", experience: "经验", unlockengram: "解锁图纸", command: "特殊" };
const genderLabel: Record<string, string> = { male: "公", female: "母", random: "随机性别" };

// bpName 从蓝图路径取出类名：.../PrimalItemResource_MetalIngot.PrimalItemResource_MetalIngot' → MetalIngot
function bpName(bp: string): string {
  let s = bp.trim().replace(/^[A-Za-z]*'/, "").replace(/'$/, "");
  s = s.split("/").pop() ?? s;
  s = s.split(".").pop() ?? s;
  s = s.replace(/_C$/, "").replace(/^PrimalItem(Resource|Armor|Consumable|Skin|Structure|Ammo|Dye|CustomFood|Costume|Artifact|Trophy)?_/, "");
  s = s.replace(/^Weapon(?=[A-Z])/, "").replace(/_Character_BP.*$/, "").replace(/_BP$/, "");
  return s || bp;
}

function entryLine(e: Entry): string {
  return `${bpName(e.blueprint)} × ${e.amount}${e.blueprint_only ? "（图纸）" : ""}${e.quality > 0 ? `，品质 ${e.quality}` : ""}`;
}

function dinoLine(d: Dino): string {
  const extra = [genderLabel[d.gender], d.neutered && "已绝育", d.saddle_blueprint && `带 ${bpName(d.saddle_blueprint)}`].filter(Boolean);
  return `${bpName(d.blueprint)} ${d.level} 级${extra.length ? `（${extra.join("，")}）` : ""}`;
}

function itemLines(i: ShopItem): string[] {
  return [
    ...(i.items ?? []).map(entryLine),
    ...(i.dinos ?? []).map(dinoLine),
    ...(i.beacon ? [`补给箱 ${i.beacon}`] : []),
    ...(i.experience ? [`${i.experience.amount} 经验${i.experience.to_dino ? "（给骑乘的恐龙）" : ""}`] : []),
    ...(i.engrams ?? []).map((bp) => `解锁 ${bpName(bp)}`),
    ...(i.commands ?? []),
  ];
}

function reqText(r: Req & { only_from_spawn?: boolean }): string[] {
  const out: string[] = [];
  if (r.min_level > 0 && r.max_level > 0) out.push(`等级 ${r.min_level}–${r.max_level}`);
  else if (r.min_level > 0) out.push(`等级 ≥ ${r.min_level}`);
  else if (r.max_level > 0) out.push(`等级 ≤ ${r.max_level}`);
  if (r.permissions?.length) out.push(`仅限 ${r.permissions.join("、")}`);
  if (r.only_from_spawn) out.push("只能在刚出生时领取");
  return out;
}

function ShopTab({ base, servers, selected, onSelect, me }: { base: string; servers: PortalServer[]; selected: string; onSelect: (k: string) => void; me: MeData | null }) {
  const mine = me?.user && servers.find((s) => s.key === selected)?.points ? me.data?.[selected] : undefined;
  const [shop, setShop] = useState<Shop | null>(null);
  const [error, setError] = useState("");
  const [kind, setKind] = useState<"items" | "kits" | "sell">("items");
  const [q, setQ] = useState("");
  const seq = useRef(0);

  const load = useCallback(async () => {
    const my = ++seq.current;
    try {
      const raw = await PortalService.Shop(base, selected);
      if (my !== seq.current) return;
      setShop(raw ? (JSON.parse(raw) as Shop) : null);
      setError(raw ? "" : "这台服务器没有商店");
    } catch (e) {
      if (my === seq.current) setError(errText(e));
    }
  }, [base, selected]);

  useEffect(() => {
    setShop(null);
    load();
    const t = setInterval(() => !document.hidden && load(), REFRESH_MS);
    return () => clearInterval(t);
  }, [load]);

  const query = q.trim().toLowerCase();
  const match = (...f: string[]) => !query || f.some((x) => x.toLowerCase().includes(query));
  const items = useMemo(() => (shop?.items ?? []).filter((i) => match(i.id, i.description, ...itemLines(i))), [shop, query]);
  const kits = useMemo(
    () => (shop?.kits ?? []).filter((k) => match(k.name, k.description, ...k.items.map(entryLine), ...k.dinos.map(dinoLine))),
    [shop, query],
  );
  const sell = useMemo(() => (shop?.sell ?? []).filter((s) => match(s.id, s.description, bpName(s.blueprint))), [shop, query]);

  return (
    <div className="portal-shop">
      {servers.length > 1 && (
        <div className="seg">
          {servers.map((s) => (
            <button key={s.key} className={s.key === selected ? "on" : ""} onClick={() => onSelect(s.key)}>
              {s.name}
            </button>
          ))}
        </div>
      )}
      {!shop ? (
        <p className={error ? "bad" : "muted"}>{error || "正在读取商店…"}</p>
      ) : (
        <>
          <p className="muted small-text">
            在游戏里按 Enter 打开聊天框，粘贴命令购买；点数不够或不满足条件时游戏里会提示。查看点数：<code>{shop.commands.points}</code>
          </p>
          {mine && (
            <p className="portal-price">
              我的点数：{mine.points}（已花 {mine.total_spent}）
            </p>
          )}
          <div className="portal-shop-bar">
            <div className="seg">
              <button className={kind === "items" ? "on" : ""} onClick={() => setKind("items")}>
                商品 {shop.items.length}
              </button>
              {shop.kits.length > 0 && (
                <button className={kind === "kits" ? "on" : ""} onClick={() => setKind("kits")}>
                  礼包 {shop.kits.length}
                </button>
              )}
              {shop.sell.length > 0 && (
                <button className={kind === "sell" ? "on" : ""} onClick={() => setKind("sell")}>
                  回收 {shop.sell.length}
                </button>
              )}
            </div>
            <input className="text portal-input" value={q} placeholder="搜索名称、编号" onChange={(e) => setQ(e.target.value)} />
          </div>
          <div className="portal-list">
            {kind === "items" &&
              (items.length ? (
                items.map((i) => (
                  <ShopRow
                    key={i.id}
                    badge={typeLabel[i.type] ?? i.type}
                    title={i.description || i.id}
                    price={`${i.price} 点`}
                    lines={itemLines(i)}
                    meta={`编号 ${i.id}`}
                    reqs={reqText(i)}
                    commands={[{ label: "购买", amount: true, build: (n) => `${shop.commands.buy} ${i.id} ${n}` }]}
                  />
                ))
              ) : (
                <p className="muted">没有符合条件的商品</p>
              ))}
            {kind === "kits" &&
              kits.map((k) => (
                <ShopRow
                  key={k.name}
                  badge="礼包"
                  title={k.description || k.name}
                  price={k.price !== null ? `${k.price} 点` : "免费"}
                  lines={[...k.items.map(entryLine), ...k.dinos.map(dinoLine), ...k.commands]}
                  meta={`名称 ${k.name}${k.default_amount > 0 ? ` · 每人免费 ${k.default_amount} 次` : ""}${mine?.kits?.[k.name] !== undefined ? ` · 你还能领 ${mine.kits[k.name]} 次` : ""}`}
                  reqs={reqText(k)}
                  commands={[
                    ...(k.default_amount > 0 ? [{ label: "领取", amount: false, build: () => `${shop.commands.kit} ${k.name}` }] : []),
                    ...(k.price !== null ? [{ label: "购买", amount: true, build: (n: number) => `${shop.commands.buy_kit} ${k.name} ${n}` }] : []),
                  ]}
                />
              ))}
            {kind === "sell" &&
              sell.map((s) => (
                <ShopRow
                  key={s.id}
                  badge="回收"
                  title={s.description || s.id}
                  price={`+${s.price} 点`}
                  lines={[`交出 ${bpName(s.blueprint)} × ${s.amount}`]}
                  meta={`编号 ${s.id}`}
                  reqs={[]}
                  commands={[{ label: "出售", amount: true, build: (n) => `${shop.commands.sell} ${s.id} ${n}` }]}
                />
              ))}
          </div>
        </>
      )}
    </div>
  );
}

type Cmd = { label: string; amount: boolean; build: (n: number) => string };

function ShopRow({ badge, title, price, lines, meta, reqs, commands }: { badge: string; title: string; price: string; lines: string[]; meta: string; reqs: string[]; commands: Cmd[] }) {
  return (
    <div className="room-server">
      <div className="room-server-head">
        <span className="tag">{badge}</span>
        <span className="name">{title}</span>
        <span className="sv-players portal-price">{price}</span>
      </div>
      <div className="sv-facts portal-lines">
        {lines.map((l, i) => (
          <span key={i}>{l}</span>
        ))}
        <span className="muted">{meta}</span>
      </div>
      {reqs.length > 0 && (
        <div className="contacts">
          {reqs.map((r) => (
            <span key={r} className="tag warn-text">
              {r}
            </span>
          ))}
        </div>
      )}
      {commands.map((c) => (
        <CommandLine key={c.label} cmd={c} />
      ))}
    </div>
  );
}

function CommandLine({ cmd }: { cmd: Cmd }) {
  const [n, setN] = useState(1);
  const [copied, setCopied] = useState(false);
  const text = cmd.build(n);
  return (
    <div className="command">
      <span className="muted small-text">{cmd.label}</span>
      <code>{text}</code>
      <div className="btns">
        {cmd.amount && (
          <input
            className="text"
            type="number"
            min={1}
            max={999}
            value={n}
            aria-label="数量"
            onChange={(e) => setN(Math.min(999, Math.max(1, Math.floor(Number(e.target.value)) || 1)))}
          />
        )}
        <button
          className={copied ? "ok small" : "ghost small"}
          onClick={async () => {
            await Clipboard.SetText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
        >
          {copied ? "已复制" : "复制"}
        </button>
      </div>
    </div>
  );
}
