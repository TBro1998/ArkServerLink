package arkfgf

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Rates 是房间的倍率。
type Rates struct {
	XP       float64 `json:"xp"`
	Tame     float64 `json:"tame"`
	Harvest  float64 `json:"harvest"`
	Breeding float64 `json:"breeding"`
}

// Room 对应接口里的 publicRoom。
type Room struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Map         string   `json:"map"`
	Players     int      `json:"players"`
	MaxPlayers  int      `json:"max_players"`
	Password    bool     `json:"password"`
	PvP         bool     `json:"pvp"`
	ArkAPI      bool     `json:"arkapi"`
	Cluster     string   `json:"cluster"`
	Mods        []string `json:"mods"`
	Version     string   `json:"version"`
	Rates       Rates    `json:"rates"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	QQGroup     string   `json:"qq_group"`
	Discord     string   `json:"discord"`
	// 服主开启了玩家站（公告、服务器状态、商店），加入房间后可以在助手里查看
	Portal    bool     `json:"portal"`
	Online    bool     `json:"online"`
	LastSeen  UnixTime `json:"last_seen"`
	CreatedAt UnixTime `json:"created_at"`
	Country   string   `json:"country"`
	Province  string   `json:"province"`
	// 房间里的服务器（一个房间 = 一个 Commander，可以有多台）；上面的地图、人数、倍率等是汇总值
	Servers []RoomServer `json:"servers"`
}

// Game 是服务器的游戏：ASE（方舟：生存进化）或 ASA（方舟：生存飞升）。
type Game string

const (
	GameASE Game = "ase"
	GameASA Game = "asa"
)

// NormalizeGame 规范化网站返回的 game 字段：旧版网站不返回（空字符串），按 ASE 处理；不认识的值也按 ASE 处理。
func NormalizeGame(g Game) Game {
	if strings.EqualFold(strings.TrimSpace(string(g)), string(GameASA)) {
		return GameASA
	}
	return GameASE
}

// RoomServer 是房间里的一台服务器（公开信息，不含端口）。
type RoomServer struct {
	// Commander 里的服务器 ID，和加入响应里的 key 相同（旧版网站不返回，为空）
	Key        string `json:"key"`
	Name       string `json:"name"`
	Game       Game   `json:"game"` // ase 或 asa，客户端解析后不会为空
	Map        string `json:"map"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Password   bool   `json:"password"`
	PvP        bool   `json:"pvp"`
	ArkAPI     bool   `json:"arkapi"`
	InCluster  bool   `json:"in_cluster"`
	// 房间里的集群编号：同一集群的服务器编号相同（从 1 开始），不属于集群为 0
	ClusterGroup int      `json:"cluster_group"`
	Mods         []string `json:"mods"` // ASE 是创意工坊 ID，ASA 是 CurseForge 项目 ID
	Version      string   `json:"version"`
	Rates        Rates    `json:"rates"`
	Online       bool     `json:"online"`
}

// Mod 是房间详情里带名称的 Mod。
type Mod struct {
	ID    string `json:"id"`
	Game  Game   `json:"game"`  // 用到这个 Mod 的服务器的游戏；ASA 的 Mod 在 CurseForge 上，没有名称
	Title string `json:"title"` // 还没查到名称时为空
}

// RoomDetail 是 GET /link/rooms/:id 的响应。
type RoomDetail struct {
	Room
	ModList []Mod `json:"mod_list"`
}

// RoomStats 是全站在线房间数和人数（网站在约定之外额外返回，可能没有）。
type RoomStats struct {
	Rooms   int `json:"rooms"`
	Players int `json:"players"`
}

// RoomList 是 GET /link/rooms 的响应。
type RoomList struct {
	Items []Room     `json:"items"`
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Pages int        `json:"pages"`
	Stats *RoomStats `json:"stats"`
}

// RoomRef 是加入响应里房间的简要信息。
type RoomRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// JoinResult 是 POST /link/rooms/:id/join 的响应：入网需要的全部参数。
type JoinResult struct {
	Room                RoomRef  `json:"room"`
	NetworkName         string   `json:"network_name"`
	Credential          string   `json:"credential"`
	CredentialExpiresAt int64    `json:"credential_expires_at"`
	Peers               []string `json:"peers"`
	ServerIP            string   `json:"server_ip"`
	// 房间里的全部服务器：同一个虚拟 IP，靠端口区分，客户端为每台服务器各转发一组端口
	Servers []JoinServer `json:"servers"`
	// 服主玩家站在 ServerIP 上的 TCP 端口，没有开启时为 0。客户端把本机一个 TCP 端口转发过去
	PortalPort int `json:"portal_port"`
}

// JoinServer 是加入响应里的一台服务器。Key 在房间内唯一（Commander 里的服务器 ID）。
// ASA 服务器没有 Steam 查询端口（QueryPort 为 0），玩家只能在游戏控制台里用 open 命令进服。
type JoinServer struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Map       string `json:"map"`
	Game      Game   `json:"game"` // ase 或 asa，客户端解析后不会为空
	Online    bool   `json:"online"`
	GamePort  int    `json:"game_port"`
	QueryPort int    `json:"query_port"`
}

// User 是 GET /me 返回的当前用户。
type User struct {
	SteamID   string `json:"steam_id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// UnmarshalJSON 同时接受 snake_case 和网站现有 /me 的 Go 默认字段名（SteamID、AvatarURL）。
func (u *User) UnmarshalJSON(b []byte) error {
	var v struct {
		SteamID    string `json:"steam_id"`
		Name       string `json:"name"`
		AvatarURL  string `json:"avatar_url"`
		SteamID2   string `json:"SteamID"`
		Name2      string `json:"Name"`
		AvatarURL2 string `json:"AvatarURL"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*u = User{SteamID: first(v.SteamID, v.SteamID2), Name: first(v.Name, v.Name2), AvatarURL: first(v.AvatarURL, v.AvatarURL2)}
	return nil
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// UnixTime 是 Unix 秒。约定没写时间字段的格式，这里同时接受数字和 RFC 3339 字符串。
type UnixTime int64

func (t *UnixTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*t = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*t = 0
			return nil
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			*t = UnixTime(n)
			return nil
		}
		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return err
		}
		if tm.Year() <= 1 { // Go 的零值时间
			*t = 0
			return nil
		}
		*t = UnixTime(tm.Unix())
		return nil
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*t = UnixTime(int64(f))
	return nil
}
