package arkfgf

import (
	"bytes"
	"encoding/json"
	"strconv"
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
	Online      bool     `json:"online"`
	LastSeen    UnixTime `json:"last_seen"`
	CreatedAt   UnixTime `json:"created_at"`
	Country     string   `json:"country"`
	Province    string   `json:"province"`
}

// Mod 是房间详情里带名称的 Mod。
type Mod struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
	GamePort            int      `json:"game_port"`
	QueryPort           int      `json:"query_port"`
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
