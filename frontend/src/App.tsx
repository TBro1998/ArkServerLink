import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { Clipboard, Events } from "@wailsio/runtime";
import { AuthService, RoomService } from "../bindings/arkfgf.com/link";
import type { AuthState } from "../bindings/arkfgf.com/link/models";
import type { Room, RoomList } from "../bindings/arkfgf.com/link/internal/arkfgf/models";
import { Phase, type Status } from "../bindings/arkfgf.com/link/internal/session/models";
import { AuthChanged, DeeplinkJoin } from "../bindings/events";
import { RoomDetailPane } from "./RoomDetail";
import { errText, mapName } from "./format";
import { ASABadge, hasASA, isActive, LockIcon, phaseLabel, phaseTone, SteamIcon, type LoginProps } from "./shared";
import { UpdateBadge, UpdateDialog, useUpdate } from "./Update";

const idleStatus: Status = {
  phase: Phase.PhaseIdle,
  room_id: 0,
  room_name: "",
  local_port: 0,
  raw_port: 0,
  query_port: 0,
  command: "",
  servers: [],
  latency_ms: 0,
  message: "",
  error_code: "",
  retry_in: 0,
};

export default function App() {
  const [auth, setAuth] = useState<AuthState | null>(null);
  const [loggingIn, setLoggingIn] = useState(false);
  const [loginError, setLoginError] = useState("");

  const [status, setStatus] = useState<Status>(idleStatus);
  const [selected, setSelected] = useState<number>(0);
  // 通过 arkfgf-link:// 链接打开、等待自动加入的房间
  const [autoJoin, setAutoJoin] = useState<number>(0);

  // 登录状态
  useEffect(() => {
    AuthService.Status().then(setAuth).catch(() => setAuth({ logged_in: false, user: null, message: "" }));
    return Events.On(AuthChanged, (ev) => setAuth(ev.data));
  }, []);

  // 连接状态
  useEffect(() => {
    let alive = true;
    const poll = () =>
      RoomService.Status()
        .then((s) => alive && setStatus(s))
        .catch(() => {});
    poll();
    const t = setInterval(poll, 1000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  // 链接唤起：先订阅事件再取一次，启动时就带着的链接也不会漏
  useEffect(() => {
    const take = () =>
      RoomService.TakePendingJoin().then((id) => {
        if (id > 0) {
          setSelected(id);
          setAutoJoin(id);
        }
      });
    const off = Events.On(DeeplinkJoin, () => take());
    take();
    return off;
  }, []);

  const login = useCallback(async () => {
    setLoggingIn(true);
    setLoginError("");
    try {
      setAuth(await AuthService.Login());
    } catch (e) {
      setLoginError(errText(e));
    } finally {
      setLoggingIn(false);
    }
  }, []);

  const cancelLogin = () => AuthService.CancelLogin();
  const logout = async () => {
    await AuthService.Logout();
    setAuth({ logged_in: false, user: null, message: "" });
  };

  const loginProps = { auth, loggingIn, loginError, login, cancelLogin };
  const update = useUpdate();
  const [updateOpen, setUpdateOpen] = useState(false);

  return (
    <div className="app">
      <Header {...loginProps} logout={logout} status={status} onOpenRoom={setSelected}>
        <UpdateBadge state={update} onOpen={() => setUpdateOpen(true)} />
      </Header>
      <div className="body">
        <RoomListPane selected={selected} onSelect={setSelected} activeRoom={isActive(status) ? status.room_id : 0} />
        <main className="detail">
          {selected ? (
            <RoomDetailPane
              key={selected}
              id={selected}
              status={status}
              autoJoin={autoJoin === selected}
              onAutoJoinDone={() => setAutoJoin(0)}
              {...loginProps}
            />
          ) : (
            <Welcome status={status} onOpenRoom={setSelected} />
          )}
        </main>
      </div>
      {updateOpen && update && <UpdateDialog state={update} connected={isActive(status)} onClose={() => setUpdateOpen(false)} />}
    </div>
  );
}

function Header(props: LoginProps & { logout: () => void; status: Status; onOpenRoom: (id: number) => void; children?: ReactNode }) {
  const { auth, loggingIn, login, cancelLogin, logout, status, onOpenRoom, children } = props;
  const [menu, setMenu] = useState(false);
  return (
    <header className="topbar">
      <div className="brand">
        <img className="mark" src="/arkfgf-icon-64.png" alt="" />
        <span className="brand-name">ARKFGF</span>
        <span className="brand-sub">联机助手</span>
      </div>
      {isActive(status) && (
        <button className="session-pill" onClick={() => onOpenRoom(status.room_id)} title="查看当前连接">
          <span className={`dot ${phaseTone(status.phase)}`} />
          <span className="pill-name">{status.room_name || `房间 #${status.room_id}`}</span>
          <span className="pill-state">{phaseLabel(status.phase)}</span>
        </button>
      )}
      <div className="spacer" />
      {children}
      {auth === null ? null : auth.logged_in ? (
        <div className="user" onMouseLeave={() => setMenu(false)}>
          <button className="user-chip" onClick={() => setMenu(!menu)}>
            {auth.user?.avatar_url ? <img src={auth.user.avatar_url} alt="" /> : <span className="avatar-fallback">{(auth.user?.name || "?").slice(0, 1)}</span>}
            <span>{auth.user?.name || "已登录"}</span>
          </button>
          {menu && (
            <div className="menu">
              {auth.message && <p className="menu-note">{auth.message}</p>}
              <button onClick={logout}>退出登录</button>
            </div>
          )}
        </div>
      ) : loggingIn ? (
        <div className="login-wait">
          <span className="spinner" /> 请在弹出的 Steam 窗口（或浏览器）里点「登录」…
          <button className="ghost small" onClick={cancelLogin}>
            取消
          </button>
        </div>
      ) : (
        <button className="steam" onClick={login}>
          <SteamIcon /> 用 Steam 登录
        </button>
      )}
    </header>
  );
}

function RoomListPane({ selected, onSelect, activeRoom }: { selected: number; onSelect: (id: number) => void; activeRoom: number }) {
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [data, setData] = useState<RoomList | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const seq = useRef(0);

  const load = useCallback(async (query: string, p: number) => {
    const my = ++seq.current;
    setLoading(true);
    setError("");
    try {
      const res = await RoomService.List(query, p);
      if (my === seq.current) setData(res);
    } catch (e) {
      if (my === seq.current) setError(errText(e));
    } finally {
      if (my === seq.current) setLoading(false);
    }
  }, []);

  // 输入停顿 300ms 后搜索
  useEffect(() => {
    const t = setTimeout(() => load(q, page), q ? 300 : 0);
    return () => clearTimeout(t);
  }, [q, page, load]);

  const items = data?.items ?? [];
  return (
    <aside className="list">
      <div className="list-tools">
        <div className="search">
          <SearchIcon />
          <input
            placeholder="搜索房间名、地图、标签"
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setPage(1);
            }}
          />
        </div>
        <button className="icon-btn" title="刷新" onClick={() => load(q, page)} disabled={loading}>
          <RefreshIcon spinning={loading} />
        </button>
      </div>
      <div className="list-head">
        <span>在线房间</span>
        <span>{data ? `${data.total} 个${data.stats ? ` · ${data.stats.players} 人在线` : ""}` : ""}</span>
      </div>
      <div className="rows">
        {error && (
          <div className="list-msg bad">
            {error}
            <button className="ghost small" onClick={() => load(q, page)}>
              重试
            </button>
          </div>
        )}
        {!error && data && items.length === 0 && <div className="list-msg">{q ? "没有找到匹配的房间" : "暂时没有在线的联机房"}</div>}
        {items.map((r) => (
          <RoomRow key={r.id} room={r} selected={r.id === selected} active={r.id === activeRoom} onClick={() => onSelect(r.id)} />
        ))}
      </div>
      {data && data.pages > 1 && (
        <div className="pager">
          <button className="ghost small" disabled={page <= 1 || loading} onClick={() => setPage(page - 1)}>
            上一页
          </button>
          <span>
            {data.page} / {data.pages}
          </span>
          <button className="ghost small" disabled={page >= data.pages || loading} onClick={() => setPage(page + 1)}>
            下一页
          </button>
        </div>
      )}
    </aside>
  );
}

function RoomRow({ room, selected, active, onClick }: { room: Room; selected: boolean; active: boolean; onClick: () => void }) {
  const full = room.max_players > 0 && room.players >= room.max_players;
  const mods = room.mods?.length ?? 0;
  return (
    <button className={`row ${selected ? "selected" : ""}`} onClick={onClick}>
      <div className="row-top">
        <span className="row-name">
          {room.password && <LockIcon />}
          {room.name || `房间 #${room.id}`}
        </span>
        <span className={`players ${full ? "full" : ""}`}>
          {room.players}
          <small>/{room.max_players || "?"}</small>
        </span>
      </div>
      <div className="row-meta">
        {hasASA(room.servers) && <ASABadge />}
        <span>{mapName(room.map)}</span>
        <span className={`mode ${room.pvp ? "pvp" : "pve"}`}>{room.pvp ? "PvP" : "PvE"}</span>
        {mods > 0 && <span>{mods} 个 Mod</span>}
        {active && <span className="here">当前房间</span>}
      </div>
      {(room.tags?.length ?? 0) > 0 && (
        <div className="row-tags">
          {room.tags!.slice(0, 4).map((t) => (
            <span key={t} className="tag">
              {t}
            </span>
          ))}
        </div>
      )}
    </button>
  );
}

function Welcome({ status, onOpenRoom }: { status: Status; onOpenRoom: (id: number) => void }) {
  return (
    <div className="welcome">
      <h1>
        没有公网 IP 的服务器，<span className="hl">也能直接进</span>
      </h1>
      <p>联机助手通过 P2P 打洞把你和服务器直接连起来，流量不经过任何中转服务器。</p>
      <ol className="steps">
        <li>
          <b>选择房间</b>从左侧列表挑一个联机房，或在 arkfgf.com 点「用联机助手加入」
        </li>
        <li>
          <b>加入</b>用 Steam 登录后点「加入」，等待显示「已直连」
        </li>
        <li>
          <b>进服</b>点「进入游戏」，助手启动 ARK 并自动连进服务器。ASA（生存飞升）服务器在游戏里按 Tab 打开控制台，粘贴助手给出的命令进服
        </li>
      </ol>
      {isActive(status) && (
        <button className="ghost" onClick={() => onOpenRoom(status.room_id)}>
          查看当前连接：{status.room_name || `房间 #${status.room_id}`}
        </button>
      )}
      <QQGroup />
    </div>
  );
}

const QQ_GROUP = "879898092";

function QQGroup() {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await Clipboard.SetText(QQ_GROUP);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="community">
      <span>arkfgf.com 玩家交流群（QQ）</span>
      <code>{QQ_GROUP}</code>
      <button className="ghost small" onClick={copy}>
        {copied ? "已复制" : "复制群号"}
      </button>
    </div>
  );
}

function SearchIcon() {
  return (
    <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2">
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  );
}

function RefreshIcon({ spinning }: { spinning: boolean }) {
  return (
    <svg className={spinning ? "spin" : ""} viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M20 11a8 8 0 1 0-2.3 5.7" />
      <path d="M20 4v7h-7" />
    </svg>
  );
}

